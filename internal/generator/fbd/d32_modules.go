// Библиотечный renderer модульного DO размещает D32, канальные NOT и цепь качества по обязательному профилю.
package fbd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"regexp"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	"scheme-xml-generator/internal/generator/identifiers"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	moduleid "scheme-xml-generator/internal/generator/modules"
	"scheme-xml-generator/internal/generator/planning"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"strconv"
	"strings"
)

// generateD32ModuleFBD собирает один D32 и диагностическую цепь на каждый модуль.
// Получает проверенный план, ID и обязательные библиотечные метаданные; возвращает
// проверенный XML с точной инверсией каналов, не записывая файлы или состояние allocator.
func generateD32ModuleFBD(plan stassignment.ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange, req xmlidentity.DocumentRequirements, profile *d32LibraryProfile) (xmlartifact.Result, error) {
	if profile == nil {
		return xmlartifact.Result{}, fmt.Errorf("DO: библиотечный профиль не подготовлен")
	}
	doc := xmlmodel.OutputDocument{XMLName: xml.Name{Local: "BufScadaPOUS"}, Common: xmlmodel.MappingCommon(ctx)}
	doc.ISAObjects.Items = libraryDOTypeRecords(profile)
	summary := stassignment.ControllerSummary(plan, ctx, ids)
	summary.POUGroupID, summary.POUNumber = profile.groups[0], profile.numbers[0]
	next := ids.T11Start
	cards, cardNames := map[string]string{}, map[string]string{}
	card := func(tag string) string {
		key := strings.ToUpper(tag)
		if id := cards[key]; id != "" {
			return id
		}
		id := strconv.FormatInt(ids.CardStart+int64(len(cards)), 10)
		cards[key], cardNames[id] = id, tag
		// The workbook does not carry native display paths or descriptions.
		// Use the known owner tag, never a fabricated hardware-tree path.
		doc.ISACards.Items = append(doc.ISACards.Items, xmlmodel.OutputISACard{ID: id, Info: tag, Name: tag, IsRetain: "-1", Size: "0"})
		return id
	}
	firstNumber, _ := strconv.ParseInt(ctx.POUNumber, 10, 32)
	for index, source := range plan.POUs {
		id, number := ids.POUID+int64(index), strconv.FormatInt(firstNumber+int64(index), 10)
		pou := xmlmodel.OutputPOU{ID: strconv.FormatInt(id, 10), Name: source.Name, IsFBD: "1", GroupID: ctx.GroupID, Enabled: "1", Number: number,
			Params: xmlmodel.OutputPOUParams{DParams: "3", Height: "20000", Width: "2000", TemplatePage: "0", Background: "16777215", PrintWidth: "1944", PrintHeight: "1363", PrintPageA4: "8"}}
		ps := xmlartifact.POUSummary{POUID: id, POUName: source.Name, POUGroupID: ctx.GroupID, POUNumber: number, T11First: next, Signals: []xmlartifact.SignalSummary{}}
		pou.GroupID, pou.Number = profile.groups[index], profile.numbers[index]
		ps.POUGroupID, ps.POUNumber = pou.GroupID, pou.Number
		localCards := map[string]bool{}
		add := func(objectType, info, isa, cardID, text, ci, co string, initial *string, x, y, width, height int) xmlmodel.OutputBlock {
			block := xmlmodel.OutputBlock{ObjectType: objectType, Info: info, T11ID: strconv.FormatInt(next, 10),
				Graphics: xmlmodel.OutputBlockBounds{X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: strconv.Itoa(height)},
				Params:   xmlmodel.OutputBlockParams{Text: text, CI: ci, CO: co, ViewMode: "0", Commented: "false", ISAObjectID: isa, CardID: cardID, Initial: initial}}
			next++
			if cardID != "0" {
				localCards[cardID] = true
			}
			pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, block)
			return block
		}
		link := func(from xmlmodel.OutputBlock, out string, to xmlmodel.OutputBlock, in string, start, end point) {
			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items, makeIOLink(from, out, to, in, start, end))
			next++ // Every link also consumes the shared graphics reservation.
		}
		for moduleIndex, module := range source.Modules {
			y, moduleStart := 60+770*moduleIndex, next
			moduleTag, physicalTag := moduleid.ModuleInstanceTag(plan.ControllerName, module.Name), addressing.DODiagnosticTag(*module.ID)
			moduleCard, physicalCard := card(moduleTag), card(physicalTag)
			physical := add("38", physicalTag+".Quality", "6127", physicalCard, ".Quality", "0", "1", nil, 20, y+10, 420, 20)
			qualityISA, digitalISA, signalISA := profile.quality.ISAObjectID, profile.digital.ISAObjectID, profile.signal.ISAObjectID
			digitalInitial, signalInitial := profile.digitalInitial, profile.signalInitial
			quality := add("36", "QUAL_STAT", qualityISA, "0", "", "1", "1", nil, 490, y, 120, 40)
			digital := add("37", moduleTag, digitalISA, moduleCard, "", "34", "33", digitalInitial, 660, y, 100, 700)
			link(physical, "0", quality, "QUAL", blockRightCenter(physical), blockLeftCenter(quality))
			link(quality, "Result", digital, "sts", blockRightCenter(quality), point{X: 660, Y: y + 40})
			pou.ISAGraf.Links.Items[len(pou.ISAGraf.Links.Items)-1].PointList.Points = fmt.Sprintf("(610,%d);(630,%d);(630,%d);(660,%d);", y+20, y+20, y+40, y+40)
			ms := xmlartifact.IOModuleSummary{Type: "DO", ID: *module.ID, BindingPrefix: physicalTag, InstanceName: module.Name, Capacity: 32, SignalCount: len(module.Channels), Blocks: 3, Links: 2, Cards: 2, T11First: moduleStart, T11Last: next - 1}
			ms.CardFirst, _ = strconv.ParseInt(moduleCard, 10, 64)
			ms.CardLast, _ = strconv.ParseInt(physicalCard, 10, 64)
			ps.IOModules = append(ps.IOModules, ms)
			for _, channel := range module.Channels {
				moduleID, channelNumber := *module.ID, channel.Channel
				ss := xmlartifact.SignalSummary{TemplateKey: profile.templateKey, BaseName: channel.Tag, IOType: "DO", ModuleID: &moduleID, Channel: &channelNumber}
				if !planning.IsSyntheticReserve(channel) {
					start, cardCount := next, len(cards)
					ownerCard := card(channel.Tag)
					rowY := y + 50 + 20*channel.Channel
					input := add("31", cardNames[ownerCard], signalISA, ownerCard, "", "1", "1", signalInitial, 260, rowY, 190, 20)
					invert := profile.inverted[moduleID][channel.Channel]
					if invert {
						inverse := add("35", "NOT", "-32", "0", "", "1", "1", nil, 530, rowY, 80, 20)
						link(input, "0", inverse, "0", blockRightCenter(input), blockLeftCenter(inverse))
						link(inverse, "Result", digital, fmt.Sprintf("i%02d", channel.Channel), blockRightCenter(inverse), point{X: 660, Y: d32InputY(y, channel.Channel)})
						ss.Blocks, ss.Links = 2, 2
					} else {
						link(input, "0", digital, fmt.Sprintf("i%02d", channel.Channel), blockRightCenter(input), point{X: 660, Y: d32InputY(y, channel.Channel)})
						ss.Blocks, ss.Links = 1, 1
					}
					ss.Cards, ss.T11First, ss.T11Last = len(cards)-cardCount, start, next-1
					ss.CardFirst, _ = strconv.ParseInt(ownerCard, 10, 64)
					ss.CardLast = ss.CardFirst
				}
				ps.Signals = append(ps.Signals, ss)
			}
		}
		extendPOUForIO(&pou)
		ps.Blocks, ps.Links, ps.Cards, ps.T11Last = len(pou.ISAGraf.Blocks.Items), len(pou.ISAGraf.Links.Items), len(localCards), next-1
		for cardID := range localCards {
			value, _ := strconv.ParseInt(cardID, 10, 64)
			if ps.CardFirst == 0 || value < ps.CardFirst {
				ps.CardFirst = value
			}
			ps.CardLast = max(ps.CardLast, value)
		}
		doc.POUS.Items = append(doc.POUS.Items, pou)
		summary.POUs = append(summary.POUs, ps)
		summary.Blocks += ps.Blocks
		summary.Links += ps.Links
	}
	summary.Cards, summary.T11First, summary.T11Last = len(cards), ids.T11Start, next-1
	summary.CardFirst, summary.CardLast = ids.CardStart, ids.CardStart+int64(len(cards))-1
	if next-ids.T11Start != int64(req.T11Count) || len(cards) != req.CardCount {
		return xmlartifact.Result{}, fmt.Errorf("Модули native850: расчёт FBD ID не совпал с результатом")
	}
	data, err := xmlcodec.SerializeSCADAValue(doc)
	if err != nil {
		return xmlartifact.Result{}, err
	}
	if err := validateD32ModuleFBD(data, doc, req.T11Count, profile); err != nil {
		return xmlartifact.Result{}, err
	}
	warnings := append([]string(nil), plan.Warnings...)
	warnings = append(warnings, "DO FBD: типы BOOL/D32/QUAL_STAT получены из выбранной библиотеки. Один D32 на модуль; инверсия задаётся отдельно для канала. Quality канала 0 использует подтверждённый драйвер CPU850/Measurement. Импорт и выполнение в SCADA требуют отдельной проверки.")
	return xmlartifact.Result{XML: data, BaseName: "LIBRARY_DO_" + plan.ControllerName, Summary: summary, Warnings: warnings}, nil
}

var doQualityPattern = regexp.MustCompile(`^_IO_I(0|[1-9][0-9]*)_DO32P_0_VAL_DIAG$`)

// validateD32ModuleFBD повторно разбирает XML после сериализации общего renderer.
// Сверяет типы выбранного профиля, карточки и единственность входных связей,
// затем проверяет диалект/ID; это не подтверждение импорта или выполнения в SCADA.
func validateD32ModuleFBD(data []byte, expected xmlmodel.OutputDocument, count int, profile *d32LibraryProfile) error {
	if profile == nil {
		return fmt.Errorf("DO: библиотечный профиль не подготовлен")
	}
	var actual xmlmodel.OutputDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, xmlcodec.Utf8BOM), &actual); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("Модули native850: XML изменился при сериализации")
	}
	invalid := func() error {
		return fmt.Errorf("Модули native850: нарушен нативный контракт DO FBD")
	}
	validID := func(value string) bool {
		id, err := strconv.ParseInt(value, 10, 64)
		return err == nil && id > 0 && id <= xmlidentity.MaxTransportID
	}
	wantTypes := libraryDOTypeRecords(profile)
	if actual.Common.ControllerType != cpuprofile.ControllerCPU850 || !reflect.DeepEqual(actual.ISAObjects.Items, wantTypes) || len(actual.POUS.Items) == 0 || actual.FontStyles != nil {
		return invalid()
	}
	cards, owners := map[string]xmlmodel.OutputISACard{}, map[string]bool{}
	for _, card := range actual.ISACards.Items {
		if !validID(card.ID) || !identifiers.ObjectNamePattern.MatchString(card.Info) || owners[strings.ToUpper(card.Info)] || cards[card.ID].ID != "" || card.IsRetain != "-1" || card.Size != "0" || card.Name != card.Info || card.ClusterPath != "" {
			return invalid()
		}
		cards[card.ID], owners[strings.ToUpper(card.Info)] = card, true
	}
	used, called, cardRoles := map[string]bool{}, map[string]bool{}, map[string]string{}
	pouIDs, names, numbers := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var blocks []xmlmodel.OutputBlock
	var links []xmlmodel.OutputLink
	for _, pou := range actual.POUS.Items {
		if !validID(pou.ID) || pouIDs[pou.ID] || names[strings.ToUpper(pou.Name)] || numbers[pou.Number] || pou.IsFBD != "1" || pou.Enabled != "1" || len(pou.Graphics.Items) != 0 {
			return invalid()
		}
		pouIDs[pou.ID], names[strings.ToUpper(pou.Name)], numbers[pou.Number] = true, true, true
		local := map[string]xmlmodel.OutputBlock{}
		for _, block := range pou.ISAGraf.Blocks.Items {
			p := block.Params
			if !validID(block.T11ID) || local[block.T11ID].T11ID != "" || p.ViewMode != "0" || p.Commented != "false" {
				return invalid()
			}
			card := cards[p.CardID]
			valid := false
			switch block.ObjectType {
			case "37":
				valid = p.ISAObjectID == profile.digital.ISAObjectID && p.CI == "34" && p.CO == "33" && p.Text == "" && sameInitial(p.Initial, profile.digitalInitial) && card.ID != "" && card.Info == block.Info && !called[p.CardID]
				called[p.CardID] = true
			case "38":
				match := doQualityPattern.FindStringSubmatch(card.Info)
				valid = p.ISAObjectID == "6127" && p.CI == "0" && p.CO == "1" && p.Text == ".Quality" && p.Initial == nil && card.ID != "" && block.Info == card.Info+p.Text && len(match) == 2 && !called[p.CardID]
				if valid {
					physicalID, err := strconv.ParseInt(match[1], 10, 64)
					valid = err == nil && physicalID <= xmlidentity.MaxTransportID
				}
				called[p.CardID] = true
			case "31":
				valid = p.ISAObjectID == profile.signal.ISAObjectID && p.CI == "1" && p.CO == "1" && p.Text == "" && sameInitial(p.Initial, profile.signalInitial) && card.ID != "" && block.Info == card.Info
			case "35", "36":
				valid = p.CI == "1" && p.CO == "1" && p.Text == "" && p.Initial == nil && p.CardID == "0" && block.ObjectType == "35" && p.ISAObjectID == "-32" && block.Info == "NOT"
				if block.ObjectType == "36" {
					valid = p.CI == "1" && p.CO == "1" && p.Text == "" && p.Initial == nil && p.CardID == "0" && p.ISAObjectID == profile.quality.ISAObjectID && block.Info == "QUAL_STAT"
				}
			}
			if !valid {
				return invalid()
			}
			if card.ID != "" {
				if previous := cardRoles[card.ID]; previous != "" && previous != block.ObjectType {
					return invalid()
				}
				cardRoles[card.ID] = block.ObjectType
				used[card.ID] = true
			}
			local[block.T11ID] = block
		}
		inputs, outputs := map[string]int{}, map[string]int{}
		for _, link := range pou.ISAGraf.Links.Items {
			from, to := strings.Split(link.FirstPoint.Value, "|"), strings.Split(link.LastPoint.Last, "|")
			if len(from) != 4 || len(to) != 4 || from[1] != "False" || to[1] != "True" || from[3] != "0,0,100,20" || to[3] != "0,0,100,20" || link.Negative != "false" || link.AsPointer != "false" || link.UserEdit != "false" || link.Color != "0" || link.GetBitNum != "-1" || link.ConvertTo != "0" {
				return invalid()
			}
			first, last := local[from[0]], local[to[0]]
			valid := first.ObjectType == "31" && from[2] == "0" && last.ObjectType == "35" && to[2] == "0" ||
				first.ObjectType == "38" && from[2] == "0" && last.ObjectType == "36" && to[2] == "QUAL" ||
				first.ObjectType == "36" && from[2] == "Result" && last.ObjectType == "37" && to[2] == "sts"
			if (first.ObjectType == "35" && from[2] == "Result" || first.ObjectType == "31" && from[2] == "0") && last.ObjectType == "37" {
				channel, err := strconv.Atoi(strings.TrimPrefix(to[2], "i"))
				valid = err == nil && channel >= 0 && channel < 32 && to[2] == fmt.Sprintf("i%02d", channel)
			}
			inputs[to[0]+"|"+to[2]]++
			outputs[from[0]]++
			if !valid || inputs[to[0]+"|"+to[2]] != 1 || outputs[from[0]] != 1 {
				return invalid()
			}
		}
		for id, block := range local {
			switch block.ObjectType {
			case "37":
				if inputs[id+"|sts"] != 1 || outputs[id] != 0 {
					return invalid()
				}
			case "35":
				if inputs[id+"|0"] != 1 || outputs[id] != 1 {
					return invalid()
				}
			case "36":
				if inputs[id+"|QUAL"] != 1 || outputs[id] != 1 {
					return invalid()
				}
			default:
				if outputs[id] != 1 {
					return invalid()
				}
			}
		}
		blocks = append(blocks, pou.ISAGraf.Blocks.Items...)
		links = append(links, pou.ISAGraf.Links.Items...)
	}
	if len(used) != len(cards) {
		return invalid()
	}
	return validateGeneratedXML(data, count, blocks, links, nil, 0, true)
}
