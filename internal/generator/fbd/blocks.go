// Single-template FBD blocks stage: Converts ordered library primitives into FBD block records.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"

	"strconv"
	"strings"
)

// buildBlocks Converts ordered library primitives into FBD block records.
// Instance cards, signatures, initial values and shifted bounds determine each emitted block.
func (b *singleBuild) buildBlocks() error {
	b.blocks = make([]xmlmodel.OutputBlock, 0, b.blockCount)
	for _, primitive := range b.orderedBlocks {
		sourcePrimitiveID := strings.TrimSpace(primitive.ID)
		if _, exists := b.idMap[sourcePrimitiveID]; exists {
			return fmt.Errorf("повторный grprim ID=%s", sourcePrimitiveID)
		}
		newID := strconv.FormatInt(b.nextT11, 10)
		b.nextT11++
		b.idMap[sourcePrimitiveID] = newID
		params := library.ParseParams(primitive.Params)
		cardID := "0"
		var initial *string
		var card *generatedCard
		if sourceID := strings.TrimSpace(primitive.CardID); sourceID != "" && sourceID != "0" {
			resolved, ok := b.cardMap[sourceID]
			if !ok {
				return fmt.Errorf("не разрешён CARDID=%s у grprim ID=%s", sourceID, primitive.ID)
			}
			card = resolved
			cardID = resolved.ID
			if resolved.Source.InitialValue != nil {
				value := *resolved.Source.InitialValue
				initial = &value
			}
		} else if primitive.InitialValue != nil {
			value := *primitive.InitialValue
			initial = &value
		}
		ci, co := cleanNumber(params["CI"], "0"), cleanNumber(params["CO"], "0")
		if primitive.ObjectType == "36" || primitive.ObjectType == "37" {
			if b.ref.Template.ID == "19963" && primitive.ISAObjectID == "18513" && primitive.TypeName == "AD3_HLB" && library.Int(co, 0) < 17 {
				oldCO := co
				co = "17"
				b.warnings = append(b.warnings, fmt.Sprintf("Сигнатура AD3_HLB актуализирована по подтверждённому экспорту шаблона 19963: CO %s→17.", oldCO))
			} else if signature, ok := b.ref.Library.TypeSignatures[library.SignatureKey(primitive.ISAObjectID, primitive.TypeName)]; ok && (signature.CI != library.Int(ci, 0) || signature.CO != library.Int(co, 0)) {
				b.warnings = appendOnce(b.warnings, fmt.Sprintf("Для типа %s в библиотеке найдены разные сигнатуры; сохранены CI=%s, CO=%s выбранного шаблона.", primitive.TypeName, ci, co))
			}
		}
		commented, err := normalizeBool(params["COMMENT"])
		if err != nil {
			return fmt.Errorf("grprim block ID=%s: COMMENT: %w", primitive.ID, err)
		}
		b.blocks = append(b.blocks, xmlmodel.OutputBlock{
			ObjectType: primitive.ObjectType,
			Info:       blockInfo(primitive, params["TEXT"], card),
			T11ID:      newID,
			Graphics: xmlmodel.OutputBlockBounds{
				X: strconv.Itoa(library.Int(primitive.X, 0) + b.dx), Y: strconv.Itoa(library.Int(primitive.Y, 0) + b.dy),
				Width: cleanNumber(primitive.Width, "0"), Height: cleanNumber(primitive.Height, "0"),
			},
			Params: xmlmodel.OutputBlockParams{
				Text: params["TEXT"], CI: ci, CO: co, ViewMode: cleanNumber(params["VMODE"], "0"),
				Commented: commented, ISAObjectID: cleanNumber(primitive.ISAObjectID, "0"), CardID: cardID, Initial: initial,
			},
		})
	}

	return nil
}

// blockInfo Формирует подпись выходного FBD-блока по примитиву и связанной карточке.
// Сохраняет назначение библиотечного элемента при переименовании экземпляра.
func blockInfo(primitive library.Primitive, text string, card *generatedCard) string {
	if card != nil {
		if strings.HasPrefix(text, ".") {
			return card.Info + text
		}
		return card.Info
	}
	if strings.TrimSpace(primitive.TypeName) != "" {
		return primitive.TypeName
	}
	if primitive.ObjectType == "35" {
		switch strings.TrimSpace(primitive.ISAObjectID) {
		case "-3":
			return "+"
		case "-4":
			return "/"
		case "-33":
			return "OR"
		case "-32":
			return "NOT"
		}
	}
	return text
}
