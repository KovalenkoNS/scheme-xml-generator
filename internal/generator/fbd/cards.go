// Single-template FBD cards stage: Builds unique instance cards from library owners and their initial values.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/library"

	"strconv"
	"strings"
)

// prepareCards Builds unique instance cards from library owners and their initial values.
// Card validation and library legacy notices complete before primitive IDs are assigned.
func (b *singleBuild) prepareCards() error {
	var err error
	b.cards, b.cardMap, err = buildCards(b.ref, b.preview.BaseName, b.request.Description, b.ids.CardStart)
	if err != nil {
		return err
	}
	if err := validateUniqueCardInfo(b.cards); err != nil {
		return err
	}
	for _, card := range b.cards {
		if card.Source.InitialValue != nil && strings.HasPrefix(*card.Source.InitialValue, "*") {
			b.warnings = appendOnce(b.warnings, "INITIALVALUE с ведущим маркером '*' сохранён буквально; семантика маркера должна поддерживаться целевым импортёром.")
		}
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(b.ref.Template.Description)), "OLD") {
		b.warnings = append(b.warnings, "Выбранный шаблон помечен библиотекой как OLD/legacy.")
	}

	return nil
}

type generatedCard struct {
	Source library.ISAObject
	ID     string
	Info   string
	Name   string
}

// sourceCards Индексирует исходные карточки выбранного библиотечного шаблона по ID.
// Карта используется при создании экземпляров и привязке графических блоков.
func sourceCards(ref *library.TemplateRef) map[string]library.ISAObject {
	result := make(map[string]library.ISAObject, len(ref.Owner.ISAObjects.Items))
	for _, card := range ref.Owner.ISAObjects.Items {
		result[strings.TrimSpace(card.ID)] = card
	}
	return result
}

// validateUniqueCardInfo Проверяет отсутствие одинаковых имён у создаваемых карточек FBD.
// Возвращает конфликт до объединения их в выходной документ.
func validateUniqueCardInfo(cards []*generatedCard) error {
	seen := make(map[string]string, len(cards))
	for _, card := range cards {
		key := strings.ToUpper(strings.TrimSpace(card.Info))
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("Card.Info %q повторяется у исходных CARDID=%s и CARDID=%s", card.Info, previous, card.Source.ID)
		}
		seen[key] = card.Source.ID
	}
	return nil
}

// buildCards Создаёт карточки экземпляра по карточкам библиотечного шаблона.
// Подставляет имя/описание и новые ID, возвращает список и индекс для блоков.
func buildCards(ref *library.TemplateRef, baseName, description string, start int64) ([]*generatedCard, map[string]*generatedCard, error) {
	index := sourceCards(ref)
	ordered := make([]*generatedCard, 0)
	result := make(map[string]*generatedCard)
	for _, primitive := range ref.Template.Contents.Primitives {
		sourceID := strings.TrimSpace(primitive.CardID)
		if sourceID == "" || sourceID == "0" {
			continue
		}
		if _, exists := result[sourceID]; exists {
			continue
		}
		source, ok := index[sourceID]
		if !ok {
			return nil, nil, fmt.Errorf("CARDID %s отсутствует в ISAOBJLIST объекта %s", sourceID, ref.Owner.Name)
		}
		info := baseName + source.EffectivePrefix()
		name := strings.TrimSpace(description)
		if name == "" {
			name = info
		}
		if suffix := strings.TrimSpace(source.Description); suffix != "" {
			name += " " + suffix
		}
		card := &generatedCard{Source: source, ID: strconv.FormatInt(start+int64(len(ordered)), 10), Info: info, Name: name}
		result[sourceID] = card
		ordered = append(ordered, card)
	}
	return ordered, result, nil
}
