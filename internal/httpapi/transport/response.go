// Единые JSON-ответы HTTP-компонентов.
package transport

import (
	"net/http"

	"encoding/json"
)

// WriteJSON записывает JSON-ответ предметного HTTP-компонента.
// Устанавливает content-type и переданный статус, сериализует контракт в ResponseWriter.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteError возвращает ошибку проверки или генерации в общем JSON-контракте API.
// Записывает переданный HTTP-статус и текст error без создания файлов результата.
func WriteError(w http.ResponseWriter, status int, err error) {
	WriteJSON(w, status, map[string]any{"error": err.Error()})
}
