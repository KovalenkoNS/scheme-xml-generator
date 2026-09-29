// Ограничение браузерных источников и встраивания локального интерфейса.
package transport

import (
	"net/http"
)

// SecurityHeaders добавляет браузерные защитные заголовки к локальному интерфейсу и API.
// Ограничивает источники ресурсов текущим приложением и запрещает встраивание страницы в чужой frame.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
