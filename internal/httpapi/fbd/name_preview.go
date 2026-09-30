// Предварительное имя библиотечного объекта без расходования ID.
package fbd

import (
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator/fbd"
	"scheme-xml-generator/internal/httpapi/transport"
)

// HandlePreviewName предварительно показывает имя объекта для выбранного библиотечного шаблона.
// Разрешает templateKey через Repository и возвращает PreviewName без генерации или расходования ID.
func (s *Service) HandlePreviewName(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TemplateKey string `json:"templateKey"`
		ObjectName  string `json:"objectName"`
		NameMode    string `json:"nameMode"`
	}
	if err := transport.DecodeJSON(w, r, &request); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ref, ok := s.Repository.Resolve(request.TemplateKey)
	if !ok {
		transport.WriteError(w, http.StatusNotFound, fmt.Errorf("шаблон не найден; обновите список библиотек"))
		return
	}
	preview, err := fbd.PreviewName(ref, request.ObjectName, request.NameMode)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, preview)
}
