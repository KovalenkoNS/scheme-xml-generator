// Текущие настройки генератора и явный статус неподключённого контракта Host.
package workspace

import (
	"net/http"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/httpapi/transport"
)

type Service struct{ Generator *generator.Generator }

// HandleWorkspace показывает интерфейсу фактические Common/Page настройки генератора.
// Возвращает текущий Config и честный статус Host contract-pending без подмены серверных данных локальными.
func (s *Service) HandleWorkspace(w http.ResponseWriter, _ *http.Request) {
	transport.WriteJSON(w, http.StatusOK, map[string]any{
		"common": s.Generator.Config.Common,
		"page":   s.Generator.Config.Page,
		"host":   map[string]any{"connected": false, "status": "contract-pending"},
	})
}
