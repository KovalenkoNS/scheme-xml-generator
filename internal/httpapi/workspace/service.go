// Текущие настройки генератора; снимок соединения Host предоставляет отдельный интеграционный клиент.
package workspace

import (
	"net/http"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/integration/hostclient"
)

type Service struct {
	Config *config.Config
	Host   *hostclient.Client
}

// HandleWorkspace показывает интерфейсу фактические Common/Page настройки генератора.
// Возвращает текущий Config и фактическое состояние Host, не утверждая готовность IO-контракта.
func (s *Service) HandleWorkspace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"common": s.Config.Common,
		"page":   s.Config.Page,
		"host":   s.Host.Session(r.Context()),
	})
}
