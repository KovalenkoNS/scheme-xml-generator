// Нормализация выбора ПЛК и настроек создаваемых технологических объектов.
package techobjectapi

import (
	"fmt"

	"io"
	"scheme-xml-generator/internal/iomap"

	"encoding/json"

	"strings"
)

type techObjectsOptions struct {
	Controllers    []iomap.Selection `json:"controllers"`
	ResourceNumber int               `json:"resourceNumber"`
}

// decodeTechObjectsOptions проверяет выбор ПЛК и номер ресурса до генерации XLS объектов.
// Возвращает нормализованные настройки с ресурсом 1 по умолчанию, отклоняя null/неизвестные поля.
func decodeTechObjectsOptions(raw string) (techObjectsOptions, error) {
	options := techObjectsOptions{ResourceNumber: 1}
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return options, fmt.Errorf("настройки технологических объектов должны быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return options, fmt.Errorf("неверные настройки технологических объектов: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return options, fmt.Errorf("после настроек технологических объектов обнаружены лишние данные")
	}
	// encoding/json accepts null for scalar strings and integers. Reject it in
	// every field, including controller entries, instead of silently using defaults.
	var fields any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return options, err
	}
	var rejectNull func(any) error
	rejectNull = func(value any) error {
		switch value := value.(type) {
		case nil:
			return fmt.Errorf("поля настроек технологических объектов не могут быть null")
		case map[string]any:
			for _, child := range value {
				if err := rejectNull(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := rejectNull(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := rejectNull(fields); err != nil {
		return options, err
	}
	return options, nil
}
