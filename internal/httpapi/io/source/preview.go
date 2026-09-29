// Предварительный просмотр фактических исходных данных без генерации.
package iosource

import (
	"net/http"
	"scheme-xml-generator/internal/iomap"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandlePLCDiagnosticPreview показывает ПЛК/модули исходного IO перед выбором HMI-диагностики.
// Возвращает iomap.Plan из XLSX без allocator и записи результатов.
func (s *Service) HandlePLCDiagnosticPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := ReadIOUpload(w, r)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := iomap.Parse(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, plan)
}
