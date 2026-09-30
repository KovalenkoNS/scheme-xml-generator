// Проверка явного контекста локальной IO-карты без изменения Config.
package ioctx

import (
	"encoding/json"
	"fmt"
	"io"
	programcontext "scheme-xml-generator/internal/generator/program"
	"strings"
)

// DecodeMappingContextWithDefault объединяет явно заданные строковые настройки с переданным профилем IO.
// Проверяет единственный JSON-объект и запрет null, возвращает новый контекст без изменения Config.
func DecodeMappingContextWithDefault(raw string, result programcontext.ProgramContext) (programcontext.ProgramContext, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return result, nil
	}
	if !strings.HasPrefix(raw, "{") {
		return result, fmt.Errorf("контекст проекта должен быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("неверный контекст проекта: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return result, fmt.Errorf("после контекста проекта обнаружены лишние данные")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return result, fmt.Errorf("неверный контекст проекта: %w", err)
	}
	for name, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			return result, fmt.Errorf("поле контекста %s должно быть строкой, а не null", name)
		}
	}
	return result, nil
}
