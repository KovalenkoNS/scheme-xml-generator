// Проверка явно заданного контекста генерации.
package hmi

import (
	"encoding/json"
	"fmt"
	"io"
	"scheme-xml-generator/internal/generator/hmi"
	"strings"
)

// decodeDiagnosticObject читает настройки диагностики из текстового поля multipart.
// Требует JSON-объект без неизвестных полей и хвоста, заполняет переданный контракт HMI.
func decodeDiagnosticObject(raw string, target any) error {
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return fmt.Errorf("настройки диагностики должны быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("неверные настройки диагностики: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("после настроек диагностики обнаружены лишние данные")
	}
	return nil
}

// decodeHMIContext объединяет явный контекст диагностики с текущим подтверждённым профилем.
// Возвращает HMIContext; null вместо строк и некорректный JSON отклоняются до генерации.
func decodeHMIContext(raw string) (hmi.HMIContext, error) {
	ctx := hmi.DefaultHMIContext()
	if strings.TrimSpace(raw) == "" {
		return ctx, nil
	}
	if err := decodeDiagnosticObject(raw, &ctx); err != nil {
		return ctx, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return ctx, err
	}
	for key, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			return ctx, fmt.Errorf("поле контекста %s должно быть строкой, а не null", key)
		}
	}
	return ctx, nil
}
