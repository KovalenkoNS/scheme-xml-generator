// Предварительный просмотр фактических исходных данных без генерации.
package techobjectapi

import (
	"net/http"

	"scheme-xml-generator/internal/iomap"

	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/techobjects"
)

// HandleTechObjectsPreview показывает доступные ПЛК исходного IO в сценарии технологических объектов.
// Разбирает XLSX и возвращает структуру выбора без создания XLS и расходования XML ID.
func (s *Service) HandleTechObjectsPreview(w http.ResponseWriter, r *http.Request) {
	data, _, err := readTechObjectsUpload(w, r, false)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.ParseInventory(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	controllers, err := techobjects.Preview(source)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	transport.WriteJSON(w, http.StatusOK, map[string]any{"controllers": controllers, "warnings": source.Warnings})
}
