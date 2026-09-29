// Предварительный просмотр фактических исходных данных без генерации.
package ioao

import (
	"scheme-xml-generator/internal/aomap"

	"net/http"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleAOPreview показывает разобранную AO-карту до выбора генерации FBD/ST/HMI.
// Читает ограниченный TXT, проверяет размеры плана и возвращает группы/каналы без расходования ID.
func (s *Service) HandleAOPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := ReadAOMappingUpload(w, r)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := aomap.Parse(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := CheckAOMappingSize(plan); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, plan)
}
