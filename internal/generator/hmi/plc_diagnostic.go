// Диагностика ПЛК собирает иерархию HMI-панелей из инвентаря и встроенного подтверждённого профиля.
package hmi

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"scheme-xml-generator/internal/domain/hardware"
	iomap "scheme-xml-generator/internal/domain/inventory"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/exportprofile"
	"scheme-xml-generator/internal/generator/identifiers"
	moduleid "scheme-xml-generator/internal/generator/modules"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed assets/plc_diagnostic_profile.xml.gz
var plcDiagnosticProfileGZIP []byte

var plcProfileOnce sync.Once

var plcProfile plcDiagnosticProfile

var plcProfileError error

// loadPLCDiagnosticProfile Читает встроенный проверенный профиль HMI из сжатого XML-ресурса.
// Возвращает определения символов и параметров либо ошибку загрузки профиля.
func loadPLCDiagnosticProfile() (plcDiagnosticProfile, error) {
	plcProfileOnce.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(plcDiagnosticProfileGZIP))
		if err != nil {
			plcProfileError = err
			return
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, 4<<20))
		if err == nil {
			err = xml.Unmarshal(data, &plcProfile)
		}
		if err == nil && (len(plcProfile.Pictures) != 3 || len(plcProfile.Front) != 10 || len(plcProfile.Back) != 10 || len(plcProfile.Symbols) != 10) {
			err = fmt.Errorf("повреждён встроенный профиль диагностики")
		}
		plcProfileError = err
	})
	return plcProfile, plcProfileError
}

// PLCDiagnosticPlan owns a deep snapshot of one physical PLC inventory. Counts
// include all three nested page levels, and T11Count also reserves receptor,
// chart and parameter-record IDs. Program POU IDs are never consumed.
type PLCDiagnosticPlan struct {
	ControllerName string
	Controller     iomap.Controller
	FrameCount     int
	T11Count       int
	CardCount      int
	SignalCount    int
	Warnings       []string
}

// PreparePLCDiagnosticPlans Выбирает контроллеры исходного IO-листа для диагностических кадров HMI.
// Проверяет инвентарь модулей и формирует планы с явным контекстом экспорта.
func PreparePLCDiagnosticPlans(source *iomap.Plan, selected []iomap.Selection, ctx HMIContext) ([]PLCDiagnosticPlan, error) {
	ctx, err := normalizeHMIContext(ctx)
	if err != nil {
		return nil, err
	}
	if source == nil || len(source.Controllers) == 0 || len(source.Controllers) > 128 || len(selected) == 0 || len(selected) > 128 {
		return nil, fmt.Errorf("диагностика IO: выберите от 1 до 128 ПЛК")
	}
	available := make(map[string]iomap.Controller)
	for _, controller := range source.Controllers {
		if controller.Key == "" {
			return nil, fmt.Errorf("диагностика IO: пустой ключ ПЛК")
		}
		if _, exists := available[controller.Key]; exists {
			return nil, fmt.Errorf("диагностика IO: повторный ключ ПЛК %s", controller.Key)
		}
		available[controller.Key] = controller
	}
	keys, names := map[string]bool{}, map[string]bool{}
	plans := make([]PLCDiagnosticPlan, 0, len(selected))
	totalModules := 0
	for _, selection := range selected {
		controller, exists := available[selection.Key]
		if !exists || keys[selection.Key] {
			return nil, fmt.Errorf("диагностика IO: неизвестный или повторный ПЛК %q", selection.Key)
		}
		keys[selection.Key] = true
		name := strings.TrimSpace(selection.Name)
		if name == "" {
			name = controller.Name
		}
		if !identifiers.ControllerNamePattern.MatchString(name) || names[strings.ToUpper(name)] {
			return nil, fmt.Errorf("диагностика IO: неверное или повторное имя ПЛК %q", name)
		}
		names[strings.ToUpper(name)] = true
		controller.Racks = append([]iomap.Rack(nil), controller.Racks...)
		controller.Modules = append([]iomap.Module(nil), controller.Modules...)
		for i := range controller.Modules {
			module := &controller.Modules[i]
			module.Channels = append([]iomap.Channel(nil), module.Channels...)
			for j := range module.Channels {
				channel := &module.Channels[j]
				if channel.Reserve {
					channel.Tag = "_" + name + "_" + module.Name + "_" + strconv.Itoa(channel.Channel)
				}
			}
		}
		controller.Name = name
		if err := validatePLCInventory(controller); err != nil {
			return nil, err
		}
		totalModules += len(controller.Modules)
		if totalModules > 4096 {
			return nil, fmt.Errorf("диагностика IO: не более 4096 модулей за одну генерацию")
		}
		sort.Slice(controller.Racks, func(i, j int) bool { return plcDiagnosticRackLess(controller.Racks[i].Name, controller.Racks[j].Name) })
		sort.Slice(controller.Modules, func(i, j int) bool {
			a, b := controller.Modules[i], controller.Modules[j]
			if a.Rack != b.Rack {
				return plcDiagnosticRackLess(a.Rack, b.Rack)
			}
			return a.Slot < b.Slot
		})
		plan := PLCDiagnosticPlan{ControllerName: name, Controller: controller, Warnings: append([]string(nil), source.Warnings...)}
		_, summary, count, err := buildPLCDiagnostic(plan, ctx, contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 1000000, PageStart: 1000000})
		if err != nil {
			return nil, err
		}
		plan.FrameCount, plan.T11Count, plan.CardCount, plan.SignalCount = summary.FrameCount, count, summary.Cards, summary.SignalCount
		plans = append(plans, plan)
	}
	return plans, nil
}

// The inventory permits different digit widths: A2 precedes A10. The spelling
// tie-breaker keeps allocation deterministic even for zero-padded rack names.
// Callers validate the A + 1..6 digit grammar before invoking this helper.
func plcDiagnosticRackLess(left, right string) bool {
	a, _ := strconv.Atoi(strings.TrimPrefix(left, "A"))
	b, _ := strconv.Atoi(strings.TrimPrefix(right, "A"))
	if a != b {
		return a < b
	}
	return left < right
}

// validatePLCInventory Проверяет физический состав выбранного ПЛК для HMI-профиля.
// Отклоняет неподдержанные типы и конфликтующие записи до построения страниц.
func validatePLCInventory(controller iomap.Controller) error {
	if !identifiers.ControllerNamePattern.MatchString(controller.Name) || len(controller.Racks) == 0 || len(controller.Racks) > 64 || len(controller.Modules) == 0 || len(controller.Modules) > 1024 {
		return fmt.Errorf("диагностика IO: неверный ПЛК, список крейтов или модулей %q", controller.Name)
	}
	racks, positions := map[string]bool{}, map[string]bool{}
	firstRack := ""
	for _, rack := range controller.Racks {
		position := fmt.Sprintf("%s:%d", rack.Panel, rack.Order)
		if !moduleid.RackPrefixPattern.MatchString(rack.Name) || racks[rack.Name] || (rack.Panel != "front" && rack.Panel != "back") || rack.Order < 0 || rack.Order > 31 || positions[position] {
			return fmt.Errorf("диагностика IO: неверное или повторное размещение крейта %s", rack.Name)
		}
		racks[rack.Name], positions[position] = true, true
		if firstRack == "" || plcDiagnosticRackLess(rack.Name, firstRack) {
			firstRack = rack.Name
		}
	}
	modules := map[string]bool{}
	for _, module := range controller.Modules {
		hardwareModule, _ := hardware.Lookup(module.Type)
		capacity := hardwareModule.Channels
		if capacity == 0 || module.Capacity != capacity || !racks[module.Rack] || module.Slot < 0 || module.Slot > 15 || module.Name != fmt.Sprintf("%s_%02d", module.Rack, module.Slot) || modules[module.Name] || len(module.Channels) != capacity {
			return fmt.Errorf("диагностика IO: неверный модуль, тип или каналы %s", module.Name)
		}
		if module.Rack == firstRack && module.Slot < 2 {
			return fmt.Errorf("диагностика IO: %s занимает слот CPU 00/01 первого крейта %s", module.Name, firstRack)
		}
		modules[module.Name] = true
		for i, channel := range module.Channels {
			if channel.Channel != i || !identifiers.ObjectNamePattern.MatchString(channel.Tag) {
				return fmt.Errorf("диагностика IO: неверный тег или номер канала %s:%d", module.Name, i)
			}
			if expected := exportprofile.DiagnosticObjectType(module.Type); expected != "" && channel.ObjectType != expected {
				return fmt.Errorf("диагностика IO: неподдерживаемый тип объекта %q для %s:%d", channel.ObjectType, module.Name, i)
			}
		}
	}
	return nil
}

type plcDiagnosticBuilder struct {
	doc            plcDiagnosticDocument
	ctx            HMIContext
	controllerName string
	ids            contracts.DiagnosticIDRange
	nextID         int64
	nextPage       int64
	cards          map[string]string
	summary        contracts.Summary
}

// id Выдаёт следующий ID примитива внутри уже выделенного диапазона HMI.
// Меняет только локальный курсор построителя, не пишет state.json.
func (b *plcDiagnosticBuilder) id() string {
	id := b.nextID
	b.nextID++
	return strconv.FormatInt(id, 10)
}

// card Добавляет или переиспользует HMI-карточку по строке привязки.
// Возвращает её ID из выделенного диапазона построителя диагностики.
func (b *plcDiagnosticBuilder) card(info string) string {
	key := strings.ToUpper(info)
	if id, ok := b.cards[key]; ok {
		return id
	}
	id := strconv.FormatInt(b.ids.CardStart+int64(len(b.cards)), 10)
	b.cards[key] = id
	b.doc.Cards = append(b.doc.Cards, outputHMICard{ID: id, Info: info})
	return id
}

// objectCard Формирует типизированную привязку объекта и получает карточку HMI.
// Используется символами диагностики для связи тега с определением профиля.
func (b *plcDiagnosticBuilder) objectCard(tag, objectType string) string {
	return b.card("2/" + b.controllerName + "/" + b.ctx.ResourceNumber + "/" + tag + "/(" + objectType + ")")
}

// page Создаёт страницу HMI с именем, путём и размерами выбранного диагностического кадра.
// Берёт PageID из локального диапазона построителя и возвращает модель страницы.
func (b *plcDiagnosticBuilder) page(name, path, module string, width, height int) plcDiagnosticPage {
	id := strconv.FormatInt(b.nextPage, 10)
	b.nextPage++
	b.doc.Groups = append(b.doc.Groups, plcDiagnosticGroup{ID: id, FullName: path})
	b.summary.Frames = append(b.summary.Frames, contracts.DiagnosticFrameSummary{ID: b.nextPage - 1, Name: name, Module: module})
	return plcDiagnosticPage{IDAttribute: id, ID: id, Name: name, TemplateID: "0", ForMarka: "0", Background: "536870913", DParams: "2", Height: strconv.Itoa(height), GridSize: "10", Width: strconv.Itoa(width), Number: "0", PrintWidth: strconv.Itoa(width), PrintHeight: strconv.Itoa(height), PrintPageA4: "8", FrameNumber: "4", Srez: "1", PageLayers: []plcDiagnosticLayer{{Number: "1", Visible: "1", Name: "[по умолчанию]"}}}
}

// primitive Создаёт базовый графический примитив HMI с геометрией и новым локальным ID.
// Специализированные символы дополняют его параметрами и карточкой.
func (b *plcDiagnosticBuilder) primitive(x, y, width, height int) plcDiagnosticPrimitive {
	b.summary.Graphics++
	return plcDiagnosticPrimitive{T11ID: b.id(), X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: strconv.Itoa(height), ObjectType: "8", GroupNumber: "0", DrawType: "0", PenParams: "1", PenColor: "0", BrushColor: "16777215", GradColor: "536870911", Params: aoDiagnosticParams}
}

// symbol Создаёт символ диагностического HMI-профиля по MS-ID и карточке.
// Привязывает определение символа к заданной геометрии кадра.
func (b *plcDiagnosticBuilder) symbol(x, y, width, height int, ms, card string) plcDiagnosticPrimitive {
	p := b.primitive(x, y, width, height)
	p.ObjectMSID, p.CardID = ms, card
	return p
}

// link Добавляет переход графического элемента на другую страницу HMI.
// Меняет параметры переданного примитива, используя фактический ID целевого кадра.
func (b *plcDiagnosticBuilder) link(p *plcDiagnosticPrimitive, page string) {
	p.Receptors = &plcDiagnosticReceptors{Items: []plcDiagnosticReceptor{{ID: b.id(), Type: "1", Int: page, Float: "0", Text: "0;0;0;();", Event: "1", DParams: "1", Rights: "0"}}}
}

// plcModuleDiagnosticTag Определяет диагностический тег физического модуля выбранного ПЛК.
// Привязка используется карточками и символами диагностического HMI-профиля.
func plcModuleDiagnosticTag(controllerName string, module iomap.Module) string {
	tag := "_" + controllerName + "_" + module.Name
	if module.Type == "AI16H" || module.Type == "AOC4H" {
		tag += "_" + module.Type
	}
	return tag
}

// plcFrameName Определяет имя HMI-кадра модуля из ПЛК и его инвентарной записи.
// Согласует имена страниц и переходов диагностики.
func plcFrameName(controllerName string, module iomap.Module) string {
	suffix := controllerName
	if i := strings.LastIndex(controllerName, "_SC_"); i >= 0 {
		suffix = controllerName[i+4:]
	}
	prefix := "AI_"
	if module.Type == "AOC4H" {
		prefix = "AO_"
	}
	return prefix + suffix + "_" + module.Name + "_" + module.Type
}

// buildPLCDiagnostic Собирает диагностические страницы, карточки и переходы по плану ПЛК.
// Возвращает модель XML, сводку и расход ID для проверки перед сериализацией.
func buildPLCDiagnostic(plan PLCDiagnosticPlan, ctx HMIContext, ids contracts.DiagnosticIDRange) (plcDiagnosticDocument, contracts.Summary, int, error) {
	profile, err := loadPLCDiagnosticProfile()
	if err != nil {
		return plcDiagnosticDocument{}, contracts.Summary{}, 0, err
	}
	b := plcDiagnosticBuilder{ctx: ctx, controllerName: plan.ControllerName, ids: ids, nextID: ids.T11Start, nextPage: ids.PageStart, cards: map[string]string{}, summary: contracts.Summary{POUs: []contracts.POUSummary{}, Frames: []contracts.DiagnosticFrameSummary{}}}
	b.doc = plcDiagnosticDocument{XMLName: xml.Name{Local: "BufScada"}, Common: outputHMICommon{Version: ctx.Version, Project: ctx.Project}, Colors: profile.Colors, Pictures: profile.Pictures, Symbols: profile.Symbols}
	b.doc.Groups = []plcDiagnosticGroup{{ID: "4705", FullName: "Диагностика"}, {ID: "6580", FullName: "Служебные шаблоны\\Шаблон диагностики контроллеров_PS"}}
	rootPath := "Диагностика\\" + plan.ControllerName
	root := b.page(plan.ControllerName, rootPath, "", 1920, 960)
	root.PrintHeight = "980"
	root.Children = &plcDiagnosticChildren{}
	firstRack := plan.Controller.Racks[0].Name
	for _, rack := range plan.Controller.Racks {
		if plcDiagnosticRackLess(rack.Name, firstRack) {
			firstRack = rack.Name
		}
	}
	var panels [2]plcDiagnosticPage
	for index, side := range []string{"front", "back"} {
		label := "Передняя панель"
		if side == "back" {
			label = "Задняя панель"
		}
		name := "Шкаф РСУ " + plan.ControllerName + ". " + label
		height := 960
		for _, rack := range plan.Controller.Racks {
			if rack.Panel == side && 520+440*rack.Order > height {
				height = 520 + 440*rack.Order
			}
		}
		panels[index] = b.page(name, rootPath+"\\"+name, "", 1920, height)
		panels[index].TemplateID = "6580"
		panels[index].Children = &plcDiagnosticChildren{}
	}
	for index, side := range []string{"front", "back"} {
		panel := &panels[index]
		layer := &panel.PageLayers[0]
		racks := map[string]iomap.Rack{}
		for _, rack := range plan.Controller.Racks {
			if rack.Panel != side {
				continue
			}
			racks[rack.Name] = rack
			y := 90 + 440*rack.Order
			for _, part := range []struct {
				x, w int
				pic  string
			}{{95, 1009, "727"}, {40, 55, "985"}, {1104, 55, "986"}} {
				p := b.primitive(part.x, y, part.w, 383)
				p.ObjectType = "5"
				p.PicID = part.pic
				p.PenColor = "536870911"
				p.BrushColor = "536870911"
				p.Params = "[HINT]="
				layer.Primitives = append(layer.Primitives, p)
			}
			if rack.Name == firstRack {
				layer.Primitives = append(layer.Primitives, b.symbol(95, y, 126, 383, "4234", b.card("7///"+plan.ControllerName+"/(TENIX-CPU715)")))
			}
		}
		var panelModules []iomap.Module
		for _, module := range plan.Controller.Modules {
			rack, ok := racks[module.Rack]
			if !ok {
				continue
			}
			panelModules = append(panelModules, module)
			b.summary.SignalCount += module.Capacity
			ms, objType, h := "3625", "AI_DIAG16_AD3v1_kvit", 382
			switch module.Type {
			case "AOC4H":
				ms, h = "3634", 383
			case "DI32":
				ms, objType, h = "3657", "D32V", 383
			case "DO32P":
				ms, objType, h = "3658", "D32V", 383
			}
			moduleCard := b.objectCard(plcModuleDiagnosticTag(plan.ControllerName, module), objType)
			p := b.symbol(95+63*module.Slot, 90+440*rack.Order, 63, h, ms, moduleCard)
			if module.Type == "AI16H" || module.Type == "AOC4H" {
				frameName := plcFrameName(plan.ControllerName, module)
				frameHeight, bgMS := 448, "3654"
				if module.Type == "AOC4H" {
					frameHeight, bgMS = 152, "3655"
				}
				frame := b.page(frameName, rootPath+"\\"+panel.Name+"\\"+frameName, module.Name, 1200, frameHeight)
				frame.PrintWidth, frame.PrintHeight = "600", "800"
				if module.Type == "AOC4H" {
					frame.DParams, frame.FrameNumber = "66", "5"
				}
				// AI and AO frame headers share the cabinet module's diagnostic object.
				frame.PageLayers[0].Primitives = append(frame.PageLayers[0].Primitives, b.symbol(0, 0, 1200, frameHeight, bgMS, moduleCard))
				for _, channel := range module.Channels {
					y, rowMS := 54+24*channel.Channel, "3679"
					if module.Type == "AI16H" {
						y += 2 * (channel.Channel / 4)
						rowMS = "4730"
						if channel.Channel%2 == 1 {
							rowMS = "4731"
						}
					}
					frame.PageLayers[0].Primitives = append(frame.PageLayers[0].Primitives, b.symbol(70, y, 1080, 24, rowMS, b.objectCard(channel.Tag, channel.ObjectType)))
				}
				b.link(&p, frame.ID)
				panel.Children.Pages = append(panel.Children.Pages, frame)
			}
			layer.Primitives = append(layer.Primitives, p)
		}
		b.panelControls(panel, panels[1-index].ID, index, panelModules, profile)
	}
	// Native export lists rear before front; pagination remains front=1/rear=2.
	root.Children.Pages = append(root.Children.Pages, panels[1], panels[0])
	b.doc.Pages = []plcDiagnosticPage{root}
	b.summary.FrameCount = int(b.nextPage - ids.PageStart)
	b.summary.Cards = len(b.cards)
	b.summary.IOModuleCount = len(plan.Controller.Modules)
	b.summary.T11First, b.summary.T11Last = ids.T11Start, b.nextID-1
	b.summary.CardFirst, b.summary.CardLast = ids.CardStart, ids.CardStart+int64(len(b.cards))-1
	return b.doc, b.summary, int(b.nextID - ids.T11Start), nil
}

// panelControls Добавляет органы навигации и диагностические элементы панели HMI.
// Использует модули шкафа и определения встроенного профиля, обновляет страницу.
func (b *plcDiagnosticBuilder) panelControls(panel *plcDiagnosticPage, otherID string, index int, modules []iomap.Module, profile plcDiagnosticProfile) {
	controls := profile.Front
	if index == 1 {
		controls = profile.Back
	}
	height, _ := strconv.Atoi(panel.Height)
	delta := height - 960
	for _, original := range controls {
		p := original
		p.T11ID = b.id()
		b.summary.Graphics++
		p.Receptors = nil
		if p.ObjectType == "14" {
			rec := plcDiagnosticReceptor{ID: b.id(), Type: "3", Int: "-10", Float: "1", Event: "1", DParams: "0", Rights: "0", Charts: &plcDiagnosticCharts{}}
			for _, module := range modules {
				paramID := b.id()
				command := "КОМ. КВИТИРОВАТЬ"
				if module.Type == "DI32" || module.Type == "DO32P" {
					command = "D32_КОМ. КВИТИРОВАТЬ"
				}
				b.doc.CardParams = append(b.doc.CardParams, outputHMIPageMS{ID: paramID, Info: "2/" + b.controllerName + "/" + b.ctx.ResourceNumber + "/" + plcModuleDiagnosticTag(b.controllerName, module) + "/([]" + command + ")"})
				chartID := b.id()
				rec.Charts.Items = append(rec.Charts.Items, plcDiagnosticChart{IDAttribute: chartID, ID: chartID, ParentID: rec.ID, Mode: "12", ParamID: paramID, Color: "0", LSide: "1", NDParamID: "-1", Style: "0", Width: "1", Stairs: "0", Marks: "0", BitNum: "-1", NDBitNum: "-1", ParamMode: "0", TreePID: "0", TimeOffset: "0"})
			}
			p.Receptors = &plcDiagnosticReceptors{Items: []plcDiagnosticReceptor{rec}}
		} else {
			y, _ := strconv.Atoi(p.Y)
			p.Y = strconv.Itoa(y + delta)
			// The source native front arrow has no ScriptName; the rear page
			// uses CAROUSEL_BACK. Their complementary arrows remain disabled.
			if index == 0 && p.X == "1868" && p.ScriptName == "" || index == 1 && p.ScriptName == "CAROUSEL_BACK" {
				b.link(&p, otherID)
			}
		}
		panel.PageLayers[0].Primitives = append(panel.PageLayers[0].Primitives, p)
	}
}

// GeneratePLCDiagnostic Экспортирует диагностические HMI-кадры выбранного ПЛК в SCADA XML.
// Строит модель, сериализует и проверяет её, возвращает файл и сводку без импорта SCADA.
func (g Generator) GeneratePLCDiagnostic(plan PLCDiagnosticPlan, ctx HMIContext, ids contracts.DiagnosticIDRange) (contracts.Result, error) {
	ctx, err := normalizeHMIContext(ctx)
	if err != nil {
		return contracts.Result{}, err
	}
	if plan.ControllerName != plan.Controller.Name {
		return contracts.Result{}, fmt.Errorf("диагностика IO: несогласованное имя ПЛК")
	}
	if err := validatePLCInventory(plan.Controller); err != nil {
		return contracts.Result{}, err
	}
	for _, item := range []struct {
		start int64
		count int
		name  string
	}{{ids.T11Start, plan.T11Count, "ID примитивов/рецепторов"}, {ids.CardStart, plan.CardCount, "CardID"}, {ids.PageStart, plan.FrameCount, "PageID"}} {
		if _, err := allocation.AddTransportCount(item.start, item.count, item.name); err != nil {
			return contracts.Result{}, err
		}
	}
	for _, external := range []int64{4705, 6580} {
		if external >= ids.PageStart && external < ids.PageStart+int64(plan.FrameCount) {
			return contracts.Result{}, fmt.Errorf("диагностика IO: диапазон страниц пересекает внешний шаблон %d", external)
		}
	}
	doc, summary, count, err := buildPLCDiagnostic(plan, ctx, ids)
	if err != nil {
		return contracts.Result{}, err
	}
	if count != plan.T11Count || summary.Cards != plan.CardCount || summary.FrameCount != plan.FrameCount || summary.SignalCount != plan.SignalCount {
		return contracts.Result{}, fmt.Errorf("диагностика IO: план изменился после расчёта ID")
	}
	data, err := xmlcodec.SerializeSCADAValue(doc)
	if err != nil {
		return contracts.Result{}, err
	}
	if err := validateGeneratedPLCDiagnostic(data, doc); err != nil {
		return contracts.Result{}, err
	}
	warnings := append([]string(nil), plan.Warnings...)
	warnings = append(warnings, "Размещение крейтов на передней/задней панели и CPU TENIX-CPU715 в слотах 00/01 первого крейта следует соглашению образца; проверьте его для выбранного шкафа.", "Импортируйте группу ПЛК внутрь группы «Диагностика». Требуются существующие служебный шаблон «Шаблон диагностики контроллеров_PS», мнемосимволы и экземпляры объектов; этот XML не создаёт их программную часть.")
	return contracts.Result{XML: data, BaseName: "PLC_DIAG", Summary: summary, Warnings: warnings}, nil
}

// validateGeneratedPLCDiagnostic Сверяет сериализованный HMI-документ с ожидаемыми страницами и карточками.
// Проверяет целостность выходных ссылок, не запускает целевую SCADA.
func validateGeneratedPLCDiagnostic(data []byte, expected plcDiagnosticDocument) error {
	var actual plcDiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, xmlcodec.Utf8BOM), &actual); err != nil {
		return fmt.Errorf("некорректный XML диагностики ПЛК: %w", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("диагностика ПЛК: структура изменилась при сериализации")
	}
	return validatePLCReferences(actual)
}
