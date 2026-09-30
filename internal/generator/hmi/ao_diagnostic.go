// AO-диагностика строит отдельные HMI-кадры модулей и проверяет привязки технологических и резервных объектов.
package hmi

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	aomap "scheme-xml-generator/internal/domain/analogoutput"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/generator/allocation"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	"scheme-xml-generator/internal/generator/identifiers"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	moduleid "scheme-xml-generator/internal/generator/modules"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"sort"
	"strconv"
	"strings"
)

// AODiagnosticPlan owns a validated snapshot for exactly one PLC. Unlike FBD,
// every mapped physical channel remains visible, including repeated tags.
type AODiagnosticPlan struct {
	ControllerName string
	Frames         []AODiagnosticFrame
	FrameCount     int
	T11Count       int
	CardCount      int
	SignalCount    int
}

type AODiagnosticFrame struct {
	Name   string
	Module string
	Tags   [hardware.AOC4HChannels]string
}

// PrepareAODiagnosticPlans selects entire controllers and validates all of
// them before transport IDs are reserved. A11_02 and A11_03 are separate pages;
// the A11 grouping used for POU generation is intentionally not a page boundary.
func PrepareAODiagnosticPlans(source *aomap.Plan, selectedControllers []string, ctx HMIContext) ([]AODiagnosticPlan, error) {
	if _, err := normalizeHMIContext(ctx); err != nil {
		return nil, err
	}
	if source == nil || len(source.Groups) == 0 || len(source.Groups) > 128 || len(selectedControllers) == 0 || len(selectedControllers) > 128 {
		return nil, fmt.Errorf("диагностика: выберите от 1 до 128 ПЛК из карты AO")
	}
	available := map[string]bool{}
	groupKeys := map[string]bool{}
	for _, group := range source.Groups {
		if group.Key == "" || groupKeys[group.Key] {
			return nil, fmt.Errorf("диагностика: пустой или повторный ключ группы в карте AO")
		}
		groupKeys[group.Key] = true
		available[group.ControllerName] = true
	}
	selected := map[string]bool{}
	for _, controllerName := range selectedControllers {
		if !available[controllerName] || selected[controllerName] {
			return nil, fmt.Errorf("диагностика: неизвестный или повторно выбранный ПЛК %q", controllerName)
		}
		selected[controllerName] = true
	}
	var plans []AODiagnosticPlan
	byController := map[string]int{}
	totalFrames := 0
	for _, group := range source.Groups {
		if !selected[group.ControllerName] {
			continue
		}
		if !identifiers.ControllerNamePattern.MatchString(group.ControllerName) || !moduleid.RackPrefixPattern.MatchString(group.Prefix) || len(group.Modules) == 0 {
			return nil, fmt.Errorf("диагностика: неверный ПЛК, префикс или пустая группа %s", group.Key)
		}
		planIndex, exists := byController[group.ControllerName]
		if !exists {
			planIndex = len(plans)
			byController[group.ControllerName] = planIndex
			plans = append(plans, AODiagnosticPlan{ControllerName: group.ControllerName})
		}
		modules := append([]aomap.Module(nil), group.Modules...)
		moduleIndices := map[string]int{}
		usedIndices := map[int]bool{}
		for _, module := range modules {
			index, err := moduleid.AoSTModuleIndex(module.Name, group.Prefix)
			if err != nil || usedIndices[index] || module.ObjectType != "AN_v1" || len(module.Channels) != hardware.AOC4HChannels {
				return nil, fmt.Errorf("диагностика: модуль %s должен быть уникальным AN_v1 с четырьмя каналами", module.Name)
			}
			moduleIndices[module.Name], usedIndices[index] = index, true
		}
		sort.Slice(modules, func(i, j int) bool { return moduleIndices[modules[i].Name] < moduleIndices[modules[j].Name] })
		for _, module := range modules {
			frame := AODiagnosticFrame{Name: aoDiagnosticFrameName(group.ControllerName, module.Name), Module: module.Name}
			for index, channel := range module.Channels {
				if channel.Channel != index || !identifiers.ObjectNamePattern.MatchString(channel.Tag) {
					return nil, fmt.Errorf("диагностика %s/%s: неверный тег или последовательность каналов 0..%d", group.ControllerName, module.Name, hardware.AOC4HChannels-1)
				}
				frame.Tags[index] = channel.Tag // Never apply the FBD Duplicate omission rule.
			}
			plans[planIndex].Frames = append(plans[planIndex].Frames, frame)
			totalFrames++
			if totalFrames > 4096 {
				return nil, fmt.Errorf("диагностика: допускается не более 4096 кадров во всех выбранных ПЛК")
			}
		}
	}
	for index := range plans {
		if err := validateAODiagnosticPlan(&plans[index]); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

// aoDiagnosticFrameName Строит имя диагностического кадра из ПЛК и физического AO-модуля.
// Имя используется одинаково для страницы HMI и ссылок на неё.
func aoDiagnosticFrameName(controllerName, module string) string {
	suffix := controllerName
	if marker := strings.LastIndex(controllerName, "_SC_"); marker >= 0 {
		suffix = controllerName[marker+len("_SC_"):]
	}
	return "AO_" + suffix + "_" + module + "_AOC4H"
}

// validateAODiagnosticPlan Проверяет AO-модули и каналы до резервирования ID диагностических кадров.
// Не допускает конфликтующих имён и неподходящего состава плана HMI.
func validateAODiagnosticPlan(plan *AODiagnosticPlan) error {
	if !identifiers.ControllerNamePattern.MatchString(plan.ControllerName) || strings.HasSuffix(plan.ControllerName, "_") || len(plan.Frames) == 0 || len(plan.Frames) > 4096 {
		return fmt.Errorf("диагностика: неверный ПЛК или число кадров")
	}
	names, modules, tags := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, frame := range plan.Frames {
		prefix, _, _ := strings.Cut(frame.Module, "_")
		if _, err := moduleid.AoSTModuleIndex(frame.Module, prefix); err != nil || !moduleid.RackPrefixPattern.MatchString(prefix) {
			return fmt.Errorf("диагностика: неверное имя модуля %q", frame.Module)
		}
		if frame.Name != aoDiagnosticFrameName(plan.ControllerName, frame.Module) || identifiers.ValidatePOUName(frame.Name) != nil || names[strings.ToUpper(frame.Name)] || modules[strings.ToUpper(frame.Module)] {
			return fmt.Errorf("диагностика: неверное или повторное имя кадра %q", frame.Name)
		}
		names[strings.ToUpper(frame.Name)], modules[strings.ToUpper(frame.Module)] = true, true
		for _, tag := range frame.Tags {
			if !identifiers.ObjectNamePattern.MatchString(tag) {
				return fmt.Errorf("диагностика %s: неверный тег %q", frame.Name, tag)
			}
			tags[strings.ToUpper(tag)] = true
		}
	}
	plan.FrameCount, plan.T11Count = len(plan.Frames), 5*len(plan.Frames)
	plan.CardCount, plan.SignalCount = len(tags), hardware.AOC4HChannels*len(plan.Frames)
	return nil
}

type outputAODiagnosticDocument struct {
	XMLName     xml.Name                 `xml:"BufScada"`
	Common      outputHMICommon          `xml:"Common"`
	Pages       []outputAODiagnosticPage `xml:"Pages>OnePage"`
	ColorStyles []outputHMIColor         `xml:"COLORSTYLES>rec"`
	Cards       []outputHMICard          `xml:"CARDSINFO>rec"`
	PageMS      []outputHMIPageMS        `xml:"PAGEMSINFO>rec"`
}

// Field order and empty elements follow diagnostic.xml's native profile.
type outputAODiagnosticPage struct {
	IDAttribute string                    `xml:"ID,attr"`
	ID          string                    `xml:"ID"`
	Name        string                    `xml:"NAME"`
	TemplateID  string                    `xml:"SHABLONPAGEID"`
	ForMarka    string                    `xml:"FORMARKA"`
	ExData      string                    `xml:"EXDATA"`
	Background  string                    `xml:"FONCOLOR"`
	DParams     string                    `xml:"DPARAMS"`
	Height      string                    `xml:"HEIGHT"`
	GridSize    string                    `xml:"GRIDSIZE"`
	Width       string                    `xml:"WIDTH"`
	Number      string                    `xml:"NUM"`
	PrintWidth  string                    `xml:"PRINTWIDTH"`
	PrintHeight string                    `xml:"PRINTHEIGHT"`
	PrintPageA4 string                    `xml:"PRINTPAGEA4"`
	FrameNumber string                    `xml:"FRAMENUM"`
	Srez        string                    `xml:"SREZ"`
	Description string                    `xml:"DISC"`
	Layers      string                    `xml:"LAYERS"`
	ScriptCode  string                    `xml:"SCRIPTCODE"`
	PageLayers  []outputAODiagnosticLayer `xml:"PageLayers>OneLayer"`
}

type outputAODiagnosticLayer struct {
	Number     string                        `xml:"Num,attr"`
	Visible    string                        `xml:"Visible,attr"`
	Name       string                        `xml:"Name,attr"`
	Primitives []outputAODiagnosticPrimitive `xml:"OnePrim"`
}

type outputAODiagnosticPrimitive struct {
	T11ID       string `xml:"SourceT11ID,attr"`
	X           string `xml:"X,attr"`
	Y           string `xml:"Y,attr"`
	Width       string `xml:"WIDTH,attr"`
	Height      string `xml:"HEIGHT,attr"`
	ObjectType  string `xml:"OBJTYPE,attr"`
	GroupNumber string `xml:"GRNUM,attr"`
	DrawType    string `xml:"DRAWTYPE,attr"`
	PenParams   string `xml:"PenParams,attr"`
	PenColor    string `xml:"PenColor,attr"`
	BrushColor  string `xml:"BrushColor,attr"`
	GradColor   string `xml:"GradColor,attr"`
	ScriptName  string `xml:"ScriptName,attr"`
	Params      string `xml:"PARAMS"`
	ObjectMSID  string `xml:"ObjMSID"`
	CardID      string `xml:"CardID"`
}

const aoDiagnosticLayerName = "[\ufba0\U000ecbab\u00f7\u0b68\u00fe]"

const aoDiagnosticParams = "[MODE]=-1\n[TEXT]=\n[FONTID]=0\n[USERFONT]=8;Arial;0;0;\n"

// aoDiagnosticPrimitive Создаёт экземпляр графического символа AO-диагностики с привязкой к карточке.
// Получает геометрию и ID профиля, возвращает примитив для кадра HMI.
func aoDiagnosticPrimitive(id int64, x, y, width, height int, objectMSID, cardID string) outputAODiagnosticPrimitive {
	return outputAODiagnosticPrimitive{T11ID: strconv.FormatInt(id, 10), X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: strconv.Itoa(height),
		ObjectType: "8", GroupNumber: "0", DrawType: "0", PenParams: "1", PenColor: "0", BrushColor: "16777215", GradColor: "536870911",
		Params: aoDiagnosticParams, ObjectMSID: objectMSID, CardID: cardID}
}

// GenerateAODiagnostic produces native panel pages, not BufScadaPOUS or POU
// instances. The rows bind existing AN_v1 objects by controller/resource/tag.
func (g Generator) GenerateAODiagnostic(plan AODiagnosticPlan, ctx HMIContext, ids xmlidentity.DiagnosticIDRange) (xmlartifact.Result, error) {
	if err := validateAODiagnosticPlan(&plan); err != nil {
		return xmlartifact.Result{}, err
	}
	ctx, err := normalizeHMIContext(ctx)
	if err != nil {
		return xmlartifact.Result{}, err
	}
	for _, item := range []struct {
		start int64
		count int
		name  string
	}{{ids.T11Start, plan.T11Count, "SourceT11ID"}, {ids.CardStart, plan.CardCount, "CardID"}, {ids.PageStart, plan.FrameCount, "PageID"}} {
		if _, err := allocation.AddTransportCount(item.start, item.count, item.name); err != nil {
			return xmlartifact.Result{}, err
		}
	}
	doc := outputAODiagnosticDocument{XMLName: xml.Name{Local: "BufScada"}, Common: outputHMICommon{Version: ctx.Version, Project: ctx.Project},
		ColorStyles: []outputHMIColor{{ID: "2", Name: "Фон мнемосхемы", Color: "14935011"}},
		PageMS:      []outputHMIPageMS{{ID: "3679", Info: "2//(AN_v1)/(8x_Diag_МФК1500_HART_AO)"}, {ID: "3655", Info: "2//(AI_DIAG16_AD3v1_kvit)/(8x_AO_4_DIAG)"}}}
	summary := xmlartifact.Summary{Graphics: plan.T11Count, Cards: plan.CardCount, SignalCount: plan.SignalCount, IOModuleCount: plan.FrameCount, FrameCount: plan.FrameCount,
		T11First: ids.T11Start, T11Last: ids.T11Start + int64(plan.T11Count) - 1, CardFirst: ids.CardStart, CardLast: ids.CardStart + int64(plan.CardCount) - 1,
		POUs: []xmlartifact.POUSummary{}, Frames: []xmlartifact.DiagnosticFrameSummary{}}
	cards := map[string]string{}
	nextT11 := ids.T11Start
	for index, frame := range plan.Frames {
		pageID := ids.PageStart + int64(index)
		id := strconv.FormatInt(pageID, 10)
		page := outputAODiagnosticPage{IDAttribute: id, ID: id, Name: frame.Name, TemplateID: "0", ForMarka: "0", Background: "536870913", DParams: "66",
			Height: "152", GridSize: "10", Width: "1200", Number: "0", PrintWidth: "600", PrintHeight: "800", PrintPageA4: "8", FrameNumber: "5", Srez: "1"}
		layer := outputAODiagnosticLayer{Number: "1", Visible: "1", Name: aoDiagnosticLayerName,
			Primitives: []outputAODiagnosticPrimitive{aoDiagnosticPrimitive(nextT11, 0, 0, 1200, 152, "3655", "0")}}
		nextT11++
		for channel, tag := range frame.Tags {
			key := strings.ToUpper(tag)
			card, exists := cards[key]
			if !exists {
				card = strconv.FormatInt(ids.CardStart+int64(len(cards)), 10)
				cards[key] = card
				doc.Cards = append(doc.Cards, outputHMICard{ID: card, Info: "2/" + plan.ControllerName + "/" + ctx.ResourceNumber + "/" + tag + "/(AN_v1)"})
			}
			layer.Primitives = append(layer.Primitives, aoDiagnosticPrimitive(nextT11, 70, 54+24*channel, 1080, 24, "3679", card))
			nextT11++
		}
		page.PageLayers = []outputAODiagnosticLayer{layer}
		doc.Pages = append(doc.Pages, page)
		summary.Frames = append(summary.Frames, xmlartifact.DiagnosticFrameSummary{ID: pageID, Name: frame.Name, Module: frame.Module})
	}
	data, err := xmlcodec.SerializeSCADAValue(doc)
	if err != nil {
		return xmlartifact.Result{}, fmt.Errorf("создать XML диагностики: %w", err)
	}
	if err := validateGeneratedAODiagnostic(data, doc); err != nil {
		return xmlartifact.Result{}, err
	}
	warnings := []string{"XML диагностики содержит все кадры выбранного ПЛК; импортируйте кадры в проект панели оператора, проверив ПЛК в привязках. Теги берутся из TXT, повторные каналы сохраняются.",
		"Кадры ссылаются на существующие объекты AN_v1 и мнемосимволы 8x_Diag_МФК1500_HART_AO / 8x_AO_4_DIAG. Определения мнемосимволов и экземпляры объектов этим XML не создаются."}
	return xmlartifact.Result{XML: data, BaseName: "AO_DIAG", Summary: summary, Warnings: warnings}, nil
}

// validateGeneratedAODiagnostic Повторно разбирает сохранённую форму XML диагностики AO.
// Сверяет страницы, карточки и ссылки с ожидаемой моделью без запуска SCADA.
func validateGeneratedAODiagnostic(data []byte, expected outputAODiagnosticDocument) error {
	var actual outputAODiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, xmlcodec.Utf8BOM), &actual); err != nil {
		return fmt.Errorf("некорректный XML диагностики: %w", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("структура XML диагностики изменилась при сериализации")
	}
	cards, t11s, pages := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, card := range actual.Cards {
		if card.ID == "0" || cards[card.ID] {
			return fmt.Errorf("диагностика: повторный или нулевой CardID")
		}
		cards[card.ID] = true
	}
	for _, page := range actual.Pages {
		if page.ID != page.IDAttribute || pages[page.ID] || len(page.PageLayers) != 1 || len(page.PageLayers[0].Primitives) != 5 {
			return fmt.Errorf("диагностика: нарушена структура кадра %s", page.Name)
		}
		pages[page.ID] = true
		for index, primitive := range page.PageLayers[0].Primitives {
			if t11s[primitive.T11ID] || index == 0 && (primitive.ObjectMSID != "3655" || primitive.CardID != "0") || index > 0 && (primitive.ObjectMSID != "3679" || !cards[primitive.CardID]) {
				return fmt.Errorf("диагностика: повторный SourceT11ID или неразрешённая ссылка в кадре %s", page.Name)
			}
			t11s[primitive.T11ID] = true
		}
	}
	return nil
}
