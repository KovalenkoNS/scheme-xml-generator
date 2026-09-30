// Декодирование и маршрутизация входного контракта HTTP.
package moduleassignment

import (
	"encoding/json"
	"fmt"
	"io"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"strings"
)

// decodeModuleAssignmentRequest разбирает единственное поле config совместимого multipart-запроса IO.
// Возвращает выбор модулей/формата без неизвестных JSON-полей и постороннего хвоста.
func decodeModuleAssignmentRequest(values []string) (stassignment.ModuleMappingRequest, error) {
	var request stassignment.ModuleMappingRequest
	if len(values) != 1 || !strings.HasPrefix(strings.TrimSpace(values[0]), "{") {
		return request, fmt.Errorf("укажите настройки назначений модулей в JSON-объекте config")
	}
	decoder := json.NewDecoder(strings.NewReader(values[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("неверные настройки назначений модулей: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return request, fmt.Errorf("после настроек назначений модулей обнаружены лишние данные")
	}
	return request, nil
}
