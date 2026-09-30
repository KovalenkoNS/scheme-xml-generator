// Библиотечная сборка DO извлекает BOOL, D32 и QUAL_STAT из загруженных источников и создаёт один D32 на модуль.
package fbd

import (
	"fmt"
	"reflect"
	"scheme-xml-generator/internal/domain/assignments"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/allocation"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/generator/identifiers"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	moduleid "scheme-xml-generator/internal/generator/modules"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
	"sort"
	"strconv"
	"strings"
)

// LibraryDORequest assembles each physical DO module once. Library templates
// supply logical types; the evidenced CPU850 driver supplies Quality metadata.
type LibraryDORequest struct {
	TemplateKey     string                        `json:"templateKey"`
	PLCName         string                        `json:"plcName"`
	Context         *fbdrequest.GenerationContext `json:"context,omitempty"`
	PhysicalProfile string                        `json:"physicalProfile,omitempty"`
	FileName        string                        `json:"fileName,omitempty"`
	POUs            []LibraryDOPOURequest         `json:"pous"`
}

type LibraryDOPOURequest struct {
	Name      string                   `json:"name"`
	GroupID   *int64                   `json:"groupId,omitempty"`
	POUNumber *int64                   `json:"pouNumber,omitempty"`
	Modules   []LibraryDOModuleRequest `json:"modules"`
}

type LibraryDOModuleRequest struct {
	Name     string                    `json:"name"`
	ID       *int64                    `json:"id"`
	Channels []LibraryDOChannelRequest `json:"channels"`
}

type LibraryDOChannelRequest struct {
	Channel int    `json:"channel"`
	Tag     string `json:"tag"`
	Invert  bool   `json:"invert,omitempty"`
}

// LibraryDOPlan is an immutable validated snapshot; callers can inspect counts
// without exposing mutable library pointers or a partially prepared graph.
type LibraryDOPlan struct {
	plan         stassignment.ControllerPlan
	context      programcontext.ProgramContext
	profile      *d32LibraryProfile
	requirements xmlidentity.DocumentRequirements
}

// Requirements вызывается HTTP-слоем перед резервированием диапазона allocator.
// Возвращает копию рассчитанных количеств объектов, карточек и POU без записи состояния.
func (p *LibraryDOPlan) Requirements() xmlidentity.DocumentRequirements { return p.requirements }

type d32LibraryProfile struct {
	templateKey                   string
	digital, signal, quality      library.Primitive
	digitalInitial, signalInitial *string
	inverted                      map[int64]map[int]bool
	groups, numbers               []string
}

// cloneInitial переносит IV карточки библиотеки в независимый план генерации.
// Сохраняет различие отсутствующего значения и пустой строки, не разделяя изменяемый указатель.
func cloneInitial(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

// libraryDOProfile извлекает образцы BOOL/D32 и QUAL_STAT при подготовке модульного DO.
// Читает выбранный TemplateRef и ту же библиотеку; возвращает независимые метаданные
// либо ошибку отсутствия/противоречия, не подставляя встроенные ISA-номера.
func libraryDOProfile(ref *library.TemplateRef) (*d32LibraryProfile, error) {
	if ref == nil || ref.Library == nil || ref.Template == nil || ref.Owner == nil || ref.Library.Document == nil {
		return nil, fmt.Errorf("DO: библиотечный шаблон не разрешён")
	}
	if _, err := invertD32Template(ref); err != nil {
		return nil, fmt.Errorf("DO: %w", err)
	}
	cards := sourceCards(ref)
	profile := &d32LibraryProfile{templateKey: ref.Key, inverted: map[int64]map[int]bool{}}
	for _, item := range ref.Template.Contents.Primitives {
		if item.ObjectType == "37" && item.TypeName == "D32V_v1" {
			if profile.digital.ID != "" {
				return nil, fmt.Errorf("DO: выберите шаблон ровно с одним D32V_v1")
			}
			profile.digital = item
			profile.digitalInitial = cloneInitial(cards[item.CardID].InitialValue)
		}
		if item.ObjectType == "31" && cards[item.CardID].TypeName == "BOOL" && strings.TrimSpace(library.ParseParams(item.Params)["TEXT"]) == "" {
			if profile.signal.ID != "" {
				return nil, fmt.Errorf("DO: шаблон должен иметь один образец BOOL-сигнала")
			}
			profile.signal = item
			profile.signalInitial = cloneInitial(cards[item.CardID].InitialValue)
		}
	}
	if profile.digital.ID == "" || profile.signal.ID == "" {
		return nil, fmt.Errorf("DO: библиотека не содержит связанный образец BOOL и D32V_v1")
	}
	if library.Int(profile.digital.ISAObjectID, 0) <= 0 || strings.TrimSpace(profile.digital.LibraryName) == "" {
		return nil, fmt.Errorf("DO: библиотека не задаёт ISA ID и имя библиотеки D32")
	}
	if library.Int(profile.signal.ISAObjectID, 0) > 0 && (profile.signal.TypeName == "" || profile.signal.LibraryName == "") {
		return nil, fmt.Errorf("DO: неполное определение типа BOOL-сигнала в библиотеке")
	}
	for _, primitive := range []library.Primitive{profile.digital, profile.signal} {
		if _, err := strconv.ParseInt(primitive.ISAObjectID, 10, 32); err != nil {
			return nil, fmt.Errorf("DO: некорректный ISA ID библиотечного блока %s", primitive.TypeName)
		}
	}
	for _, required := range []struct {
		p      library.Primitive
		ci, co int
	}{{profile.digital, 34, 33}, {profile.signal, 1, 1}} {
		params := library.ParseParams(required.p.Params)
		if library.Int(params["CI"], -1) != required.ci || library.Int(params["CO"], -1) != required.co {
			return nil, fmt.Errorf("DO: неподтверждённая сигнатура библиотечного блока %s", required.p.TypeName)
		}
	}
	var conflict bool
	var walk func([]library.ObjectType)
	walk = func(objects []library.ObjectType) {
		for _, owner := range objects {
			for _, template := range owner.Templates.Items {
				for _, item := range template.Contents.Primitives {
					if item.ObjectType != "36" || item.TypeName != "QUAL_STAT" {
						continue
					}
					params := library.ParseParams(item.Params)
					if library.Int(item.ISAObjectID, 0) <= 0 || item.LibraryName == "" || library.Int(params["CI"], -1) != 1 || library.Int(params["CO"], -1) != 1 || strings.TrimSpace(params["TEXT"]) != "" || (item.CardID != "" && item.CardID != "0") {
						conflict = true
						continue
					}
					if profile.quality.ID != "" && (item.ISAObjectID != profile.quality.ISAObjectID || item.LibraryName != profile.quality.LibraryName) {
						conflict = true
					}
					profile.quality = item
				}
			}
			walk(owner.Child.ObjectTypes)
		}
	}
	for _, section := range ref.Library.Document.Sections {
		walk(section.Other.ObjectTypes)
	}
	if conflict {
		return nil, fmt.Errorf("DO: библиотека содержит противоречивые определения QUAL_STAT")
	}
	if profile.quality.ID == "" {
		return nil, fmt.Errorf("DO: подключённая библиотека не содержит QUAL_STAT для диагностики модуля")
	}
	if profile.quality.ISAObjectID == profile.digital.ISAObjectID {
		return nil, fmt.Errorf("DO: D32 и QUAL_STAT имеют один ISA ID")
	}
	if _, err := strconv.ParseInt(profile.quality.ISAObjectID, 10, 32); err != nil {
		return nil, fmt.Errorf("DO: ISA ID QUAL_STAT выходит за signed 32-bit")
	}
	if library.Int(profile.signal.ISAObjectID, 0) > 0 && (profile.signal.ISAObjectID == profile.digital.ISAObjectID || profile.signal.ISAObjectID == profile.quality.ISAObjectID) {
		return nil, fmt.Errorf("DO: тип BOOL конфликтует с D32 или QUAL_STAT")
	}
	return profile, nil
}

// PrepareLibraryDO вызывается до allocator для JSON-запроса модульной DO-генерации.
// Сверяет библиотеку, аппаратный профиль, ПЛК/модули/каналы и контекст; возвращает
// снимок графа и точные расходы ID без изменения библиотеки, конфигурации или файлов.
func (g Generator) PrepareLibraryDO(ref *library.TemplateRef, request LibraryDORequest) (*LibraryDOPlan, error) {
	profile, err := libraryDOProfile(ref)
	if err != nil {
		return nil, err
	}
	g, err = g.withGenerationContext(request.Context)
	if err != nil {
		return nil, err
	}
	physical := strings.TrimSpace(request.PhysicalProfile)
	if physical == "" {
		physical = addressing.PhysicalProfileMeasurement
	}
	if g.Config.Common.ControllerType != cpuprofile.ControllerCPU850 || physical != addressing.PhysicalProfileMeasurement {
		return nil, fmt.Errorf("DO с диагностикой Quality: подтверждён только %s с профилем %s", cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement)
	}
	if !identifiers.ControllerNamePattern.MatchString(request.PLCName) || len(request.POUs) == 0 || len(request.POUs) > 128 {
		return nil, fmt.Errorf("DO: укажите имя ПЛК и от 1 до 128 POU")
	}
	ctx := programcontext.ProgramContext{Version: g.Config.Common.Version, Project: g.Config.Common.Project, ControllerTypeName: g.Config.Common.ControllerType, ControllerID: g.Config.Common.ControllerID, ResourceID: g.Config.Common.ResourceID, GroupID: g.Config.Page.GroupID, POUNumber: g.Config.Page.POUNumber, PhysicalProfile: physical}
	ctx, err = addressing.NormalizeProgramContext(ctx, len(request.POUs))
	if err != nil {
		return nil, err
	}
	plan := stassignment.ControllerPlan{ControllerName: request.PLCName, Kind: "fbd", POUs: make([]stassignment.ModuleGroup, 0, len(request.POUs))}
	req := xmlidentity.DocumentRequirements{POUCount: len(request.POUs)}
	cards, cardRoles := map[string]bool{}, map[string]string{}
	reserveCard := func(tag, role string) error {
		key := strings.ToUpper(tag)
		if previous := cardRoles[key]; previous != "" && previous != role {
			return fmt.Errorf("DO: имя %s используется для разных видов карточек", tag)
		}
		cardRoles[key] = role
		cards[key] = true
		return nil
	}
	moduleIDs, moduleNames, pouNames, pouNumbers := map[int64]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	baseNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 64)
	for pouIndex, source := range request.POUs {
		if err := identifiers.ValidatePOUName(source.Name); err != nil {
			return nil, err
		}
		if pouNames[strings.ToUpper(source.Name)] {
			return nil, fmt.Errorf("DO: повторное имя POU %s", source.Name)
		}
		pouNames[strings.ToUpper(source.Name)] = true
		group, number := ctx.GroupID, strconv.FormatInt(baseNumber+int64(pouIndex), 10)
		if source.GroupID != nil {
			group = strconv.FormatInt(*source.GroupID, 10)
		}
		if source.POUNumber != nil {
			number = strconv.FormatInt(*source.POUNumber, 10)
		}
		for _, value := range []string{group, number} {
			if _, err := addressing.NormalizeContextInteger(value, "POU"); err != nil {
				return nil, err
			}
		}
		if pouNumbers[number] {
			return nil, fmt.Errorf("DO: повторный POUNum %s", number)
		}
		pouNumbers[number] = true
		profile.groups = append(profile.groups, group)
		profile.numbers = append(profile.numbers, number)
		if len(source.Modules) == 0 || len(source.Modules) > 128 {
			return nil, fmt.Errorf("DO: POU %s должна содержать от 1 до 128 модулей", source.Name)
		}
		pou := stassignment.ModuleGroup{Name: source.Name, Kind: "DO", Modules: make([]stassignment.PhysicalModule, 0, len(source.Modules))}
		for _, sourceModule := range source.Modules {
			if sourceModule.ID == nil || *sourceModule.ID < 0 || *sourceModule.ID > xmlidentity.MaxTransportID {
				return nil, fmt.Errorf("DO: укажите физический ModuleID 0…%d для %s", xmlidentity.MaxTransportID, sourceModule.Name)
			}
			moduleID := *sourceModule.ID
			moduleTag := moduleid.ModuleInstanceTag(request.PLCName, sourceModule.Name)
			if identifiers.ValidateIdentifier(moduleTag) != nil || len(sourceModule.Name) == 0 || moduleIDs[moduleID] || moduleNames[strings.ToUpper(moduleTag)] {
				return nil, fmt.Errorf("DO: неверное или повторное имя/ID модуля %s", sourceModule.Name)
			}
			moduleIDs[moduleID] = true
			moduleNames[strings.ToUpper(moduleTag)] = true
			if err := reserveCard(moduleTag, "module"); err != nil {
				return nil, err
			}
			if err := reserveCard(addressing.DODiagnosticTag(moduleID), "physical"); err != nil {
				return nil, err
			}
			module := stassignment.PhysicalModule{Name: sourceModule.Name, Type: "DO32P", ObjectType: "D32V", Capacity: hardware.ChannelCount("DO32P"), ID: &moduleID, Channels: make([]assignments.Channel, 0, len(sourceModule.Channels))}
			if len(sourceModule.Channels) > 32 {
				return nil, fmt.Errorf("DO: у модуля %s более 32 каналов", sourceModule.Name)
			}
			channels := map[int]bool{}
			profile.inverted[moduleID] = map[int]bool{}
			for _, channel := range sourceModule.Channels {
				if channel.Channel < 0 || channel.Channel > 31 || channels[channel.Channel] || identifiers.ValidateIdentifier(channel.Tag) != nil {
					return nil, fmt.Errorf("DO: неверный или повторный канал/тег модуля %s", sourceModule.Name)
				}
				channels[channel.Channel] = true
				if err := reserveCard(channel.Tag, "signal"); err != nil {
					return nil, err
				}
				profile.inverted[moduleID][channel.Channel] = channel.Invert
				module.Channels = append(module.Channels, assignments.Channel{Channel: channel.Channel, Tag: channel.Tag, SourceRow: 1})
				req.T11Count += 2
				if channel.Invert {
					req.T11Count += 2
				}
				req.SignalCount++
			}
			sort.Slice(module.Channels, func(i, j int) bool { return module.Channels[i].Channel < module.Channels[j].Channel })
			pou.Modules = append(pou.Modules, module)
			plan.ModuleCount++
			req.T11Count += 5
			if plan.ModuleCount > 128 || req.SignalCount > 4096 {
				return nil, fmt.Errorf("DO: допустимо не более 128 модулей и 4096 сигналов")
			}
		}
		plan.POUs = append(plan.POUs, pou)
	}
	req.CardCount = len(cards)
	plan.SignalCount = req.SignalCount
	return &LibraryDOPlan{plan: plan, context: ctx, profile: profile, requirements: req}, nil
}

// GenerateLibraryDO выполняет уже проверенный план внутри транзакции резервирования ID.
// Передаёт библиотечные метаданные общему модульному renderer и возвращает XML/сводку;
// сохранение файлов и продвижение allocator остаются у вызывающего HTTP-слоя.
func (g Generator) GenerateLibraryDO(plan *LibraryDOPlan, ids xmlidentity.IDRange) (xmlartifact.Result, error) {
	if plan == nil || plan.profile == nil {
		return xmlartifact.Result{}, fmt.Errorf("DO: план не подготовлен")
	}
	if err := allocation.ValidateDocumentRanges(ids, plan.requirements); err != nil {
		return xmlartifact.Result{}, err
	}
	return generateD32ModuleFBD(plan.plan, plan.context, ids, plan.requirements, plan.profile)
}

// libraryDOTypeRecords формирует словарь ISAOBJSINFO для renderer и его валидатора.
// Сохраняет ISA-номера/имена подключённой библиотеки; встроенные операторы сюда не добавляются.
func libraryDOTypeRecords(profile *d32LibraryProfile) []xmlmodel.OutputISAObject {
	items := []xmlmodel.OutputISAObject{{ID: profile.digital.ISAObjectID, Info: profile.digital.TypeName, LibraryName: profile.digital.LibraryName}, {ID: profile.quality.ISAObjectID, Info: profile.quality.TypeName, LibraryName: profile.quality.LibraryName}}
	if library.Int(profile.signal.ISAObjectID, 0) > 0 {
		items = append(items, xmlmodel.OutputISAObject{ID: profile.signal.ISAObjectID, Info: profile.signal.TypeName, LibraryName: profile.signal.LibraryName})
	}
	return items
}

// sameInitial применяется валидатором при сравнении сериализованного IV с библиотекой.
// Сравнивает и наличие элемента, и значение; nil не равен указателю на пустую строку.
func sameInitial(left, right *string) bool { return reflect.DeepEqual(left, right) }
