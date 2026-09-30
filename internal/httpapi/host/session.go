// HTTP-адаптер текущего соединения с Host; не выполняет авторизацию или операции IO.
package host

import (
	"net/http"
	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/integration/hostclient"
)

type Service struct{ Client *hostclient.Client }

// HandleSession отдаёт UI свежий безопасный снимок Host/Server, в том числе при ошибке связи.
// Запрет кэширования не позволяет браузеру подменить новый выход прежним успешным входом.
func (s *Service) HandleSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	transport.WriteJSON(w, http.StatusOK, s.Client.Session(r.Context()))
}
