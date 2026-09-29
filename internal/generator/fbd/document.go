// Сборка многопоуного библиотечного документа объединяет графы, карточки, шрифты и диапазоны ID.
package fbd

import (
	"encoding/xml"
	"fmt"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/identifiers"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
	"sort"
	"strconv"
	"strings"
)

// RequirementsForDocument calculates a single reservation for a resolved
// document. Source IDs may repeat between signals; only generated IDs share a
// document-wide namespace.
func RequirementsForDocument(pous []contracts.ResolvedPOU) (contracts.DocumentRequirements, error) {
	normalized, err := normalizeResolvedPOUs(pous)
	if err != nil {
		return contracts.DocumentRequirements{}, err
	}
	return requirementsForNormalizedDocument(normalized)
}

// requirementsForNormalizedDocument Считает ресурсы уже нормализованных библиотечных POU перед выделением ID.
// Учитывает шаблоны, карточки и физический слой, возвращает ошибки некорректного плана.
func requirementsForNormalizedDocument(pous []contracts.ResolvedPOU) (contracts.DocumentRequirements, error) {
	if len(pous) == 0 {
		return contracts.DocumentRequirements{}, fmt.Errorf("документ должен содержать хотя бы один POU")
	}
	result := contracts.DocumentRequirements{POUCount: len(pous)}
	for pouIndex, pou := range pous {
		if len(pou.Signals) == 0 {
			return contracts.DocumentRequirements{}, fmt.Errorf("POU %d не содержит сигналов", pouIndex+1)
		}
		for signalIndex, signal := range pou.Signals {
			if signal.Ref == nil || signal.Ref.Template == nil || signal.Ref.Library == nil {
				return contracts.DocumentRequirements{}, fmt.Errorf("POU %d, сигнал %d: шаблон не разрешён", pouIndex+1, signalIndex+1)
			}
			t11Count, cardCount := Requirements(signal.Ref)
			result.T11Count += t11Count
			result.CardCount += cardCount
			result.SignalCount++
		}
	}
	return result, nil
}

// GenerateDocument builds multiple independently configured POU sections in
// one BufScadaPOUS document. Each signal is generated with an isolated source
// ID map, while generated T11ID/cardId ranges and root dictionaries are global.
func (g Generator) GenerateDocument(request contracts.Request, pous []contracts.ResolvedPOU, ids contracts.IDRange) (contracts.Result, error) {
	var err error
	g, err = g.withGenerationContext(request.Context)
	if err != nil {
		return contracts.Result{}, err
	}
	pous, err = normalizeResolvedPOUs(pous)
	if err != nil {
		return contracts.Result{}, err
	}
	requirements, err := requirementsForNormalizedDocument(pous)
	if err != nil {
		return contracts.Result{}, err
	}
	if request.T11Start != nil {
		ids.T11Start = *request.T11Start
	}
	if request.CardStart != nil {
		ids.CardStart = *request.CardStart
	}
	if err := allocation.ValidateDocumentRanges(ids, requirements); err != nil {
		return contracts.Result{}, err
	}

	nextT11 := ids.T11Start
	nextCard := ids.CardStart
	primitiveCount := 0
	baseName := ""
	warnings := []string{}
	outputPOUs := make([]xmlmodel.OutputPOU, 0, len(pous))
	pouSummaries := make([]contracts.POUSummary, 0, len(pous))
	cardRecords := make([]xmlmodel.OutputISACard, 0, requirements.CardCount)
	cardIDs := make(map[string]struct{}, requirements.CardCount)
	cardInfos := make(map[string]string, requirements.CardCount)
	fontRecords := make(map[string]xmlmodel.OutputFontStyle)
	typeRecords := make(map[string]xmlmodel.OutputISAObject)
	pouIDs := make(map[int64]struct{}, len(pous))
	pouNames := make(map[string]struct{}, len(pous))
	pouNumbers := make(map[string]struct{}, len(pous))
	var common xmlmodel.OutputCommon
	commonSet := false

	for pouIndex, resolvedPOU := range pous {
		pouRequest := resolvedPOU.Request
		if err := identifiers.ValidateText(pouRequest.Description, fmt.Sprintf("описание POU %d", pouIndex+1)); err != nil {
			return contracts.Result{}, err
		}
		page, err := applyPageRequest(g.Config.Page, pouRequest.Page)
		if err != nil {
			return contracts.Result{}, fmt.Errorf("POU %d: %w", pouIndex+1, err)
		}
		pouName, err := resolveDocumentPOUName(pouRequest.Name, resolvedPOU.Signals[0].Ref.Template.Name, pouIndex)
		if err != nil {
			return contracts.Result{}, fmt.Errorf("POU %d: %w", pouIndex+1, err)
		}
		nameKey := strings.ToUpper(pouName)
		if _, exists := pouNames[nameKey]; exists {
			return contracts.Result{}, fmt.Errorf("имя POU %q повторяется в документе", pouName)
		}
		pouNames[nameKey] = struct{}{}

		pouID, err := resolveDocumentPOUID(ids.POUID, pouIndex, pouRequest.POUID)
		if err != nil {
			return contracts.Result{}, fmt.Errorf("POU %s: %w", pouName, err)
		}
		if _, exists := pouIDs[pouID]; exists {
			return contracts.Result{}, fmt.Errorf("POU ID %d повторяется в документе", pouID)
		}
		pouIDs[pouID] = struct{}{}

		groupID, err := resolvePOUContextValue(page.GroupID, pouRequest.GroupID, "POU GroupID", false)
		if err != nil {
			return contracts.Result{}, fmt.Errorf("POU %s: %w", pouName, err)
		}
		pouNumber, err := resolveDocumentPOUNumber(page.POUNumber, pouIndex, pouRequest.POUNumber)
		if err != nil {
			return contracts.Result{}, fmt.Errorf("POU %s: %w", pouName, err)
		}
		if _, exists := pouNumbers[pouNumber]; exists {
			return contracts.Result{}, fmt.Errorf("POUNum %s повторяется в документе", pouNumber)
		}
		pouNumbers[pouNumber] = struct{}{}
		page.GroupID = groupID
		page.POUNumber = pouNumber

		pouT11First := nextT11
		pouCardFirst := nextCard
		pouSummary := contracts.POUSummary{
			POUID: pouID, POUName: pouName, POUGroupID: groupID, POUNumber: pouNumber,
			T11First: pouT11First, CardFirst: pouCardFirst,
			Signals: make([]contracts.SignalSummary, 0, len(resolvedPOU.Signals)),
		}
		var combined xmlmodel.OutputPOU
		combinedSet := false
		autoY := contracts.DefaultSignalOffsetY

		for signalIndex, resolvedSignal := range resolvedPOU.Signals {
			signal := resolvedSignal.Request
			if strings.TrimSpace(signal.TemplateKey) == "" {
				signal.TemplateKey = resolvedSignal.Ref.Key
			}
			layout := library.TemplateLayoutBounds(resolvedSignal.Ref.Template)
			dx := contracts.DefaultSignalOffsetX - min(0, layout.MinX)
			dy := autoY - min(0, layout.MinY)
			if signal.OffsetX != nil {
				dx = *signal.OffsetX
			}
			if signal.OffsetY != nil {
				dy = *signal.OffsetY
			}
			templateBottom := dy + templateLayoutBottom(resolvedSignal.Ref) + contracts.DefaultSignalGapY
			if templateBottom > autoY {
				autoY = templateBottom
			}
			dxCopy, dyCopy := dx, dy
			localGenerator := g
			localGenerator.Config.Page = page
			legacyRequest := contracts.Request{
				TemplateKey: signal.TemplateKey,
				ObjectName:  signal.ObjectName,
				POUName:     pouName,
				NameMode:    signal.NameMode,
				Description: signal.Description,
				ClusterPath: signal.ClusterPath,
				OffsetX:     &dxCopy,
				OffsetY:     &dyCopy,
			}
			t11Count, cardCount := Requirements(resolvedSignal.Ref)
			part, err := localGenerator.Generate(resolvedSignal.Ref, legacyRequest, contracts.IDRange{T11Start: nextT11, CardStart: nextCard, POUID: pouID})
			if err != nil {
				return contracts.Result{}, fmt.Errorf("POU %s, сигнал %d: %w", pouName, signalIndex+1, err)
			}
			var partDocument xmlmodel.OutputDocument
			if err := xml.Unmarshal(part.XML, &partDocument); err != nil {
				return contracts.Result{}, fmt.Errorf("POU %s, сигнал %d: повторно разобрать промежуточный XML: %w", pouName, signalIndex+1, err)
			}
			if len(partDocument.POUS.Items) != 1 {
				return contracts.Result{}, fmt.Errorf("POU %s, сигнал %d: промежуточный документ содержит %d POU", pouName, signalIndex+1, len(partDocument.POUS.Items))
			}
			if !commonSet {
				common = partDocument.Common
				commonSet = true
			} else if common != partDocument.Common {
				return contracts.Result{}, fmt.Errorf("POU %s, сигнал %d: шаблон требует несовместимый корневой Common", pouName, signalIndex+1)
			}

			partPOU := partDocument.POUS.Items[0]
			if !combinedSet {
				combined = partPOU
				combined.Description = pouRequest.Description
				combinedSet = true
			} else {
				combined.ISAGraf.Blocks.Items = append(combined.ISAGraf.Blocks.Items, partPOU.ISAGraf.Blocks.Items...)
				combined.ISAGraf.Links.Items = append(combined.ISAGraf.Links.Items, partPOU.ISAGraf.Links.Items...)
				combined.Graphics.Items = append(combined.Graphics.Items, partPOU.Graphics.Items...)
				combined.Params.Width = maxNumericString(combined.Params.Width, partPOU.Params.Width)
				combined.Params.Height = maxNumericString(combined.Params.Height, partPOU.Params.Height)
			}

			for _, card := range partDocument.ISACards.Items {
				if _, exists := cardIDs[card.ID]; exists {
					return contracts.Result{}, fmt.Errorf("повторный cardId=%s", card.ID)
				}
				infoKey := strings.ToUpper(strings.TrimSpace(card.Info))
				if previous, exists := cardInfos[infoKey]; exists {
					return contracts.Result{}, fmt.Errorf("Card.Info %q повторяется у cardId=%s и cardId=%s", card.Info, previous, card.ID)
				}
				cardIDs[card.ID] = struct{}{}
				cardInfos[infoKey] = card.ID
				cardRecords = append(cardRecords, card)
			}
			if partDocument.FontStyles != nil {
				for _, record := range partDocument.FontStyles.Items {
					if previous, exists := fontRecords[record.ID]; exists && previous != record {
						return contracts.Result{}, fmt.Errorf("FONTSTYLES ID=%s имеет противоречивые определения", record.ID)
					}
					fontRecords[record.ID] = record
				}
			}
			for _, record := range partDocument.ISAObjects.Items {
				if previous, exists := typeRecords[record.ID]; exists && previous != record {
					return contracts.Result{}, fmt.Errorf("ISAOBJSINFO ID=%s имеет противоречивые определения", record.ID)
				}
				typeRecords[record.ID] = record
			}

			if baseName == "" {
				baseName = part.BaseName
			}
			warnings = append(warnings, part.Warnings...)
			signalSummary := contracts.SignalSummary{
				TemplateKey: signal.TemplateKey, BaseName: part.BaseName,
				Blocks: part.Summary.Blocks, Links: part.Summary.Links, Graphics: part.Summary.Graphics, Cards: part.Summary.Cards,
				T11First: nextT11, T11Last: nextT11 + int64(t11Count) - 1,
				CardFirst: nextCard, CardLast: nextCard + int64(cardCount) - 1,
				OffsetX: dx, OffsetY: dy,
			}
			pouSummary.Signals = append(pouSummary.Signals, signalSummary)
			pouSummary.Blocks += signalSummary.Blocks
			pouSummary.Links += signalSummary.Links
			pouSummary.Graphics += signalSummary.Graphics
			pouSummary.Cards += signalSummary.Cards
			nextT11 += int64(t11Count)
			nextCard += int64(cardCount)
			primitiveCount += t11Count
		}

		pouSummary.T11Last = nextT11 - 1
		pouSummary.CardLast = nextCard - 1
		outputPOUs = append(outputPOUs, combined)
		pouSummaries = append(pouSummaries, pouSummary)
	}

	fonts := sortedFonts(fontRecords)
	types := sortedISAObjects(typeRecords)
	sort.Slice(cardRecords, func(i, j int) bool { return lessNumericID(cardRecords[i].ID, cardRecords[j].ID) })
	var fontStyles *xmlmodel.OutputFontStyles
	if len(fonts) > 0 {
		fontStyles = &xmlmodel.OutputFontStyles{Items: fonts}
	}
	document := xmlmodel.OutputDocument{
		Common: common, POUS: xmlmodel.OutputPOUS{Items: outputPOUs}, FontStyles: fontStyles,
		ISAObjects: xmlmodel.OutputISAObjects{Items: types}, ISACards: xmlmodel.OutputISACards{Items: cardRecords},
	}
	data, err := xmlcodec.SerializeSCADAValue(document)
	if err != nil {
		return contracts.Result{}, fmt.Errorf("сериализовать SCADA XML: %w", err)
	}
	if err := validateGeneratedDocumentXML(data, primitiveCount, document); err != nil {
		return contracts.Result{}, err
	}

	firstPOU := pouSummaries[0]
	summary := contracts.Summary{
		T11First: ids.T11Start, T11Last: nextT11 - 1,
		CardFirst: ids.CardStart, CardLast: nextCard - 1,
		POUID: firstPOU.POUID, POUName: firstPOU.POUName, POUGroupID: firstPOU.POUGroupID, POUNumber: firstPOU.POUNumber,
		POUCount: requirements.POUCount, SignalCount: requirements.SignalCount, POUs: pouSummaries,
	}
	for _, pou := range pouSummaries {
		summary.Blocks += pou.Blocks
		summary.Links += pou.Links
		summary.Graphics += pou.Graphics
		summary.Cards += pou.Cards
		summary.IOModuleCount += len(pou.IOModules)
	}
	return contracts.Result{XML: data, BaseName: baseName, Summary: summary, Warnings: uniqueStrings(warnings)}, nil
}

// applyPageRequest Накладывает явно заданные размеры и параметры страницы FBD на конфигурацию.
// Проверяет ограничения XML; отсутствующие значения сохраняет из текущих defaults.
func applyPageRequest(defaults config.PageDefaults, request contracts.PageRequest) (config.PageDefaults, error) {
	result := defaults
	if request.Width != nil {
		if *request.Width < 1 || *request.Width > contracts.MaxPageExtent {
			return result, fmt.Errorf("ширина страницы должна быть в диапазоне 1..%d", contracts.MaxPageExtent)
		}
		result.Width = *request.Width
	}
	if request.Height != nil {
		if *request.Height < 1 || *request.Height > contracts.MaxPageExtent {
			return result, fmt.Errorf("высота страницы должна быть в диапазоне 1..%d", contracts.MaxPageExtent)
		}
		result.Height = *request.Height
	}
	if request.MarginRight != nil {
		if *request.MarginRight < 0 || *request.MarginRight > contracts.MaxPageExtent {
			return result, fmt.Errorf("правое поле страницы должно быть в диапазоне 0..%d", contracts.MaxPageExtent)
		}
		result.MarginRight = *request.MarginRight
	}
	if request.MarginBottom != nil {
		if *request.MarginBottom < 0 || *request.MarginBottom > contracts.MaxPageExtent {
			return result, fmt.Errorf("нижнее поле страницы должно быть в диапазоне 0..%d", contracts.MaxPageExtent)
		}
		result.MarginBottom = *request.MarginBottom
	}
	if request.DParams != nil {
		value, err := addressing.NormalizeContextInteger(*request.DParams, "DPARAMS")
		if err != nil {
			return result, err
		}
		result.DParams = value
	}
	if request.BackgroundColor != nil {
		value, err := addressing.NormalizeContextInteger(*request.BackgroundColor, "FONCOLOR")
		if err != nil {
			return result, err
		}
		result.Background = value
	}
	return result, nil
}

// resolveDocumentPOUName Выбирает имя POU многопрограммного документа из запроса или шаблона.
// Добавляет индекс при автоматическом именовании и проверяет допустимые символы.
func resolveDocumentPOUName(requested, templateName string, index int) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return resolvePOUName(requested, templateName)
	}
	name := makePOUName(templateName)
	if index == 0 {
		return name, nil
	}
	suffix := "_" + strconv.Itoa(index+1)
	runes := []rune(name)
	if len(runes)+len([]rune(suffix)) > 160 {
		runes = runes[:160-len([]rune(suffix))]
	}
	return strings.TrimRight(string(runes), "_") + suffix, nil
}

// resolveDocumentPOUID Определяет ID POU из ручного значения либо выделенного allocator диапазона.
// Проверяет signed32 до записи идентификатора в XML.
func resolveDocumentPOUID(start int64, index int, requested *int64) (int64, error) {
	if requested != nil {
		if *requested < 1 || *requested > contracts.MaxTransportID {
			return 0, fmt.Errorf("POU ID должен быть положительным signed 32-bit")
		}
		return *requested, nil
	}
	if start > contracts.MaxTransportID-int64(index) {
		return 0, fmt.Errorf("POU ID должен быть положительным signed 32-bit")
	}
	value := start + int64(index)
	if value < 1 || value > contracts.MaxTransportID {
		return 0, fmt.Errorf("POU ID должен быть положительным signed 32-bit")
	}
	return value, nil
}

// resolvePOUContextValue Разрешает GroupID и другие числовые константы POU из запроса и defaults.
// Отличает пропуск от явного нуля и проверяет допустимость значения для данного поля.
func resolvePOUContextValue(defaultValue string, requested *int64, field string, allowZero bool) (string, error) {
	value, err := addressing.NormalizeContextInteger(defaultValue, field)
	if err != nil {
		return "", err
	}
	if requested == nil {
		return value, nil
	}
	minimum := int64(1)
	if allowZero {
		minimum = 0
	}
	if *requested < minimum || *requested > contracts.MaxTransportID {
		return "", fmt.Errorf("%s должен быть %ssigned 32-bit", field, map[bool]string{true: "неотрицательным ", false: "положительным "}[allowZero])
	}
	return strconv.FormatInt(*requested, 10), nil
}

// resolveDocumentPOUNumber Назначает POUNum из запроса либо базового номера и индекса документа.
// Возвращает нормализованную строку или ошибку диапазона.
func resolveDocumentPOUNumber(defaultValue string, index int, requested *int64) (string, error) {
	if requested != nil {
		return resolvePOUContextValue(defaultValue, requested, "POUNum", false)
	}
	value, err := addressing.NormalizeContextInteger(defaultValue, "POUNum")
	if err != nil {
		return "", err
	}
	number, _ := strconv.ParseInt(value, 10, 32)
	number += int64(index)
	if number > contracts.MaxTransportID {
		return "", fmt.Errorf("POUNum должен помещаться в signed 32-bit")
	}
	return strconv.FormatInt(number, 10), nil
}

// sortedFonts Упорядочивает объединённые записи шрифтов перед сериализацией FBD.
// Возвращает стабильный список вместо случайного порядка map.
func sortedFonts(records map[string]xmlmodel.OutputFontStyle) []xmlmodel.OutputFontStyle {
	result := make([]xmlmodel.OutputFontStyle, 0, len(records))
	for _, record := range records {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return lessNumericID(result[i].ID, result[j].ID) })
	return result
}

// sortedISAObjects Упорядочивает библиотечные ISA-типы объединённого FBD-документа.
// Стабильный список делает повторную сериализацию воспроизводимой.
func sortedISAObjects(records map[string]xmlmodel.OutputISAObject) []xmlmodel.OutputISAObject {
	result := make([]xmlmodel.OutputISAObject, 0, len(records))
	for _, record := range records {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return lessNumericID(result[i].ID, result[j].ID) })
	return result
}

// lessNumericID Сравнивает строковые ID как числа при сортировке XML-справочников.
// Для нечисловых значений сохраняет определённый строковый порядок.
func lessNumericID(left, right string) bool {
	leftNumber, leftErr := strconv.ParseInt(strings.TrimSpace(left), 10, 64)
	rightNumber, rightErr := strconv.ParseInt(strings.TrimSpace(right), 10, 64)
	if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return left < right
}

// maxNumericString Выбирает большую числовую строку при объединении границ страницы FBD.
// Помогает сохранить размер, необходимый для всех размещённых экземпляров.
func maxNumericString(left, right string) string {
	if library.Int(right, 0) > library.Int(left, 0) {
		return right
	}
	return left
}

// templateLayoutBottom Читает нижнюю границу фактического содержимого библиотечного шаблона.
// Она задаёт вертикальный шаг автоматического размещения экземпляров FBD.
func templateLayoutBottom(ref *library.TemplateRef) int {
	return max(1, library.TemplateLayoutBounds(ref.Template).MaxY)
}
