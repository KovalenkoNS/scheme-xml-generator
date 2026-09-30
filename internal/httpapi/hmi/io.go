// Оркестрация HMI-диагностики по исходному IO XLSX.
package hmi

import (
	"errors"
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/generator/hmi"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	iosource "scheme-xml-generator/internal/httpapi/io/source"
	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/httpapi/output"
	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/iomap"
)

// HandlePLCDiagnosticGenerate генерирует HMI-диагностику выбранных ПЛК из исходного IO XLSX.
// Проверяет выбор и план, резервирует ID кадров/объектов и сохраняет результаты с общими предупреждениями.
func (s *Service) HandlePLCDiagnosticGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := iosource.ReadIOUpload(w, r, "diagnostic")
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeHMIContext(rawContext)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var selection struct {
		Controllers []iomap.Selection `json:"controllers"`
	}
	values := r.MultipartForm.Value["diagnostic"]
	if len(values) != 1 {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле diagnostic"))
		return
	}
	if err = decodeDiagnosticObject(values[0], &selection); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.Parse(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := hmi.PreparePLCDiagnosticPlans(source, selection.Controllers, ctx)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var t11, cards, pages int
	for _, plan := range plans {
		t11 += plan.T11Count
		cards += plan.CardCount
		pages += plan.FrameCount
	}
	if t11 > limits.MaxDocumentObjects || cards > limits.MaxDocumentCards {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("диагностика IO превышает пределы объектов одного запроса"))
		return
	}
	items := make([]output.ControllerResult, len(plans))
	_, err = s.Allocator.WithDiagnosticReservation(t11, cards, pages, func(ids xmlidentity.DiagnosticIDRange) error {
		for i, plan := range plans {
			result, err := s.Generator.GeneratePLCDiagnostic(plan, ctx, ids)
			if err != nil {
				return err
			}
			items[i] = output.ControllerResult{ControllerName: plan.ControllerName, FileName: aoDiagnosticOutputName(fileName, plan.ControllerName), Result: result}
			ids.T11Start += int64(plan.T11Count)
			ids.CardStart += int64(plan.CardCount)
			ids.PageStart += int64(plan.FrameCount)
		}
		return nil
	})
	if err != nil {
		var persistenceErr *allocation.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			transport.WriteError(w, http.StatusInternalServerError, err)
		} else {
			transport.WriteError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.Output.WriteControllerBatch(w, "diagnostic", items, source.Warnings)
}
