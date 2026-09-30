// Single-template FBD requirements stage: Classifies source blocks, links and graphics and verifies the complete ID range.
package fbd

import (
	"fmt"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/library"
	"strings"
)

// classifyPrimitives Classifies source blocks, links and graphics and verifies the complete ID range.
// Unknown primitive kinds or signed32 overflow stop generation before rendering.
func (b *singleBuild) classifyPrimitives() error {
	b.primitives = b.ref.Template.Contents.Primitives
	b.blockCount, b.linkCount, b.graphicCount = 0, 0, 0
	for _, primitive := range b.primitives {
		switch {
		case library.IsSupportedBlockType(primitive.ObjectType):
			b.blockCount++
		case library.IsLinkType(primitive.ObjectType):
			b.linkCount++
		case library.IsGraphicType(primitive.ObjectType):
			b.graphicCount++
		default:
			return fmt.Errorf("шаблон содержит неподдерживаемый GROBJTYPE=%s (grprim ID=%s)", primitive.ObjectType, primitive.ID)
		}
	}
	if b.blockCount+b.linkCount+b.graphicCount != len(b.primitives) {
		return fmt.Errorf("внутренняя ошибка классификации примитивов")
	}
	if b.ids.T11Start > xmlidentity.MaxTransportID-int64(len(b.primitives))+1 {
		return fmt.Errorf("диапазон T11ID выходит за signed 32-bit")
	}
	if len(b.cards) > 0 && b.ids.CardStart > xmlidentity.MaxTransportID-int64(len(b.cards))+1 {
		return fmt.Errorf("диапазон cardId выходит за signed 32-bit")
	}

	return nil
}

// Requirements Считает примитивы и карточки конкретного библиотечного шаблона FBD.
// Вызывается перед allocator и не создаёт выходные файлы.
func Requirements(ref *library.TemplateRef) (t11Count, cardCount int) {
	cards := make(map[string]struct{})
	for _, primitive := range ref.Template.Contents.Primitives {
		cardID := strings.TrimSpace(primitive.CardID)
		if cardID != "" && cardID != "0" {
			cards[cardID] = struct{}{}
		}
	}
	return len(ref.Template.Contents.Primitives), len(cards)
}
