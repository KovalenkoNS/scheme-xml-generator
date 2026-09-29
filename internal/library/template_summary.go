// Метаданные и структурные предупреждения библиотечного шаблона для каталога.
package library

import (
	"strings"
)

// summarizeTemplate собирает метаданные, совместимость и геометрию шаблона для каталога.
// Проверяет ID/связи/карточки, возвращает счётчики, предупреждения и preview без изменения библиотечных узлов.
func summarizeTemplate(ref *TemplateRef) TemplateSummary {
	template := ref.Template
	summary := TemplateSummary{
		Key:            ref.Key,
		LibraryFile:    ref.Library.FileName,
		OwnerID:        ref.Owner.ID,
		OwnerName:      ref.Owner.Name,
		ID:             template.ID,
		Name:           template.Name,
		Description:    strings.TrimSpace(template.Description),
		Width:          Int(template.Width, 0),
		Height:         Int(template.Height, 0),
		PrimitiveCount: len(template.Contents.Primitives),
		Supported:      true,
		Warnings:       []string{},
		IOCapabilities: ioCapabilities(template),
		Layout:         TemplateLayoutBounds(template),
	}
	if strings.HasPrefix(strings.ToUpper(summary.Description), "OLD") {
		summary.Warnings = append(summary.Warnings, "Шаблон помечен библиотекой как OLD/legacy.")
	}
	cards := make(map[string]struct{})
	cardIndex := make(map[string]ISAObject)
	for _, card := range ref.Owner.ISAObjects.Items {
		cardIndex[strings.TrimSpace(card.ID)] = card
	}
	blockIDs := make(map[string]struct{})
	primitiveIDs := make(map[string]struct{})
	cardOwners := make(map[string]bool)
	cardDependents := make(map[string]bool)
	isaDefinitions := make(map[string]string)
	for _, primitive := range template.Contents.Primitives {
		if IsSupportedBlockType(primitive.ObjectType) {
			blockIDs[strings.TrimSpace(primitive.ID)] = struct{}{}
		}
	}
	preview := TemplatePreview{Width: summary.Width, Height: summary.Height, Blocks: []PreviewBlock{}, Lines: [][]Point{}}
	for _, primitive := range template.Contents.Primitives {
		primitiveID := strings.TrimSpace(primitive.ID)
		if primitiveID == "" {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "grprim содержит пустой ID")
		} else if _, exists := primitiveIDs[primitiveID]; exists {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Повторный grprim ID="+primitiveID)
		} else {
			primitiveIDs[primitiveID] = struct{}{}
		}
		switch {
		case IsLinkType(primitive.ObjectType):
			summary.LinkCount++
			params := ParseParams(primitive.Params)
			if points, err := ParsePoints(params["PL"]); err == nil {
				preview.Lines = append(preview.Lines, points)
			} else {
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "Связь содержит некорректный PointList")
			}
			for _, key := range []string{"FP", "LP"} {
				parts := strings.SplitN(strings.TrimSpace(params[key]), "|", 4)
				if len(parts) != 4 {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи имеет некорректный формат")
					continue
				}
				if _, ok := blockIDs[strings.TrimSpace(parts[0])]; !ok {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи ссылается на блок вне шаблона")
				}
				switch strings.ToLower(strings.TrimSpace(parts[1])) {
				case "0", "false", "-1", "true":
				default:
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, key+" связи содержит неизвестное направление")
				}
			}
		case IsGraphicType(primitive.ObjectType):
			summary.GraphicCount++
			params := ParseParams(primitive.Params)
			if rawPoints := strings.TrimSpace(params["PL"]); rawPoints != "" {
				if _, err := ParsePoints(rawPoints); err != nil {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, "Графический примитив содержит некорректный PointList")
				}
			}
		case IsSupportedBlockType(primitive.ObjectType):
			summary.BlockCount++
			params := ParseParams(primitive.Params)
			switch strings.ToLower(strings.TrimSpace(params["COMMENT"])) {
			case "", "false", "0", "true", "1", "-1":
			default:
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "Block ID="+primitiveID+" содержит неизвестное Boolean COMMENT")
			}
			objectID := strings.TrimSpace(primitive.ISAObjectID)
			if Int(objectID, 0) > 0 {
				definition := strings.TrimSpace(primitive.TypeName) + "\x00" + strings.TrimSpace(primitive.LibraryName)
				if previous, exists := isaDefinitions[objectID]; exists && previous != definition {
					summary.Supported = false
					summary.Warnings = appendStringOnce(summary.Warnings, "OBJMSID "+objectID+" имеет противоречивые определения")
				} else {
					isaDefinitions[objectID] = definition
				}
			}
			cardID := strings.TrimSpace(primitive.CardID)
			if cardID != "" && cardID != "0" {
				if strings.TrimSpace(params["TEXT"]) == "" {
					cardOwners[cardID] = true
				} else {
					cardDependents[cardID] = true
				}
			}
			label := strings.TrimSpace(primitive.TypeName)
			if label == "" {
				label = strings.TrimSpace(params["TEXT"])
			}
			if label == "" && cardID != "" && cardID != "0" {
				if card, ok := cardIndex[cardID]; ok {
					label = card.EffectivePrefix()
				}
			}
			if label == "" && primitive.ObjectType == "35" {
				switch strings.TrimSpace(primitive.ISAObjectID) {
				case "-3":
					label = "+"
				case "-4":
					label = "/"
				case "-33":
					label = "OR"
				}
			}
			if label == "" {
				label = "T" + primitive.ObjectType
			}
			preview.Blocks = append(preview.Blocks, PreviewBlock{
				X: Int(primitive.X, 0), Y: Int(primitive.Y, 0), Width: Int(primitive.Width, 0), Height: Int(primitive.Height, 0), Label: label, Type: primitive.ObjectType,
			})
		default:
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Неизвестный GROBJTYPE="+primitive.ObjectType)
		}
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID != "" && cardID != "0" {
			cards[cardID] = struct{}{}
			if _, ok := cardIndex[cardID]; !ok {
				summary.Supported = false
				summary.Warnings = appendStringOnce(summary.Warnings, "CARDID "+cardID+" отсутствует в ISAOBJLIST")
			}
		}
	}
	for cardID := range cardDependents {
		if !cardOwners[cardID] {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "CARDID "+cardID+" используется полями без блока-владельца")
		}
	}
	cardPrefixes := make(map[string]string)
	for cardID := range cards {
		card, exists := cardIndex[cardID]
		if !exists {
			continue
		}
		prefixKey := strings.ToUpper(strings.TrimSpace(card.EffectivePrefix()))
		if previous, duplicate := cardPrefixes[prefixKey]; duplicate && previous != cardID {
			summary.Supported = false
			summary.Warnings = appendStringOnce(summary.Warnings, "Карточки "+previous+" и "+cardID+" создают одинаковый Card.Info")
		} else {
			cardPrefixes[prefixKey] = cardID
		}
	}
	summary.CardCount = len(cards)
	preview.Width = max(preview.Width, 1)
	preview.Height = max(preview.Height, 1)
	summary.Preview = preview
	return summary
}

// appendStringOnce добавляет предупреждение каталога без повторов одинакового текста.
// Возвращает прежний или расширенный срез, сохраняя порядок выявленных проблем шаблона.
func appendStringOnce(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
