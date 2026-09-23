package generator

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"scheme-xml-generator/internal/aomap"
)

// AOMappingContext describes the destination of the temporary, native AN_v1
// profile. It is deliberately independent from library-template defaults.
type AOMappingContext struct {
	Version            string `json:"version"`
	Project            string `json:"project"`
	ControllerTypeName string `json:"controllerTypeName"`
	ControllerID       string `json:"controllerId"`
	ResourceID         string `json:"resourceId"`
	GroupID            string `json:"groupId"`
	POUNumber          string `json:"pouNumber"`
}

func DefaultAOMappingContext() AOMappingContext {
	return AOMappingContext{
		Version: "29", Project: `otpscadafb3:e:\TAProject\ОАОН\СИБУР\Sinopec\SCADABD.GDB`,
		ControllerTypeName: "TENIX-CPU715", ControllerID: "189300", ResourceID: "637",
		GroupID: "19498", POUNumber: "36",
	}
}

func normalizeAOMappingContext(ctx AOMappingContext, pouCount int) (AOMappingContext, error) {
	for _, item := range []struct {
		name  string
		value *string
	}{
		{"VER", &ctx.Version}, {"ControllerID", &ctx.ControllerID},
		{"ResuorceID", &ctx.ResourceID}, {"GroupID", &ctx.GroupID}, {"POUNum", &ctx.POUNumber},
	} {
		value, err := normalizeContextInteger(*item.value, item.name)
		if err != nil {
			return ctx, err
		}
		*item.value = value
	}
	for _, value := range []string{ctx.Project, ctx.ControllerTypeName} {
		if !utf8.ValidString(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n\t") {
			return ctx, fmt.Errorf("некорректное значение контекста импорта")
		}
		for _, r := range value {
			if r < 32 || r == 0xfffe || r == 0xffff {
				return ctx, fmt.Errorf("контекст импорта содержит недопустимый XML-символ")
			}
		}
	}
	number, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	if number > maxTransportID-int64(pouCount)+1 {
		return ctx, fmt.Errorf("диапазон POUNum выходит за signed 32-bit")
	}
	return ctx, nil
}

// RequirementsForAOMap counts channel positions separately from graphic objects.
// A repeated tag is an empty slot: it consumes neither a block nor a card ID.
func RequirementsForAOMap(plan *aomap.Plan) (DocumentRequirements, error) {
	return requirementsForResolvedAOMap(aomap.ResolveDuplicates(plan))
}

func requirementsForResolvedAOMap(plan *aomap.Plan) (DocumentRequirements, error) {
	var req DocumentRequirements
	if plan == nil || len(plan.Groups) == 0 || len(plan.Groups) > 128 {
		return req, fmt.Errorf("карта AO должна содержать от 1 до 128 POU")
	}
	tags := map[string]bool{}
	names := map[string]bool{}
	modules := 0
	fcs := plan.Groups[0].FCS
	if fcs == "" {
		return req, fmt.Errorf("не задан FCS карты AO")
	}
	for _, group := range plan.Groups {
		if group.FCS != fcs {
			return req, fmt.Errorf("один AO XML может содержать только один FCS; разделите карту по контроллерам")
		}
		if err := validatePOUName(group.POUName); err != nil {
			return req, err
		}
		name := strings.ToUpper(group.POUName)
		if names[name] || len(group.Modules) == 0 {
			return req, fmt.Errorf("повторная или пустая POU %s", group.POUName)
		}
		names[name] = true
		moduleNames := map[string]bool{}
		for _, module := range group.Modules {
			if module.ObjectType != "AN_v1" || len(module.Channels) != 4 || moduleNames[module.Name] {
				return req, fmt.Errorf("модуль %s: ожидаются уникальный модуль AN_v1 и четыре канала", module.Name)
			}
			moduleNames[module.Name] = true
			modules++
			if modules > 4096 {
				return req, fmt.Errorf("карта AO содержит более 4096 модулей")
			}
			for index, channel := range module.Channels {
				if channel.Channel != index || channel.Tag == "" {
					return req, fmt.Errorf("модуль %s: нарушена последовательность каналов 0..3", module.Name)
				}
				if err := validatePOUName(channel.Tag); err != nil {
					return req, fmt.Errorf("модуль %s, канал %d: %w", module.Name, index, err)
				}
				req.SignalCount++ // All positions, including omitted duplicate slots.
				if channel.Duplicate {
					continue
				}
				low, lowErr := strconv.ParseFloat(channel.Min, 64)
				high, highErr := strconv.ParseFloat(channel.Max, 64)
				if lowErr != nil || highErr != nil || low != 0 || high != 100 {
					return req, fmt.Errorf("модуль %s, канал %d: временный профиль поддерживает только диапазон 0..100 из эталона; преобразователь REAL_TO_DINT в XML не создаётся", module.Name, index)
				}
				tags[strings.ToUpper(channel.Tag)] = true
				req.T11Count++
			}
		}
	}
	req.CardCount = len(tags)
	req.POUCount = len(plan.Groups)
	if req.CardCount != req.T11Count {
		return req, fmt.Errorf("один экземпляр AN_v1 не может иметь несколько графических вызовов в одном FCS")
	}
	return req, nil
}

// GenerateAOMap reproduces A11_00_example.xml's software-only FBD profile.
// No physical-address or converter objects are inferred from a TXT label.
func (g Generator) GenerateAOMap(plan *aomap.Plan, ctx AOMappingContext, ids IDRange) (Result, error) {
	plan = aomap.ResolveDuplicates(plan)
	req, err := requirementsForResolvedAOMap(plan)
	if err != nil {
		return Result{}, err
	}
	ctx, err = normalizeAOMappingContext(ctx, req.POUCount)
	if err != nil {
		return Result{}, err
	}
	if err = validateDocumentRanges(ids, req); err != nil {
		return Result{}, err
	}
	if ids.POUID > maxTransportID-int64(req.POUCount)+1 {
		return Result{}, fmt.Errorf("диапазон POU ID выходит за signed 32-bit")
	}
	doc := outputDocument{
		Common:     outputCommon{Version: ctx.Version, Project: ctx.Project, IsCut: "false", IsFFB: "false", ControllerType: ctx.ControllerTypeName, ControllerID: ctx.ControllerID, ResourceID: ctx.ResourceID},
		ISAObjects: outputISAObjects{Items: []outputISAObject{{ID: "888", Info: "AN_v1", LibraryName: "TACSfbl"}}},
	}
	summary := Summary{Blocks: req.T11Count, Cards: req.CardCount, SignalCount: req.SignalCount, POUCount: req.POUCount,
		T11First: ids.T11Start, T11Last: ids.T11Start + int64(req.T11Count) - 1,
		CardFirst: ids.CardStart, CardLast: ids.CardStart + int64(req.CardCount) - 1,
		POUID: ids.POUID, POUName: plan.Groups[0].POUName, POUGroupID: ctx.GroupID, POUNumber: ctx.POUNumber,
		POUs: []POUSummary{}}
	type cardRef struct {
		id  int64
		tag string
	}
	cards := map[string]cardRef{}
	nextT11 := ids.T11Start
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	for groupIndex, group := range plan.Groups {
		pouID := ids.POUID + int64(groupIndex)
		pouNumber := strconv.FormatInt(firstNumber+int64(groupIndex), 10)
		pou := outputPOU{ID: strconv.FormatInt(pouID, 10), Name: group.POUName, IsFBD: "1", GroupID: ctx.GroupID, Enabled: "1", Number: pouNumber,
			Params: outputPOUParams{DParams: "3", Height: strconv.Itoa(max(12000, 50+190*len(group.Modules)*4+100)), Width: "2000", TemplatePage: "0", Background: "16777215", PrintWidth: "1944", PrintHeight: "1363", PrintPageA4: "8"}}
		ps := POUSummary{POUID: pouID, POUName: group.POUName, POUGroupID: ctx.GroupID, POUNumber: pouNumber, T11First: nextT11, Signals: []SignalSummary{}}
		localCards := map[int64]bool{}
		for moduleIndex, module := range group.Modules {
			summary.IOModuleCount++
			for channelIndex, channel := range module.Channels {
				if channel.Duplicate {
					continue
				}
				key := strings.ToUpper(channel.Tag)
				card, exists := cards[key]
				if !exists {
					card = cardRef{id: ids.CardStart + int64(len(cards)), tag: channel.Tag}
					cards[key] = card
					doc.ISACards.Items = append(doc.ISACards.Items, outputISACard{ID: strconv.FormatInt(card.id, 10), Info: card.tag, IsRetain: "-1", Name: card.tag + " Аналоговая нагрузка", Size: "0"})
				}
				localCards[card.id] = true
				if ps.CardFirst == 0 || card.id < ps.CardFirst {
					ps.CardFirst = card.id
				}
				ps.CardLast = max(ps.CardLast, card.id)
				// Placement follows the original slot grid, not the number of emitted
				// blocks: skipped redundant calls must leave a real visual gap.
				y := 50 + 190*(moduleIndex*4+channelIndex)
				initial := ",,,100.0"
				pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, outputBlock{ObjectType: "37", Info: card.tag, T11ID: strconv.FormatInt(nextT11, 10),
					Graphics: outputBlockBounds{X: "500", Y: strconv.Itoa(y), Width: "100", Height: "160"},
					Params:   outputBlockParams{Text: "", CI: "7", CO: "2", ViewMode: "0", Commented: "false", ISAObjectID: "888", CardID: strconv.FormatInt(card.id, 10), Initial: &initial}})
				channelNumber := channel.Channel
				ps.Signals = append(ps.Signals, SignalSummary{TemplateKey: "temporary:AN_v1", BaseName: card.tag, Blocks: 1, Cards: 1, T11First: nextT11, T11Last: nextT11, CardFirst: card.id, CardLast: card.id, OffsetX: 500, OffsetY: y, IOType: "AO", Channel: &channelNumber})
				nextT11++
			}
		}
		ps.T11Last = nextT11 - 1
		ps.Blocks = len(pou.ISAGraf.Blocks.Items)
		if ps.Blocks == 0 {
			ps.T11First, ps.T11Last = 0, 0
		}
		ps.Cards = len(localCards)
		doc.POUS.Items = append(doc.POUS.Items, pou)
		summary.POUs = append(summary.POUs, ps)
	}
	data, err := serializeSCADA(doc)
	if err != nil {
		return Result{}, fmt.Errorf("создать AO XML: %w", err)
	}
	if err := validateGeneratedDocumentForProfile(data, req.T11Count, doc, true); err != nil {
		return Result{}, err
	}
	warnings := append([]string{}, plan.Warnings...)
	warnings = append(warnings, "Профиль AN_v1 создаёт только программные блоки по эталону A11_00_example.xml, без физической привязки и REAL_TO_DINT. Каждый XML содержит POU одного FCS; импортируйте файл в соответствующий ПЛК.")
	if skipped := req.SignalCount - req.T11Count; skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("Повторных вызовов пропущено: %d. Их позиции оставлены пустыми, без блоков и заглушек; координаты остальных каналов сохранены.", skipped))
	}
	return Result{XML: data, BaseName: "AO_TXT", Summary: summary, Warnings: warnings}, nil
}
