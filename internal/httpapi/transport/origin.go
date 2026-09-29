// Проверка локального Origin до выполнения изменяющего маршрута.
package transport

import (
	"fmt"
	"net/http"

	"net/url"
)

// LocalPOST защищает изменяющие локальные HTTP-маршруты перед предметной обработкой.
// Отклоняет некорректный или нелокальный Origin, остальные запросы передаёт назначенному handler.
func LocalPOST(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed == nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
				WriteError(w, http.StatusForbidden, fmt.Errorf("запрос отклонён проверкой Origin"))
				return
			}
		}
		next(w, r)
	}
}
