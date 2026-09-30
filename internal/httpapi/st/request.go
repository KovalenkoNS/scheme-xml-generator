// Декодирование и маршрутизация входного контракта HTTP.
package st

import (
	"encoding/json"
	"fmt"
	"io"
	"scheme-xml-generator/internal/generator/st"
	"strings"
)

// decodeAOSTRequest разбирает поле st загруженного HTTP multipart до планирования ST.
// Требует единственный JSON-объект без неизвестных полей и лишних значений; возвращает AOSTRequest.
func decodeAOSTRequest(values []string) (st.AOSTRequest, error) {
	var request st.AOSTRequest
	if len(values) != 1 || !strings.HasPrefix(strings.TrimSpace(values[0]), "{") {
		return request, fmt.Errorf("укажите настройки ST в JSON-объекте st")
	}
	decoder := json.NewDecoder(strings.NewReader(values[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("неверные настройки ST: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return request, fmt.Errorf("после настроек ST обнаружены лишние данные")
	}
	return request, nil
}
