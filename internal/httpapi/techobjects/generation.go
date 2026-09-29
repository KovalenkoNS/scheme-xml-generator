// Оркестрация генерации технологических XLS после проверки входного IO.
package techobjectapi

import (
	"fmt"
	"net/http"

	"scheme-xml-generator/internal/iomap"

	"scheme-xml-generator/internal/httpapi/output"

	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/techobjects"
)

// HandleTechObjectsGenerate создаёт XLS технологических объектов для выбранных ПЛК.
// Проверяет IO и настройки, строит независимые таблицы и передаёт весь пакет output.Store.
func (s *Service) HandleTechObjectsGenerate(w http.ResponseWriter, r *http.Request) {
	data, name, err := readTechObjectsUpload(w, r, true)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	values := r.MultipartForm.Value["objects"]
	if len(values) != 1 {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле objects"))
		return
	}
	options, err := decodeTechObjectsOptions(values[0])
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.ParseInventory(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := techobjects.Prepare(source, options.Controllers, options.ResourceNumber)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	// Validate and serialize the whole batch before creating any output files.
	// This path is independent of XML generation and its persistent ID allocator.
	items := make([]output.TechObjectsBatchItem, len(plans))
	for i, plan := range plans {
		content, err := techobjects.Generate(plan)
		if err != nil {
			transport.WriteError(w, http.StatusBadRequest, err)
			return
		}
		items[i] = output.TechObjectsBatchItem{ControllerName: plan.ControllerName, FileName: techObjectsOutputName(name, plan.ControllerName), Data: content, Summary: plan.Summary}
	}
	s.Output.WriteTechObjectsBatch(w, items, source.Warnings)
}
