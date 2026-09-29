// Предварительный просмотр фактических исходных данных без генерации.
package modulemapping

import (
	"net/http"

	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/inputs/assignments"
)

// HandleModuleMappingPreview возвращает структуру локальных назначений ПЛК до генерации.
// Разбирает XLSX и проверяет группы/каналы, не создавая XML и не резервируя ID.
func (s *Service) HandleModuleMappingPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := ReadModuleMappingUpload(w, r)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := assignments.Parse(data)
	if err == nil {
		err = CheckModuleMappingSize(plan)
	}
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, plan)
}
