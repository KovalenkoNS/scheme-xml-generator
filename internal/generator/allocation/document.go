// Проверка диапазонов транспортных ID выполняется до записи FBD-документа и не меняет состояние allocator.
package allocation

import (
	"fmt"
	"scheme-xml-generator/internal/generator/contracts"
)

// ValidateDocumentRanges Сверяет выделенные allocator диапазоны с потребностями подготовленного XML.
// Останавливает генерацию при недопустимых началах или переполнении ID.
func ValidateDocumentRanges(ids contracts.IDRange, requirements contracts.DocumentRequirements) error {
	if ids.T11Start < 1 || ids.CardStart < 1 || ids.POUID < 1 {
		return fmt.Errorf("начальные ID должны быть положительными")
	}
	items := []struct {
		label string
		start int64
		count int
	}{
		{"T11ID", ids.T11Start, requirements.T11Count},
		{"cardId", ids.CardStart, requirements.CardCount},
	}
	for _, item := range items {
		if item.count == 0 {
			if item.start > contracts.MaxTransportID+1 {
				return fmt.Errorf("диапазон %s выходит за signed 32-bit", item.label)
			}
			continue
		}
		if item.start > contracts.MaxTransportID || item.start > contracts.MaxTransportID-int64(item.count)+1 {
			return fmt.Errorf("диапазон %s выходит за signed 32-bit", item.label)
		}
	}
	return nil
}
