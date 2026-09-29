// Генератор AO ST сохраняет все физические присваивания, включая повторные теги и добавленные резервные модули.
package st

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	aomap "scheme-xml-generator/internal/domain/analogoutput"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/exportprofile"
	"scheme-xml-generator/internal/generator/identifiers"
	moduleid "scheme-xml-generator/internal/generator/modules"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"sort"
	"strconv"
	"strings"
)

// AOSTRequest selects POU groups and assigns one physical module ID per group
// of four outputs. A nil ID is an error; an explicitly supplied zero is valid.
type AOSTRequest struct {
	POUs []AOSTPOURequest `json:"pous"`
}

type AOSTPOURequest struct {
	GroupKey    string   `json:"groupKey"`
	ModuleCount int      `json:"moduleCount"`
	ModuleIDs   []*int64 `json:"moduleIds"`
}

// AOSTPlan is a validated, independent snapshot for a single controller.
type AOSTPlan struct {
	ControllerName          string
	POUs                    []AOSTPOU
	ModuleCount             int
	AssignmentCount         int
	RepeatedAssignmentCount int
}

type AOSTPOU struct {
	GroupKey string
	Name     string
	Modules  []AOSTModule
}

type AOSTModule struct {
	Name     string
	ID       int64
	Channels []aomap.Channel
}

// PrepareAOSTPlans validates the entire request before any IDs are allocated.
// Unlike the FBD profile, it deliberately preserves repeated tags: both physical
// outputs must receive the shared object's OUT value. Source data is not edited.
func PrepareAOSTPlans(source *aomap.Plan, request AOSTRequest) ([]AOSTPlan, error) {
	if source == nil || len(source.Groups) == 0 || len(request.POUs) == 0 || len(request.POUs) > 128 {
		return nil, fmt.Errorf("для ST необходимо выбрать от 1 до 128 POU из карты AO")
	}
	available := make(map[string]aomap.Group, len(source.Groups))
	for _, group := range source.Groups {
		if _, exists := available[group.Key]; exists || group.Key == "" {
			return nil, fmt.Errorf("карта AO содержит повторный или пустой ключ POU")
		}
		available[group.Key] = group
	}
	selected := make(map[string]AOSTPOURequest, len(request.POUs))
	totalModules := 0
	for _, pou := range request.POUs {
		group, exists := available[pou.GroupKey]
		if !exists {
			return nil, fmt.Errorf("ST: неизвестная POU %q", pou.GroupKey)
		}
		if _, duplicate := selected[pou.GroupKey]; duplicate {
			return nil, fmt.Errorf("ST: POU %s указана повторно", pou.GroupKey)
		}
		if pou.ModuleCount < 1 || pou.ModuleCount < len(group.Modules) || pou.ModuleCount > 4096 || pou.ModuleCount != len(pou.ModuleIDs) {
			return nil, fmt.Errorf("ST POU %s: задайте число модулей не меньше %d и ровно столько же ID (предел 4096)", pou.GroupKey, len(group.Modules))
		}
		totalModules += pou.ModuleCount
		if totalModules > 4096 {
			return nil, fmt.Errorf("ST: допускается не более 4096 физических модулей во всех выбранных POU")
		}
		selected[pou.GroupKey] = pou
	}
	var plans []AOSTPlan
	byController := map[string]int{}
	for _, group := range source.Groups {
		selection, exists := selected[group.Key]
		if !exists {
			continue
		}
		if !identifiers.ControllerNamePattern.MatchString(group.ControllerName) || !moduleid.RackPrefixPattern.MatchString(group.Prefix) || len(group.Modules) == 0 {
			return nil, fmt.Errorf("ST POU %s: неверный FCS, префикс группы или пустая карта модулей", group.Key)
		}
		pou := AOSTPOU{GroupKey: group.Key, Name: "AO_" + group.Prefix + "_channels"}
		modules := append([]aomap.Module(nil), group.Modules...)
		moduleIndices := make(map[string]int, len(modules))
		usedIndices := map[int]bool{}
		maxIndex := -1
		for _, module := range modules {
			index, err := moduleid.AoSTModuleIndex(module.Name, group.Prefix)
			if err != nil || usedIndices[index] || module.ObjectType != "AN_v1" {
				return nil, fmt.Errorf("ST POU %s: неверный или повторный модуль %s (ожидается AN_v1)", group.Key, module.Name)
			}
			usedIndices[index] = true
			moduleIndices[module.Name] = index
			maxIndex = max(maxIndex, index)
		}
		sort.Slice(modules, func(i, j int) bool { return moduleIndices[modules[i].Name] < moduleIndices[modules[j].Name] })
		for index := 0; index < selection.ModuleCount; index++ {
			id := selection.ModuleIDs[index]
			if id == nil || *id < 0 || *id > contracts.MaxTransportID {
				return nil, fmt.Errorf("ST POU %s, модуль %d: ID должен быть целым числом 0..%d", group.Key, index+1, contracts.MaxTransportID)
			}
			var module AOSTModule
			if index < len(modules) {
				module = AOSTModule{Name: modules[index].Name, ID: *id, Channels: append([]aomap.Channel(nil), modules[index].Channels...)}
			} else {
				moduleIndex := int64(maxIndex) + int64(index-len(modules)) + 1
				if moduleIndex > contracts.MaxTransportID {
					return nil, fmt.Errorf("ST POU %s: номер дополнительного модуля выходит за signed 32-bit", group.Key)
				}
				module = AOSTModule{Name: fmt.Sprintf("%s_%02d", group.Prefix, moduleIndex), ID: *id}
				for channel := 0; channel < hardware.AOC4HChannels; channel++ {
					module.Channels = append(module.Channels, aomap.Channel{Channel: channel,
						Tag: fmt.Sprintf("_%s_%s_%d", group.ControllerName, module.Name, channel), Reserve: true, Min: "0.0", Max: "100.0"})
				}
			}
			pou.Modules = append(pou.Modules, module)
		}
		key := strings.ToUpper(group.ControllerName)
		planIndex, exists := byController[key]
		if !exists {
			planIndex = len(plans)
			byController[key] = planIndex
			plans = append(plans, AOSTPlan{ControllerName: group.ControllerName})
		}
		plans[planIndex].POUs = append(plans[planIndex].POUs, pou)
	}
	for index := range plans {
		if err := validateAOSTPlan(&plans[index]); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

// validateAOSTPlan Проверяет физические AO-модули и назначения до создания ST-кода.
// Отклоняет недопустимые или конфликтующие данные подготовленной перекладки.
func validateAOSTPlan(plan *AOSTPlan) error {
	if !identifiers.ControllerNamePattern.MatchString(plan.ControllerName) || len(plan.POUs) == 0 || len(plan.POUs) > 128 {
		return fmt.Errorf("ST: не задан FCS или неверное число POU")
	}
	plan.ModuleCount, plan.AssignmentCount, plan.RepeatedAssignmentCount = 0, 0, 0
	ids := map[int64]string{}
	names := map[string]bool{}
	tags := map[string]bool{}
	for _, pou := range plan.POUs {
		if err := identifiers.ValidatePOUName(pou.Name); err != nil {
			return err
		}
		if !strings.HasPrefix(pou.Name, "AO_") || !strings.HasSuffix(pou.Name, "_channels") || len(pou.Modules) == 0 || names[strings.ToUpper(pou.Name)] {
			return fmt.Errorf("ST: неверная, пустая или повторная POU %s", pou.Name)
		}
		names[strings.ToUpper(pou.Name)] = true
		moduleNames := map[string]bool{}
		for _, module := range pou.Modules {
			if module.ID < 0 || module.ID > contracts.MaxTransportID {
				return fmt.Errorf("ST POU %s, модуль %s: ID должен быть в диапазоне 0..%d", pou.Name, module.Name, contracts.MaxTransportID)
			}
			if owner, exists := ids[module.ID]; exists {
				return fmt.Errorf("ST FCS %s: ID физического модуля %d повторяется в %s и %s/%s", plan.ControllerName, module.ID, owner, pou.Name, module.Name)
			}
			ids[module.ID] = pou.Name + "/" + module.Name
			if module.Name == "" || moduleNames[strings.ToUpper(module.Name)] || len(module.Channels) != hardware.AOC4HChannels {
				return fmt.Errorf("ST POU %s: модуль %s должен быть уникальным и иметь четыре канала", pou.Name, module.Name)
			}
			moduleNames[strings.ToUpper(module.Name)] = true
			plan.ModuleCount++
			if plan.ModuleCount > 4096 {
				return fmt.Errorf("ST: допускается не более 4096 физических модулей")
			}
			for index, channel := range module.Channels {
				if channel.Channel != index || !identifiers.ObjectNamePattern.MatchString(channel.Tag) {
					return fmt.Errorf("ST POU %s, модуль %s: неверный тег или последовательность каналов 0..%d", pou.Name, module.Name, hardware.AOC4HChannels-1)
				}
				for _, limit := range []string{channel.Min, channel.Max} {
					value, err := strconv.ParseFloat(limit, 64)
					if !identifiers.NumberPattern.MatchString(limit) || err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
						return fmt.Errorf("ST POU %s, модуль %s, канал %d: неверный числовой диапазон REAL_TO_DINT", pou.Name, module.Name, index)
					}
				}
				low, _ := strconv.ParseFloat(channel.Min, 64)
				high, _ := strconv.ParseFloat(channel.Max, 64)
				if low >= high {
					return fmt.Errorf("ST POU %s, модуль %s, канал %d: min должен быть меньше max", pou.Name, module.Name, index)
				}
				plan.AssignmentCount++
				key := strings.ToUpper(channel.Tag)
				if tags[key] {
					plan.RepeatedAssignmentCount++
				}
				tags[key] = true
			}
		}
	}
	return nil
}

// GenerateAOST emits the native ST-only XML profile. No object instances,
// graphical blocks, or card IDs are created by these physical assignments.
func (g Generator) GenerateAOST(plan AOSTPlan, ctx contracts.ProgramContext, ids contracts.IDRange) (contracts.Result, error) {
	if err := validateAOSTPlan(&plan); err != nil {
		return contracts.Result{}, err
	}
	ctx, err := addressing.NormalizeProgramContext(ctx, len(plan.POUs))
	if err != nil {
		return contracts.Result{}, err
	}
	if err := exportprofile.ValidateAOPhysicalST(ctx.ControllerTypeName); err != nil {
		return contracts.Result{}, err
	}
	if ctx.PhysicalProfile != "" && ctx.PhysicalProfile != addressing.PhysicalProfileLegacy {
		return contracts.Result{}, fmt.Errorf("AO ST поддерживает только физический профиль %s", addressing.PhysicalProfileLegacy)
	}
	if ids.POUID < 1 || ids.POUID > contracts.MaxTransportID-int64(len(plan.POUs))+1 {
		return contracts.Result{}, fmt.Errorf("диапазон ST POU ID выходит за signed 32-bit")
	}
	doc := xmlmodel.OutputSTDocument{Common: xmlmodel.OutputCommon{Version: ctx.Version, Project: ctx.Project, IsCut: "false", IsFFB: "false", ControllerType: ctx.ControllerTypeName, ControllerID: ctx.ControllerID, ResourceID: ctx.ResourceID}}
	summary := contracts.Summary{POUCount: len(plan.POUs), IOModuleCount: plan.ModuleCount, SignalCount: plan.AssignmentCount,
		POUID: ids.POUID, POUName: plan.POUs[0].Name, POUGroupID: ctx.GroupID, POUNumber: ctx.POUNumber, POUs: []contracts.POUSummary{}}
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	for index, pou := range plan.POUs {
		id := ids.POUID + int64(index)
		number := strconv.FormatInt(firstNumber+int64(index), 10)
		var code strings.Builder
		fmt.Fprintf(&code, "PROGRAM %s\n\n", pou.Name)
		ps := contracts.POUSummary{POUID: id, POUName: pou.Name, POUGroupID: ctx.GroupID, POUNumber: number, Signals: []contracts.SignalSummary{}}
		for _, module := range pou.Modules {
			moduleID := module.ID
			ps.IOModules = append(ps.IOModules, contracts.IOModuleSummary{Type: "AO", ID: module.ID, BindingPrefix: fmt.Sprintf("_IO_QU%d", module.ID), InstanceName: module.Name, Capacity: hardware.AOC4HChannels, SignalCount: hardware.AOC4HChannels})
			for _, channel := range module.Channels {
				fmt.Fprintf(&code, "_IO_QU%d_%d.ValueDINT := REAL_TO_DINT(%s.OUT, %s, %s);\n", module.ID, channel.Channel, channel.Tag, channel.Min, channel.Max)
				channelNumber := channel.Channel
				ps.Signals = append(ps.Signals, contracts.SignalSummary{TemplateKey: "temporary:AO_ST", BaseName: channel.Tag, IOType: "AO", ModuleID: &moduleID, Channel: &channelNumber})
			}
			code.WriteByte('\n')
		}
		code.WriteString("END_PROGRAM")
		doc.POUS.Items = append(doc.POUS.Items, xmlmodel.OutputSTPOU{ID: strconv.FormatInt(id, 10), Name: pou.Name, IsFBD: "0", GroupID: ctx.GroupID, Enabled: "1", Number: number, Code: code.String()})
		summary.POUs = append(summary.POUs, ps)
	}
	data, err := xmlcodec.SerializeSCADAValue(doc)
	if err != nil {
		return contracts.Result{}, fmt.Errorf("создать ST XML: %w", err)
	}
	if err := validateGeneratedAOST(data, doc, plan.AssignmentCount); err != nil {
		return contracts.Result{}, err
	}
	warnings := []string{"ST XML содержит POU одного FCS; импортируйте его в соответствующий ПЛК. Повторные теги сохранены для всех физических выходов; экземпляры AN_v1 должны существовать в проекте."}
	return contracts.Result{XML: data, BaseName: "AO_ST", Summary: summary, Warnings: warnings}, nil
}

// Native ST exports have only Common and POUS, and each OnePOU has only STCODE.
// They cannot use the FBD validator, which intentionally requires graph sections.
func validateGeneratedAOST(data []byte, expected xmlmodel.OutputSTDocument, assignments int) error {
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, xmlcodec.Utf8BOM)))
	var stack []string
	counts := map[string]int{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("некорректный ST XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) != 0 {
				parent = stack[len(stack)-1]
			}
			allowed := parent == "" && value.Name.Local == "BufScadaPOUS" || parent == "BufScadaPOUS" && (value.Name.Local == "Common" || value.Name.Local == "POUS") || parent == "POUS" && value.Name.Local == "OnePOU" || parent == "OnePOU" && value.Name.Local == "STCODE"
			if !allowed || value.Name.Space != "" {
				return fmt.Errorf("неожиданный элемент ST XML: %s", value.Name.Local)
			}
			counts[value.Name.Local]++
			stack = append(stack, value.Name.Local)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if (len(stack) == 0 || stack[len(stack)-1] != "STCODE") && strings.TrimSpace(string(value)) != "" {
				return fmt.Errorf("неожиданный текст вне STCODE")
			}
		}
	}
	if counts["BufScadaPOUS"] != 1 || counts["Common"] != 1 || counts["POUS"] != 1 || counts["OnePOU"] != len(expected.POUS.Items) || counts["STCODE"] != len(expected.POUS.Items) {
		return fmt.Errorf("неверная структура ST XML")
	}
	var actual xmlmodel.OutputSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, xmlcodec.Utf8BOM), &actual); err != nil {
		return err
	}
	if actual.Common != expected.Common || actual.Common.IsCut != "false" || actual.Common.IsFFB != "false" {
		return fmt.Errorf("изменён контекст ST XML")
	}
	count := 0
	for index, pou := range actual.POUS.Items {
		if pou != expected.POUS.Items[index] || pou.IsFBD != "0" || pou.Enabled != "1" {
			return fmt.Errorf("неверная ST POU %s", pou.Name)
		}
		count += strings.Count(pou.Code, ".ValueDINT := REAL_TO_DINT(")
	}
	if count != assignments {
		return fmt.Errorf("неверное число присваиваний ST: %d вместо %d", count, assignments)
	}
	return nil
}
