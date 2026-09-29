// Декодирование и маршрутизация входного контракта HTTP.
package transport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DecodeJSON декодирует контракт JSON-запроса FBD до обращения к библиотеке и allocator.
// Ограничивает тело 8 МиБ, отклоняет неизвестные поля/лишние значения и закрывает body.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	const maxJSONBodyBytes int64 = 8 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("неверный JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("после JSON обнаружены лишние данные")
	}
	return nil
}
