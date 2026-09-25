package generator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"scheme-xml-generator/internal/skzmap"
)

// SKZRequest separates the executable object calls from physical ST mapping.
// ModuleIDs are required only for ST and correspond to the preview's modules.
type SKZRequest struct {
	Kind string          `json:"kind"`
	POUs []SKZPOURequest `json:"pous"`
}

type SKZPOURequest struct {
	GroupKey    string   `json:"groupKey"`
	ModuleCount *int     `json:"moduleCount,omitempty"`
	ModuleIDs   []*int64 `json:"moduleIds"`
}

type SKZPlan struct {
	SCS                     string
	Kind                    string
	POUs                    []SKZPOU
	ModuleCount             int
	SignalCount             int
	AssignmentCount         int
	RepeatedAssignmentCount int
	Warnings                []string
}

type SKZPOU struct {
	GroupKey string
	Name     string
	Kind     string
	Prefix   string
	Modules  []SKZModule
}

type SKZModule struct {
	Name       string
	Type       string
	ObjectType string
	Capacity   int
	ID         *int64
	Channels   []skzmap.Channel
}

var skzModulePattern = regexp.MustCompile(`^(A[0-9]{1,6})-([0-9]{2,4})$`)

func skzModuleSlot(name, prefix string) (int, bool) {
	parts := skzModulePattern.FindStringSubmatch(name)
	if len(parts) != 3 || parts[1] != prefix {
		return 0, false
	}
	slot, err := strconv.Atoi(parts[2])
	return slot, err == nil && slot <= 4095 && name == fmt.Sprintf("%s-%02d", prefix, slot)
}

// Values are transport context from SOGO_AI.xml / SOGO_DO.xml, not hardware IDs.
func DefaultSKZContext() AOMappingContext {
	ctx := DefaultAOMappingContext()
	ctx.ControllerTypeName, ctx.ControllerID, ctx.ResourceID = "TENIX-CPU850", "189311", "647"
	ctx.GroupID, ctx.POUNumber = "19912", "4"
	return ctx
}

func PrepareSKZPlans(source *skzmap.Plan, request SKZRequest) ([]SKZPlan, error) {
	if source == nil || len(source.Groups) == 0 || len(source.Groups) > 128 || len(request.POUs) == 0 || len(request.POUs) > 128 || request.Kind != "st" && request.Kind != "fbd" {
		return nil, fmt.Errorf("СКЗ: выберите режим ST/FBD и от 1 до 128 POU")
	}
	available := map[string]bool{}
	sourceModules := map[string]bool{}
	sourceTags := map[string]bool{}
	for _, group := range source.Groups {
		if group.Key == "" || available[group.Key] {
			return nil, fmt.Errorf("СКЗ: пустой или повторный ключ POU")
		}
		available[group.Key] = true
		for _, module := range group.Modules {
			sourceModules[strings.ToUpper(group.SCS)+"|"+module.Name] = true
			for _, channel := range module.Channels {
				sourceTags[strings.ToUpper(group.SCS+"|"+channel.Tag)] = true
			}
		}
	}
	selected := map[string]SKZPOURequest{}
	for _, choice := range request.POUs {
		if _, exists := selected[choice.GroupKey]; exists || !available[choice.GroupKey] {
			return nil, fmt.Errorf("СКЗ: неизвестная или повторно выбранная POU %q", choice.GroupKey)
		}
		selected[choice.GroupKey] = choice
	}
	var plans []SKZPlan
	controllers := map[string]int{}
	totalModules, totalSignals, totalPOUs := 0, 0, 0
	for _, group := range source.Groups {
		choice, ok := selected[group.Key]
		if !ok {
			continue
		}
		moduleCount := len(group.Modules)
		if choice.ModuleCount != nil {
			moduleCount = *choice.ModuleCount
		}
		if len(group.Modules) == 0 || moduleCount < 1 || moduleCount < len(group.Modules) || moduleCount > 4096 {
			return nil, fmt.Errorf("СКЗ %s: число модулей должно быть от %d до 4096 и сохранять все модули Excel", group.Key, max(1, len(group.Modules)))
		}
		capacity, moduleType, objectType := 16, "AI16H", "AD3_v2"
		if group.Kind == "DO" {
			capacity, moduleType, objectType = 32, "DO32P", "D32V"
		} else if group.Kind != "AI" {
			return nil, fmt.Errorf("СКЗ %s: неизвестный тип модулей %q", group.Key, group.Kind)
		}
		maxSlot := -1
		for _, module := range group.Modules {
			slot, valid := skzModuleSlot(module.Name, group.Prefix)
			if !valid {
				return nil, fmt.Errorf("СКЗ %s: неверное имя модуля %q", group.Key, module.Name)
			}
			maxSlot = max(maxSlot, slot)
			totalSignals += len(module.Channels)
		}
		added := moduleCount - len(group.Modules)
		if maxSlot+added > 4095 {
			return nil, fmt.Errorf("СКЗ %s: номер добавленного модуля превышает 4095", group.Key)
		}
		totalModules += moduleCount
		totalSignals += added * capacity
		if totalModules > 4096 || totalSignals > 4096 {
			return nil, fmt.Errorf("СКЗ: не более 4096 модулей и 4096 каналов за одну генерацию")
		}
		pouCount := 1
		if request.Kind == "fbd" && group.Kind == "AI" {
			pouCount = moduleCount
		}
		totalPOUs += pouCount
		if totalPOUs > 128 {
			return nil, fmt.Errorf("СКЗ: после разделения модулей AI FBD не более 128 POU за одну генерацию")
		}
		if request.Kind == "st" && len(choice.ModuleIDs) != moduleCount {
			return nil, fmt.Errorf("СКЗ %s: ST требует по одному ID на модуль", group.Key)
		}
		// The UI may retain ST settings while switching to FBD. Hardware IDs
		// are intentionally neither read nor carried into an FBD plan.
		pou := SKZPOU{GroupKey: group.Key, Name: group.POUName, Kind: group.Kind, Prefix: group.Prefix}
		if request.Kind == "st" {
			pou.Name += "_channels"
		}
		for _, module := range group.Modules {
			copyModule := SKZModule{Name: module.Name, Type: module.Type, ObjectType: module.ObjectType, Capacity: module.Capacity, Channels: append([]skzmap.Channel(nil), module.Channels...)}
			pou.Modules = append(pou.Modules, copyModule)
		}
		for extra := 1; extra <= added; extra++ {
			name := fmt.Sprintf("%s-%02d", group.Prefix, maxSlot+extra)
			if sourceModules[strings.ToUpper(group.SCS)+"|"+name] {
				return nil, fmt.Errorf("СКЗ %s: добавленный модуль %s уже существует в другой POU исходного файла", group.Key, name)
			}
			module := SKZModule{Name: name, Type: moduleType, ObjectType: objectType, Capacity: capacity}
			for channel := 0; channel < capacity; channel++ {
				tag := fmt.Sprintf("%s_%d", skzModuleTag(group.SCS, name), channel)
				if group.Kind == "AI" && sourceTags[strings.ToUpper(group.SCS+"|"+tag)] {
					return nil, fmt.Errorf("СКЗ %s: имя добавленного резервного AI %s уже используется в исходном файле", group.Key, tag)
				}
				module.Channels = append(module.Channels, skzmap.Channel{Channel: channel, Tag: tag, Reserve: true})
			}
			pou.Modules = append(pou.Modules, module)
		}
		for index := range pou.Modules {
			if request.Kind == "st" {
				if choice.ModuleIDs[index] == nil {
					return nil, fmt.Errorf("СКЗ %s/%s: не указан физический ID", group.Key, pou.Modules[index].Name)
				}
				id := *choice.ModuleIDs[index]
				pou.Modules[index].ID = &id
			}
		}
		key := strings.ToUpper(group.SCS)
		index, exists := controllers[key]
		if !exists {
			index = len(plans)
			controllers[key] = index
			plans = append(plans, SKZPlan{SCS: group.SCS, Kind: request.Kind, Warnings: append([]string(nil), source.Warnings...)})
		} else if plans[index].SCS != group.SCS {
			return nil, fmt.Errorf("СКЗ: неоднозначный регистр имени ПЛК %q", group.SCS)
		}
		if request.Kind == "fbd" && group.Kind == "AI" {
			// The native AI page represents one physical module. Each module
			// gets its own executable POU and its own native page coordinates.
			for _, module := range pou.Modules {
				modulePOU := pou
				modulePOU.Name = "AI_" + strings.ReplaceAll(module.Name, "-", "_")
				modulePOU.Modules = []SKZModule{module}
				plans[index].POUs = append(plans[index].POUs, modulePOU)
			}
		} else {
			plans[index].POUs = append(plans[index].POUs, pou)
		}
	}
	for index := range plans {
		if _, err := validateSKZPlan(&plans[index]); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func skzModuleTag(scs, name string) string {
	return "_" + scs + "_" + strings.ReplaceAll(name, "-", "_")
}

func skzSyntheticReserve(channel skzmap.Channel) bool {
	return channel.Reserve && channel.SourceRow == 0
}

func validateSKZPlan(plan *SKZPlan) (DocumentRequirements, error) {
	var req DocumentRequirements
	if !aoSTFCSPattern.MatchString(plan.SCS) || plan.Kind != "st" && plan.Kind != "fbd" || len(plan.POUs) == 0 || len(plan.POUs) > 128 {
		return req, fmt.Errorf("СКЗ: неверный ПЛК, режим или число POU")
	}
	plan.ModuleCount, plan.SignalCount, plan.AssignmentCount, plan.RepeatedAssignmentCount = 0, 0, 0, 0
	pouNames, modules, hardware := map[string]bool{}, map[string]bool{}, map[int64]bool{}
	cards, called, assigned := map[string]string{}, map[string]bool{}, map[string]bool{}
	reservedAI := map[string]bool{}
	card := func(name, kind string) error {
		if !aoSTTagPattern.MatchString(name) {
			return fmt.Errorf("СКЗ: недопустимое имя карточки %q", name)
		}
		key := strings.ToUpper(name)
		if old, ok := cards[key]; ok && old != kind {
			return fmt.Errorf("СКЗ: имя %s использовано несовместимыми объектами %s/%s", name, old, kind)
		}
		cards[key] = kind
		return nil
	}
	for _, pou := range plan.POUs {
		wantName := pou.Kind + "_" + pou.Prefix
		if plan.Kind == "st" {
			wantName += "_channels"
		} else if pou.Kind == "AI" {
			if len(pou.Modules) != 1 {
				return req, fmt.Errorf("СКЗ AI FBD: POU должна содержать ровно один модуль")
			}
			wantName = "AI_" + strings.ReplaceAll(pou.Modules[0].Name, "-", "_")
		}
		if (pou.Kind != "AI" && pou.Kind != "DO") || !aoSTPrefixPattern.MatchString(pou.Prefix) || pou.Name != wantName || pouNames[strings.ToUpper(pou.Name)] || len(pou.Modules) == 0 {
			return req, fmt.Errorf("СКЗ: неверная или повторная POU %q", pou.Name)
		}
		pouNames[strings.ToUpper(pou.Name)] = true
		for _, module := range pou.Modules {
			_, validSlot := skzModuleSlot(module.Name, pou.Prefix)
			capacity, moduleType, objectType := 16, "AI16H", "AD3_v2"
			if pou.Kind == "DO" {
				capacity, moduleType, objectType = 32, "DO32P", "D32V"
			}
			if !validSlot || module.Type != moduleType || module.ObjectType != objectType || module.Capacity != capacity || modules[module.Name] || len(module.Channels) == 0 || len(module.Channels) > capacity {
				return req, fmt.Errorf("СКЗ %s: неверный или повторный модуль %q", pou.Name, module.Name)
			}
			modules[module.Name] = true
			plan.ModuleCount++
			if plan.ModuleCount > 4096 {
				return req, fmt.Errorf("СКЗ: не более 4096 модулей")
			}
			if plan.Kind == "st" {
				if module.ID == nil || *module.ID < 0 || *module.ID > maxTransportID || hardware[*module.ID] {
					return req, fmt.Errorf("СКЗ %s/%s: ID должен быть указан, уникален в ПЛК и лежать в диапазоне 0..%d", pou.Name, module.Name, maxTransportID)
				}
				hardware[*module.ID] = true
			} else if module.ID != nil {
				return req, fmt.Errorf("СКЗ FBD: физический ID модуля не используется")
			}
			if plan.Kind == "fbd" && pou.Kind == "DO" {
				if err := card(skzModuleTag(plan.SCS, module.Name), "D32V_v1"); err != nil {
					return req, err
				}
				req.T11Count++
			}
			lastChannel := -1
			for _, channel := range module.Channels {
				if channel.Channel <= lastChannel || channel.Channel >= capacity || !aoSTTagPattern.MatchString(channel.Tag) || channel.Member != "" && !aoSTTagPattern.MatchString(channel.Member) {
					return req, fmt.Errorf("СКЗ %s/%s: неверный тег или номер канала %d", pou.Name, module.Name, channel.Channel)
				}
				if pou.Kind == "AI" && channel.Member != "" || plan.Kind == "fbd" && pou.Kind == "DO" && channel.Member != "" {
					return req, fmt.Errorf("СКЗ: ожидается имя экземпляра AI или логической переменной DO без поля: %s.%s", channel.Tag, channel.Member)
				}
				lastChannel = channel.Channel
				plan.SignalCount++
				if plan.SignalCount > 4096 {
					return req, fmt.Errorf("СКЗ: не более 4096 каналов")
				}
				assignments := 1
				if pou.Kind == "AI" {
					assignments = 2
				}
				plan.AssignmentCount += assignments
				key := strings.ToUpper(channel.Tag + "." + channel.Member)
				if assigned[key] {
					if pou.Kind == "AI" && (skzSyntheticReserve(channel) || reservedAI[key]) {
						return req, fmt.Errorf("СКЗ: имя резервного AI %s совпадает с другим назначением", channel.Tag)
					}
					plan.RepeatedAssignmentCount += assignments
				}
				assigned[key] = true
				if pou.Kind == "AI" && skzSyntheticReserve(channel) {
					reservedAI[key] = true
				}
				if plan.Kind == "fbd" {
					if pou.Kind == "DO" && skzSyntheticReserve(channel) {
						continue
					}
					kind := "AD3_v2"
					if pou.Kind == "DO" {
						kind = "BOOL"
					}
					if err := card(channel.Tag, kind); err != nil {
						return req, err
					}
					if pou.Kind == "DO" {
						req.T11Count += 2
					} else if !called[key] {
						for _, suffix := range []string{"_MOS", "_SRV"} {
							if err := card(channel.Tag+suffix, "BOOL"); err != nil {
								return req, err
							}
						}
						req.T11Count += 9 // Seven native blocks and two explanatory primitives.
					}
					called[key] = true
				}
			}
		}
	}
	req.POUCount, req.SignalCount, req.CardCount = len(plan.POUs), plan.SignalCount, len(cards)
	return req, nil
}

func RequirementsForSKZ(plan SKZPlan) (DocumentRequirements, error) { return validateSKZPlan(&plan) }

func (g Generator) GenerateSKZ(plan SKZPlan, ctx AOMappingContext, ids IDRange) (Result, error) {
	req, err := validateSKZPlan(&plan)
	if err != nil {
		return Result{}, err
	}
	ctx, err = normalizeAOMappingContext(ctx, req.POUCount)
	if err != nil {
		return Result{}, err
	}
	if ctx.ControllerTypeName != "TENIX-CPU850" {
		return Result{}, fmt.Errorf("СКЗ: требуется контекст TENIX-CPU850")
	}
	if ids.POUID < 1 || ids.POUID > maxTransportID-int64(req.POUCount)+1 {
		return Result{}, fmt.Errorf("СКЗ: диапазон POU ID выходит за signed 32-bit")
	}
	if plan.Kind == "fbd" {
		if err := validateDocumentRanges(ids, req); err != nil {
			return Result{}, err
		}
		return generateSKZFBD(plan, ctx, ids, req)
	}
	return generateSKZST(plan, ctx, ids)
}

func skzCommon(ctx AOMappingContext) outputCommon {
	return outputCommon{Version: ctx.Version, Project: ctx.Project, IsCut: "false", IsFFB: "false", ControllerType: ctx.ControllerTypeName, ControllerID: ctx.ControllerID, ResourceID: ctx.ResourceID}
}

func skzSummary(plan SKZPlan, ctx AOMappingContext, ids IDRange) Summary {
	return Summary{POUCount: len(plan.POUs), IOModuleCount: plan.ModuleCount, SignalCount: plan.SignalCount, POUID: ids.POUID, POUName: plan.POUs[0].Name, POUGroupID: ctx.GroupID, POUNumber: ctx.POUNumber, POUs: []POUSummary{}}
}

func skzDOAssignment(scs string, module SKZModule, channel skzmap.Channel) string {
	return fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := %s._%02d;", *module.ID, channel.Channel, skzModuleTag(scs, module.Name), channel.Channel)
}

func generateSKZST(plan SKZPlan, ctx AOMappingContext, ids IDRange) (Result, error) {
	doc := outputAOSTDocument{XMLName: xml.Name{Local: "BufScadaPOUS"}, Common: skzCommon(ctx)}
	summary := skzSummary(plan, ctx, ids)
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	for index, pou := range plan.POUs {
		id, number := ids.POUID+int64(index), strconv.FormatInt(firstNumber+int64(index), 10)
		ps := POUSummary{POUID: id, POUName: pou.Name, POUGroupID: ctx.GroupID, POUNumber: number, Signals: []SignalSummary{}}
		var code strings.Builder
		fmt.Fprintf(&code, "PROGRAM %s\n\n", pou.Name)
		for _, module := range pou.Modules {
			fmt.Fprintf(&code, "(* %s *)\n", strings.ReplaceAll(module.Name, "-", "_"))
			prefix := fmt.Sprintf("_IO_I%d_AI16H", *module.ID)
			if pou.Kind == "DO" {
				prefix = fmt.Sprintf("_IO_Q%d_DO32P", *module.ID)
			}
			ps.IOModules = append(ps.IOModules, IOModuleSummary{Type: pou.Kind, ID: *module.ID, BindingPrefix: prefix, InstanceName: module.Name, Capacity: module.Capacity, SignalCount: len(module.Channels)})
			for _, channel := range module.Channels {
				if pou.Kind == "AI" {
					fmt.Fprintf(&code, "%s.Xin := %s_%d_VAL.Measurement;\n%s.Xs := QUAL_STAT(%s_%d_VAL.Quality);\n", channel.Tag, prefix, channel.Channel, channel.Tag, prefix, channel.Channel)
				} else {
					code.WriteString(skzDOAssignment(plan.SCS, module, channel) + "\n")
				}
				moduleID, channelNumber := *module.ID, channel.Channel
				ps.Signals = append(ps.Signals, SignalSummary{TemplateKey: "temporary:SKZ_ST_" + pou.Kind, BaseName: channel.Tag, IOType: pou.Kind, ModuleID: &moduleID, Channel: &channelNumber})
			}
			code.WriteByte('\n')
		}
		code.WriteString("END_PROGRAM")
		doc.POUS.Items = append(doc.POUS.Items, outputAOSTPOU{ID: strconv.FormatInt(id, 10), Name: pou.Name, IsFBD: "0", GroupID: ctx.GroupID, Enabled: "1", Number: number, Code: code.String()})
		summary.POUs = append(summary.POUs, ps)
	}
	data, err := serializeSCADAValue(doc)
	if err != nil {
		return Result{}, err
	}
	var actual outputAOSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &actual); err != nil {
		return Result{}, err
	}
	if !reflect.DeepEqual(actual, doc) || bytes.Contains(data, []byte("<ISAGraf")) || bytes.Contains(data, []byte("<ISACARDSINFO")) {
		return Result{}, fmt.Errorf("СКЗ: ST XML изменился при сериализации")
	}
	assignments := 0
	for _, pou := range actual.POUS.Items {
		assignments += strings.Count(pou.Code, ":=")
	}
	if assignments != plan.AssignmentCount {
		return Result{}, fmt.Errorf("СКЗ: неверное число ST присваиваний")
	}
	warnings := append([]string(nil), plan.Warnings...)
	warnings = append(warnings, "ST СКЗ сохраняет перечисленные в Excel каналы и назначает все каналы добавленных резервных модулей. Объекты AD3_v2 и D32V_v1 должны существовать; DO использует выходы _00…_31 объекта модуля, как SOGO_DO.xml. Функция QUAL_STAT должна присутствовать в TenixRtLib.")
	return Result{XML: data, BaseName: "SKZ_ST", Summary: summary, Warnings: warnings}, nil
}

// Initial values and call signatures come from the native AI_GRAF/DO_GRAF
// exports; this profile does not add CPU715 physical channel blocks.
const skzAD3Initial = "2(0),-1000,17383,-1000.0,6(0.0),17383.0,0.0,T#0ms,2(FALSE),0.0,FALSE,0,0.0,0,0,0.0,3(FALSE),0.0,0,T#0ms,3(0.0),D#1970-01-01,FALSE,0"

// SOGO_AI_fbd exports member reads before their owner. Import requires the
// opposite order: declare the AD3 card once, then address its Out/Stat fields.
// The commented native references/constants remain commented; physical input
// assignments belong to the separate ST POU, not inferred FBD connections.
func skzAIFragment(tag string, moduleIndex, channel int, start int64, ownerCard, mosCard, srvCard string) ([]outputBlock, []outputPrimitive) {
	yOffsets := [...]int{60, 580, 1110, 1640, 2160, 2670, 3200, 3720}
	x, y := 710+760*(channel%2), 4400*moduleIndex+yOffsets[channel/2]
	initial, boolean := skzAD3Initial, "FALSE"
	block := func(offset int64, objectType, info, text, kind, card, ci, co, commented string, iv *string, dx, dy, width, height int) outputBlock {
		return outputBlock{ObjectType: objectType, Info: info, T11ID: strconv.FormatInt(start+offset, 10),
			Graphics: outputBlockBounds{X: strconv.Itoa(x + dx), Y: strconv.Itoa(y + dy), Width: strconv.Itoa(width), Height: strconv.Itoa(height)},
			Params:   outputBlockParams{Text: text, CI: ci, CO: co, ViewMode: "0", Commented: commented, ISAObjectID: kind, CardID: card, Initial: iv}}
	}
	blocks := []outputBlock{
		block(0, "37", tag, "", "17480", ownerCard, "19", "3", "false", &initial, 0, 0, 100, 400),
		block(1, "31", tag+".Out", ".Out", "17480", ownerCard, "1", "1", "true", &initial, 120, 80, 180, 20),
		block(2, "31", tag+".Stat", ".Stat", "17480", ownerCard, "1", "1", "true", &initial, 120, 110, 180, 20),
		block(3, "31", tag+"_MOS", "", "-9", mosCard, "1", "1", "false", &boolean, -20, 430, 110, 20),
		block(4, "31", tag+"_SRV", "", "-9", srvCard, "1", "1", "false", &boolean, -210, 310, 110, 20),
		block(5, "34", "-1000", "-1000", "-100", "0", "0", "1", "true", nil, -200, 80, 100, 20),
		block(6, "34", "17383", "17383", "-100", "0", "0", "1", "true", nil, -200, 220, 100, 20),
	}
	primitive := func(offset int64, dx, dy, width, height int, pen, brush, label, color string) outputPrimitive {
		return outputPrimitive{SourceT11ID: strconv.FormatInt(start+offset, 10), X: strconv.Itoa(x + dx), Y: strconv.Itoa(y + dy), Width: strconv.Itoa(width), Height: strconv.Itoa(height),
			ObjectType: "1", GraphicNo: "0", DrawType: "2", PenParams: pen, PenColor: "0", BrushColor: brush, Gradient: "536870911",
			Params: "[TEXT]=" + label + "\n[FONTID]=65535\n[USERFONT]=8;Times New Roman;0;" + color + ";\n[HINT]=\n[DELTAST]=2\n[LINEHEIGHT]=15"}
	}
	graphics := []outputPrimitive{
		primitive(7, 130, 30, 200, 48, "337", "15790320", "  Копирование для диагностики#  и резервирования", "16757504"),
		primitive(8, -250, -40, 240, 24, "81", "16757504", " Обработка для отправки на ADR", "15790320"),
	}
	return blocks, graphics
}

func generateSKZFBD(plan SKZPlan, ctx AOMappingContext, ids IDRange, req DocumentRequirements) (Result, error) {
	doc := outputDocument{XMLName: xml.Name{Local: "BufScadaPOUS"}, Common: skzCommon(ctx)}
	summary := skzSummary(plan, ctx, ids)
	nextT11 := ids.T11Start
	cards, calls, types := map[string]string{}, map[string]bool{}, map[string]bool{}
	card := func(tag string) string {
		key := strings.ToUpper(tag)
		if id, exists := cards[key]; exists {
			return id
		}
		id := strconv.FormatInt(ids.CardStart+int64(len(cards)), 10)
		cards[key] = id
		doc.ISACards.Items = append(doc.ISACards.Items, outputISACard{ID: id, Info: tag, IsRetain: "-1", Name: tag, Size: "0"})
		return id
	}
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	for index, source := range plan.POUs {
		id, number := ids.POUID+int64(index), strconv.FormatInt(firstNumber+int64(index), 10)
		pou := outputPOU{ID: strconv.FormatInt(id, 10), Name: source.Name, IsFBD: "1", GroupID: ctx.GroupID, Enabled: "1", Number: number,
			Params: outputPOUParams{DParams: "3", Height: "2000", Width: "2000", TemplatePage: "0", Background: "16777215", PrintWidth: "1944", PrintHeight: "1363", PrintPageA4: "8"}}
		ps := POUSummary{POUID: id, POUName: source.Name, POUGroupID: ctx.GroupID, POUNumber: number, T11First: nextT11, Signals: []SignalSummary{}}
		localCards := map[string]bool{}
		add := func(tag, kind, ci, co, initial string, x, y, w, h int) outputBlock {
			cardID := card(tag)
			localCards[cardID] = true
			block := outputBlock{ObjectType: "37", Info: tag, T11ID: strconv.FormatInt(nextT11, 10), Graphics: outputBlockBounds{X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(w), Height: strconv.Itoa(h)}, Params: outputBlockParams{CI: ci, CO: co, ViewMode: "0", Commented: "false", ISAObjectID: kind, CardID: cardID, Initial: &initial}}
			if kind == "-9" {
				block.ObjectType = "31"
			}
			nextT11++
			pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, block)
			return block
		}
		for moduleIndex, module := range source.Modules {
			var digital outputBlock
			if source.Kind == "DO" {
				if !types["1933"] {
					doc.ISAObjects.Items = append(doc.ISAObjects.Items, outputISAObject{ID: "1933", Info: "D32V_v1", LibraryName: "TACSfbl"})
					types["1933"] = true
				}
				digital = add(skzModuleTag(plan.SCS, module.Name), "1933", "34", "33", "", 500, 50+770*moduleIndex, 100, 700)
			} else if !types["17480"] {
				doc.ISAObjects.Items = append(doc.ISAObjects.Items, outputISAObject{ID: "17480", Info: "AD3_v2", LibraryName: "TACSfbl"})
				types["17480"] = true
			}
			ps.IOModules = append(ps.IOModules, IOModuleSummary{Type: source.Kind, InstanceName: module.Name, Capacity: module.Capacity, SignalCount: len(module.Channels)})
			for _, channel := range module.Channels {
				channelNumber := channel.Channel
				ss := SignalSummary{TemplateKey: "temporary:SKZ_FBD_" + source.Kind, BaseName: channel.Tag, IOType: source.Kind, Channel: &channelNumber}
				if source.Kind == "AI" {
					key := strings.ToUpper(channel.Tag)
					if calls[key] {
						ps.Signals = append(ps.Signals, ss)
						continue
					}
					calls[key] = true
					ownerCard, mosCard, srvCard := card(channel.Tag), card(channel.Tag+"_MOS"), card(channel.Tag+"_SRV")
					localCards[ownerCard], localCards[mosCard], localCards[srvCard] = true, true, true
					blocks, graphics := skzAIFragment(channel.Tag, moduleIndex, channel.Channel, nextT11, ownerCard, mosCard, srvCard)
					pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, blocks...)
					pou.Graphics.Items = append(pou.Graphics.Items, graphics...)
					ss.Blocks, ss.Graphics, ss.Cards, ss.T11First, ss.T11Last = 7, 2, 3, nextT11, nextT11+8
					ss.CardFirst, _ = strconv.ParseInt(ownerCard, 10, 64)
					ss.CardLast, _ = strconv.ParseInt(srvCard, 10, 64)
					nextT11 += 9
				} else if !skzSyntheticReserve(channel) {
					y := d32InputY(50+770*moduleIndex, channel.Channel)
					input := add(channel.Tag, "-9", "1", "1", "FALSE", 220, y-10, 200, 20)
					pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items, makeIOLink(input, "0", digital, fmt.Sprintf("i%02d", channel.Channel), blockRightCenter(input), point{X: 500, Y: y}))
					nextT11++ // Links consume the shared transport reservation.
					ss.Blocks, ss.Links, ss.T11First, ss.T11Last = 1, 1, nextT11-2, nextT11-1
					ss.CardFirst, _ = strconv.ParseInt(input.Params.CardID, 10, 64)
					ss.CardLast = ss.CardFirst
				}
				ps.Signals = append(ps.Signals, ss)
			}
		}
		extendPOUForIO(&pou)
		ps.Blocks, ps.Links, ps.Graphics, ps.Cards = len(pou.ISAGraf.Blocks.Items), len(pou.ISAGraf.Links.Items), len(pou.Graphics.Items), len(localCards)
		ps.T11Last = nextT11 - 1
		if ps.Blocks == 0 {
			ps.T11First, ps.T11Last = 0, 0
		}
		for id := range localCards {
			value, _ := strconv.ParseInt(id, 10, 64)
			if ps.CardFirst == 0 || value < ps.CardFirst {
				ps.CardFirst = value
			}
			ps.CardLast = max(ps.CardLast, value)
		}
		doc.POUS.Items = append(doc.POUS.Items, pou)
		summary.POUs = append(summary.POUs, ps)
		summary.Blocks += ps.Blocks
		summary.Links += ps.Links
		summary.Graphics += ps.Graphics
	}
	summary.Cards, summary.T11First, summary.T11Last, summary.CardFirst, summary.CardLast = len(cards), ids.T11Start, nextT11-1, ids.CardStart, ids.CardStart+int64(len(cards))-1
	if nextT11-ids.T11Start != int64(req.T11Count) || len(cards) != req.CardCount {
		return Result{}, fmt.Errorf("СКЗ: расчёт FBD ID не совпал с результатом")
	}
	data, err := serializeSCADA(doc)
	if err != nil {
		return Result{}, err
	}
	if err := validateGeneratedSKZFBD(data, doc, req.T11Count); err != nil {
		return Result{}, err
	}
	warnings := append([]string(nil), plan.Warnings...)
	warnings = append(warnings, "FBD СКЗ вызывает AD3_v2 и D32V_v1; переменные DO подключены ко входам i00…i31 модулей. Физические назначения выполняются отдельными ST POU с суффиксом _channels. Библиотеки TACSfbl должны существовать в целевом проекте.")
	return Result{XML: data, BaseName: "SKZ_FBD", Summary: summary, Warnings: warnings}, nil
}

// Native AI calls and DO BOOL references use IsRetain=-1. Keep that policy
// local to SKZ rather than weakening validation for ordinary library exports.
func validateGeneratedSKZFBD(data []byte, expected outputDocument, count int) error {
	var actual outputDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &actual); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("СКЗ: FBD XML изменился при сериализации")
	}
	validID := func(value string) bool {
		id, err := strconv.ParseInt(value, 10, 64)
		return err == nil && id > 0 && id <= maxTransportID
	}
	types := map[string]bool{}
	for _, object := range actual.ISAObjects.Items {
		if types[object.ID] || object.LibraryName != "TACSfbl" || !(object.ID == "17480" && object.Info == "AD3_v2" || object.ID == "1933" && object.Info == "D32V_v1") {
			return fmt.Errorf("СКЗ: неверная или повторная библиотечная ссылка")
		}
		types[object.ID] = true
	}
	cards, instances, names, numbers, pouIDs := map[string]string{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, card := range actual.ISACards.Items {
		if !validID(card.ID) || !aoSTTagPattern.MatchString(card.Info) || cards[card.ID] != "" || instances[strings.ToUpper(card.Info)] || card.IsRetain != "-1" {
			return fmt.Errorf("СКЗ: неверная или повторная карточка")
		}
		cards[card.ID], instances[strings.ToUpper(card.Info)] = card.Info, true
	}
	var blocks []outputBlock
	var links []outputLink
	var graphics []outputPrimitive
	calledCards := map[string]bool{}
	for _, pou := range actual.POUS.Items {
		if !validID(pou.ID) || names[strings.ToUpper(pou.Name)] || numbers[pou.Number] || pouIDs[pou.ID] || pou.IsFBD != "1" || pou.Enabled != "1" {
			return fmt.Errorf("СКЗ: неверная или повторная FBD POU")
		}
		names[strings.ToUpper(pou.Name)], numbers[pou.Number], pouIDs[pou.ID] = true, true, true
		local := map[string]outputBlock{}
		aiOwners, boolReferences := map[string]string{}, map[string]int{}
		fields, constants := map[string]bool{}, map[string]int{}
		for _, block := range pou.ISAGraf.Blocks.Items {
			params := block.Params
			if !validID(block.T11ID) || params.ViewMode != "0" {
				return fmt.Errorf("СКЗ: неверная ссылка FBD блока")
			}
			valid := false
			switch {
			case block.ObjectType == "37":
				valid = types[params.ISAObjectID] && cards[params.CardID] == block.Info && params.Initial != nil && params.Commented == "false" && params.Text == "" && !calledCards[params.CardID]
				if valid && params.ISAObjectID == "17480" {
					valid = params.CI == "19" && params.CO == "3" && *params.Initial == skzAD3Initial
					aiOwners[params.CardID] = block.Info
				} else if valid && params.ISAObjectID == "1933" {
					valid = params.CI == "34" && params.CO == "33" && *params.Initial == ""
				}
				calledCards[params.CardID] = true
			case block.ObjectType == "31" && params.ISAObjectID == "17480":
				// A member must reference the already-declared owner in this POU.
				key := params.CardID + params.Text
				valid = aiOwners[params.CardID] != "" && (params.Text == ".Out" || params.Text == ".Stat") && block.Info == aiOwners[params.CardID]+params.Text && params.CI == "1" && params.CO == "1" && params.Commented == "true" && params.Initial != nil && *params.Initial == skzAD3Initial && !fields[key]
				fields[key] = true
			case block.ObjectType == "31" && params.ISAObjectID == "-9":
				valid = cards[params.CardID] == block.Info && params.Text == "" && params.CI == "1" && params.CO == "1" && params.Commented == "false" && params.Initial != nil && *params.Initial == "FALSE"
				boolReferences[block.Info]++
			case block.ObjectType == "34":
				valid = params.ISAObjectID == "-100" && params.CardID == "0" && (block.Info == "-1000" || block.Info == "17383") && params.Text == block.Info && params.CI == "0" && params.CO == "1" && params.Commented == "true" && params.Initial == nil
				constants[block.Info]++
			}
			if !valid {
				return fmt.Errorf("СКЗ: неподдерживаемый FBD блок")
			}
			local[block.T11ID] = block
		}
		for cardID, tag := range aiOwners {
			if !fields[cardID+".Out"] || !fields[cardID+".Stat"] || boolReferences[tag+"_MOS"] != 1 || boolReferences[tag+"_SRV"] != 1 {
				return fmt.Errorf("СКЗ: неразрешённые поля или вспомогательные карточки AD3_v2")
			}
		}
		if constants["-1000"] != len(aiOwners) || constants["17383"] != len(aiOwners) || len(pou.Graphics.Items) != 2*len(aiOwners) {
			return fmt.Errorf("СКЗ: нарушен состав нативного фрагмента AD3_v2")
		}
		for _, primitive := range pou.Graphics.Items {
			if !validID(primitive.SourceT11ID) || primitive.ObjectType != "1" {
				return fmt.Errorf("СКЗ: неверный графический примитив")
			}
		}
		inputs := map[string]bool{}
		for _, link := range pou.ISAGraf.Links.Items {
			from, to := strings.Split(link.FirstPoint.Value, "|"), strings.Split(link.LastPoint.Last, "|")
			if len(from) != 4 || len(to) != 4 || from[1] != "False" || from[2] != "0" || to[1] != "True" || local[from[0]].Params.ISAObjectID != "-9" || local[to[0]].Params.ISAObjectID != "1933" {
				return fmt.Errorf("СКЗ: неверные концы FBD связи")
			}
			channel, err := strconv.Atoi(strings.TrimPrefix(to[2], "i"))
			key := to[0] + "|" + to[2]
			if err != nil || channel < 0 || channel > 31 || to[2] != fmt.Sprintf("i%02d", channel) || inputs[key] {
				return fmt.Errorf("СКЗ: неверный или повторно подключённый вход D32V_v1")
			}
			inputs[key] = true
		}
		blocks = append(blocks, pou.ISAGraf.Blocks.Items...)
		links = append(links, pou.ISAGraf.Links.Items...)
		graphics = append(graphics, pou.Graphics.Items...)
	}
	return validateGeneratedXML(data, count, blocks, links, graphics, 0, true)
}
