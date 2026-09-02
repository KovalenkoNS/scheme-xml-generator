package generator

import (
	"fmt"
	"strconv"
	"strings"
)

type generatedIOSignal struct {
	Anchor outputBlock
}

type ioAugmentResult struct {
	NextT11        int64
	NextCard       int64
	PrimitiveCount int
	Blocks         int
	Links          int
	Cards          int
	Modules        []IOModuleSummary
	Warnings       []string
}

func findGeneratedIOAnchor(pou outputPOU, ioType string) (outputBlock, error) {
	usage := outputEndpointUsage(pou.ISAGraf.Links.Items)
	candidates := make([]outputBlock, 0, 1)
	for _, block := range pou.ISAGraf.Blocks.Items {
		id := strings.TrimSpace(block.T11ID)
		switch ioType {
		case ioTypeAI:
			if block.ObjectType == "37" && block.Params.ISAObjectID == "17480" &&
				!usage.incoming[endpointKey(id, "Xin")] && !usage.incoming[endpointKey(id, "Xs")] {
				candidates = append(candidates, block)
			}
		case ioTypeAO:
			if block.ObjectType == "36" && block.Params.ISAObjectID == "791" &&
				strings.EqualFold(strings.TrimSpace(block.Info), "REAL_TO_DINT") &&
				!usage.outgoing[endpointKey(id, "Result")] {
				candidates = append(candidates, block)
			}
		case ioTypeDI:
			if isGeneratedDigitalAnchor(block) && !usage.incoming[endpointKey(id, "0")] {
				candidates = append(candidates, block)
			}
		case ioTypeDO:
			if isGeneratedDigitalAnchor(block) && !usage.outgoing[endpointKey(id, "0")] {
				candidates = append(candidates, block)
			}
		}
	}
	if len(candidates) != 1 {
		return outputBlock{}, fmt.Errorf("найдено %d свободных точек физической привязки %s, требуется ровно одна", len(candidates), ioType)
	}
	return candidates[0], nil
}

func isGeneratedDigitalAnchor(block outputBlock) bool {
	return block.ObjectType == "31" && block.Params.CardID != "" && block.Params.CardID != "0" &&
		block.Params.CI == "1" && block.Params.CO == "1"
}

func outputEndpointUsage(links []outputLink) endpointUsage {
	result := endpointUsage{incoming: make(map[string]bool), outgoing: make(map[string]bool)}
	for _, link := range links {
		if id, pin, ok := endpointIdentity(link.FirstPoint.Value); ok {
			result.outgoing[endpointKey(id, pin)] = true
		}
		if id, pin, ok := endpointIdentity(link.LastPoint.Last); ok {
			result.incoming[endpointKey(id, pin)] = true
		}
	}
	return result
}

func augmentIOPOU(
	pouRequest POURequest,
	generated []generatedIOSignal,
	pou *outputPOU,
	nextT11, nextCard int64,
	cardRecords *[]outputISACard,
	cardIDs map[string]struct{},
	cardInfos map[string]string,
	typeRecords map[string]outputISAObject,
) (ioAugmentResult, error) {
	result := ioAugmentResult{NextT11: nextT11, NextCard: nextCard}
	if pouRequest.IO == nil {
		return result, nil
	}
	assignments := ioAssignmentsForPOU(pouRequest)
	if len(assignments) != len(generated) {
		return result, fmt.Errorf("число сгенерированных сигналов %d не совпадает с числом физических назначений %d", len(generated), len(assignments))
	}
	// All digital modules share one horizontal hardware lane and are separated
	// vertically. Calculate the boundary before adding the first synthetic
	// module, otherwise each subsequent module would unnecessarily move farther
	// right because the previous module had already widened the POU.
	digitalContentRight := maxPOUBlockRight(*pou)

	flatStart := 0
	for moduleIndex, module := range pouRequest.IO.Modules {
		if module.ID == nil {
			return result, fmt.Errorf("модуль %d не получил ID после нормализации", moduleIndex+1)
		}
		capacity := ioCapacity(pouRequest.IO.Type)
		bindingPrefix := effectiveBindingPrefix(pouRequest.IO.Type, module)
		instanceName := effectiveModuleInstanceName(pouRequest.IO.Type, module)
		moduleSummary := IOModuleSummary{
			Type: pouRequest.IO.Type, ID: *module.ID, BindingPrefix: bindingPrefix,
			InstanceName: instanceName, Capacity: capacity, SignalCount: len(module.Signals),
			T11First: result.NextT11, CardFirst: result.NextCard,
		}
		if strings.TrimSpace(module.BindingPrefix) == "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"Для модуля %s ID=%d bindingPrefix не задан: использован fallback %s. Перед импортом сверьте внутренний префикс физического модуля с целевым проектом.",
				pouRequest.IO.Type, *module.ID, bindingPrefix,
			))
		}

		moduleSignals := generated[flatStart : flatStart+len(module.Signals)]
		var err error
		switch pouRequest.IO.Type {
		case ioTypeAI, ioTypeAO:
			err = augmentAnalogModule(pouRequest.IO.Type, module, bindingPrefix, moduleSignals, pou, &result, cardRecords, cardIDs, cardInfos)
		case ioTypeDI, ioTypeDO:
			err = augmentDigitalModule(pouRequest.IO.Type, moduleIndex, digitalContentRight, module, bindingPrefix, instanceName, moduleSignals, pou, &result, cardRecords, cardIDs, cardInfos, typeRecords)
		default:
			err = fmt.Errorf("неизвестный тип I/O %q", pouRequest.IO.Type)
		}
		if err != nil {
			return result, fmt.Errorf("модуль %s ID=%d: %w", pouRequest.IO.Type, *module.ID, err)
		}
		moduleSummary.Blocks = result.Blocks - sumModuleBlocks(result.Modules)
		moduleSummary.Links = result.Links - sumModuleLinks(result.Modules)
		moduleSummary.Cards = result.Cards - sumModuleCards(result.Modules)
		moduleSummary.T11Last = result.NextT11 - 1
		moduleSummary.CardLast = result.NextCard - 1
		result.Modules = append(result.Modules, moduleSummary)
		flatStart += len(module.Signals)
	}
	extendPOUForIO(pou)
	return result, nil
}

func augmentAnalogModule(
	ioType string,
	module IOModuleRequest,
	bindingPrefix string,
	signals []generatedIOSignal,
	pou *outputPOU,
	result *ioAugmentResult,
	cardRecords *[]outputISACard,
	cardIDs map[string]struct{},
	cardInfos map[string]string,
) error {
	for channel, signal := range signals {
		card, err := appendPhysicalCard(ioType, module, bindingPrefix, channel, result.NextCard, cardRecords, cardIDs, cardInfos)
		if err != nil {
			return err
		}
		result.NextCard++
		result.Cards++

		anchor := signal.Anchor
		anchorX, anchorY := blockXY(anchor)
		if ioType == ioTypeAI {
			x := max(0, anchorX-260)
			value := physicalBlock(card, ".ValueDINT", "17679", "0", x, anchorY+10, 220, result.NextT11)
			result.NextT11++
			status := physicalBlock(card, ".Status", "17679", "0", x, anchorY+30, 200, result.NextT11)
			result.NextT11++
			pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, value, status)
			result.Blocks += 2
			result.PrimitiveCount += 2

			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
				makeIOLink(value, "0", anchor, "Xin", blockRightCenter(value), point{X: anchorX, Y: anchorY + 20}),
				makeIOLink(status, "0", anchor, "Xs", blockRightCenter(status), point{X: anchorX, Y: anchorY + 40}),
			)
			result.NextT11 += 2
			result.Links += 2
			result.PrimitiveCount += 2
			continue
		}

		x := anchorX + intValue(anchor.Graphics.Width) + 60
		y := anchorY + max(0, (intValue(anchor.Graphics.Height)-20)/2)
		physical := physicalBlock(card, ".ValueDINT", "17679", "1", x, y, 220, result.NextT11)
		result.NextT11++
		pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, physical)
		pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
			makeIOLink(anchor, "Result", physical, "0", blockRightCenter(anchor), blockLeftCenter(physical)),
		)
		result.NextT11++
		result.Blocks++
		result.Links++
		result.PrimitiveCount += 2
	}
	return nil
}

func augmentDigitalModule(
	ioType string,
	moduleIndex int,
	contentRight int,
	module IOModuleRequest,
	bindingPrefix, instanceName string,
	signals []generatedIOSignal,
	pou *outputPOU,
	result *ioAugmentResult,
	cardRecords *[]outputISACard,
	cardIDs map[string]struct{},
	cardInfos map[string]string,
	typeRecords map[string]outputISAObject,
) error {
	moduleCard := outputISACard{
		ID: strconv.FormatInt(result.NextCard, 10), Info: instanceName, IsRetain: "-1",
		Name: instanceName, Size: "0", ClusterPath: "",
	}
	if err := appendIOCard(moduleCard, cardRecords, cardIDs, cardInfos); err != nil {
		return err
	}
	result.NextCard++
	result.Cards++

	d32X := contentRight + 400
	if ioType == ioTypeDO {
		d32X = contentRight + 200
	}
	d32Y := 50 + moduleIndex*770
	empty := ""
	d32 := outputBlock{
		ObjectType: "37", Info: instanceName, T11ID: strconv.FormatInt(result.NextT11, 10),
		Graphics: outputBlockBounds{X: strconv.Itoa(d32X), Y: strconv.Itoa(d32Y), Width: "100", Height: "700"},
		Params: outputBlockParams{
			Text: "", CI: "34", CO: "33", ViewMode: "0", Commented: "false",
			ISAObjectID: "1933", CardID: moduleCard.ID, Initial: &empty,
		},
	}
	result.NextT11++
	result.Blocks++
	result.PrimitiveCount++
	pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, d32)

	physicalBlocks := make([]outputBlock, 32)
	for channel := 0; channel < 32; channel++ {
		card, err := appendPhysicalCard(ioType, module, bindingPrefix, channel, result.NextCard, cardRecords, cardIDs, cardInfos)
		if err != nil {
			return err
		}
		result.NextCard++
		result.Cards++
		y := d32OutputY(d32Y, channel) - 10
		x := d32X - 300
		ci := "0"
		if ioType == ioTypeDO {
			x = d32X + 190
			ci = "1"
		}
		physicalBlocks[channel] = physicalBlock(card, ".Value", "17706", ci, x, y, 190, result.NextT11)
		if ioType == ioTypeDI {
			physicalBlocks[channel].Graphics.Y = strconv.Itoa(d32InputY(d32Y, channel) - 10)
		}
		result.NextT11++
		result.Blocks++
		result.PrimitiveCount++
	}
	pou.ISAGraf.Blocks.Items = append(pou.ISAGraf.Blocks.Items, physicalBlocks...)

	for channel, physical := range physicalBlocks {
		if ioType == ioTypeDI {
			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
				makeIOLink(physical, "0", d32, fmt.Sprintf("i%02d", channel), blockRightCenter(physical), point{X: d32X, Y: d32InputY(d32Y, channel)}),
			)
		} else {
			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
				makeIOLink(d32, fmt.Sprintf("_%02d", channel), physical, "0", point{X: d32X + 100, Y: d32OutputY(d32Y, channel)}, blockLeftCenter(physical)),
			)
		}
		result.NextT11++
		result.Links++
		result.PrimitiveCount++
	}

	for channel, signal := range signals {
		anchor := signal.Anchor
		if ioType == ioTypeDI {
			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
				makeIOLink(d32, fmt.Sprintf("_%02d", channel), anchor, "0", point{X: d32X + 100, Y: d32OutputY(d32Y, channel)}, blockLeftCenter(anchor)),
			)
		} else {
			pou.ISAGraf.Links.Items = append(pou.ISAGraf.Links.Items,
				makeIOLink(anchor, "0", d32, fmt.Sprintf("i%02d", channel), blockRightCenter(anchor), point{X: d32X, Y: d32InputY(d32Y, channel)}),
			)
		}
		result.NextT11++
		result.Links++
		result.PrimitiveCount++
	}

	d32Type := outputISAObject{ID: "1933", Info: "D32V_v1", LibraryName: "TACSfbl"}
	if previous, exists := typeRecords[d32Type.ID]; exists && previous != d32Type {
		return fmt.Errorf("ISAOBJSINFO ID=1933 имеет противоречивые определения")
	}
	typeRecords[d32Type.ID] = d32Type
	return nil
}

func appendPhysicalCard(
	ioType string,
	module IOModuleRequest,
	bindingPrefix string,
	channel int,
	cardID int64,
	cardRecords *[]outputISACard,
	cardIDs map[string]struct{},
	cardInfos map[string]string,
) (outputISACard, error) {
	name := fmt.Sprintf("[%d] %s > канал %d", *module.ID, ioDeviceName(ioType), channel)
	card := outputISACard{
		ID: strconv.FormatInt(cardID, 10), Info: bindingPrefix + "_" + strconv.Itoa(channel),
		IsRetain: "-1", Name: name, Size: "0", ClusterPath: "",
	}
	if err := appendIOCard(card, cardRecords, cardIDs, cardInfos); err != nil {
		return outputISACard{}, err
	}
	return card, nil
}

func appendIOCard(card outputISACard, cardRecords *[]outputISACard, cardIDs map[string]struct{}, cardInfos map[string]string) error {
	if _, exists := cardIDs[card.ID]; exists {
		return fmt.Errorf("повторный cardId=%s", card.ID)
	}
	infoKey := strings.ToUpper(strings.TrimSpace(card.Info))
	if infoKey == "" {
		return fmt.Errorf("пустой Card.Info для cardId=%s", card.ID)
	}
	if previous, exists := cardInfos[infoKey]; exists {
		return fmt.Errorf("Card.Info %q повторяется у cardId=%s и cardId=%s", card.Info, previous, card.ID)
	}
	cardIDs[card.ID] = struct{}{}
	cardInfos[infoKey] = card.ID
	*cardRecords = append(*cardRecords, card)
	return nil
}

func physicalBlock(card outputISACard, suffix, isaObjectID, ci string, x, y, width int, t11ID int64) outputBlock {
	return outputBlock{
		ObjectType: "38", Info: card.Name + suffix, T11ID: strconv.FormatInt(t11ID, 10),
		Graphics: outputBlockBounds{X: strconv.Itoa(x), Y: strconv.Itoa(y), Width: strconv.Itoa(width), Height: "20"},
		Params: outputBlockParams{
			Text: suffix, CI: ci, CO: "1", ViewMode: "0", Commented: "false",
			ISAObjectID: isaObjectID, CardID: card.ID,
		},
	}
}

type point struct{ X, Y int }

func makeIOLink(first outputBlock, firstPin string, last outputBlock, lastPin string, start, end point) outputLink {
	return outputLink{
		Negative: "false", AsPointer: "false", UserEdit: "false", Color: "0", GetBitNum: "-1", ConvertTo: "0",
		PointList:  outputPointList{Points: fmt.Sprintf("(%d,%d);(%d,%d);", start.X, start.Y, end.X, end.Y)},
		FirstPoint: outputEndpoint{Value: strings.Join([]string{first.T11ID, "False", firstPin, "0,0,100,20"}, "|")},
		LastPoint:  outputEndpoint{Last: strings.Join([]string{last.T11ID, "True", lastPin, "0,0,100,20"}, "|")},
	}
}

func blockXY(block outputBlock) (int, int) {
	return intValue(block.Graphics.X), intValue(block.Graphics.Y)
}

func blockLeftCenter(block outputBlock) point {
	x, y := blockXY(block)
	return point{X: x, Y: y + intValue(block.Graphics.Height)/2}
}

func blockRightCenter(block outputBlock) point {
	x, y := blockXY(block)
	return point{X: x + intValue(block.Graphics.Width), Y: y + intValue(block.Graphics.Height)/2}
}

func intValue(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func d32InputY(y, channel int) int  { return y + 60 + channel*20 }
func d32OutputY(y, channel int) int { return y + 40 + channel*20 }

func maxPOUBlockRight(pou outputPOU) int {
	result := 0
	for _, block := range pou.ISAGraf.Blocks.Items {
		result = max(result, intValue(block.Graphics.X)+intValue(block.Graphics.Width))
	}
	for _, primitive := range pou.Graphics.Items {
		result = max(result, intValue(primitive.X)+intValue(primitive.Width))
	}
	return result
}

func extendPOUForIO(pou *outputPOU) {
	maxX, maxY := 0, 0
	for _, block := range pou.ISAGraf.Blocks.Items {
		maxX = max(maxX, intValue(block.Graphics.X)+intValue(block.Graphics.Width))
		maxY = max(maxY, intValue(block.Graphics.Y)+intValue(block.Graphics.Height))
	}
	pou.Params.Width = maxNumericString(pou.Params.Width, strconv.Itoa(maxX+100))
	pou.Params.Height = maxNumericString(pou.Params.Height, strconv.Itoa(maxY+100))
}

func ioCapacity(ioType string) int {
	_, capacity, _ := normalizeIOType(ioType)
	return capacity
}

func sumModuleBlocks(modules []IOModuleSummary) int {
	total := 0
	for _, module := range modules {
		total += module.Blocks
	}
	return total
}

func sumModuleLinks(modules []IOModuleSummary) int {
	total := 0
	for _, module := range modules {
		total += module.Links
	}
	return total
}

func sumModuleCards(modules []IOModuleSummary) int {
	total := 0
	for _, module := range modules {
		total += module.Cards
	}
	return total
}
