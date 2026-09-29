// Оркестрация предметной генерации по AO-карте.
package hmi

import (
	"errors"
	"fmt"
	"net/http"
	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/generator"

	ioao "scheme-xml-generator/internal/httpapi/io/ao"

	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/httpapi/output"

	"scheme-xml-generator/internal/httpapi/transport"
)

type aoDiagnosticRequest struct {
	ControllerNames []string `json:"fcs"` // Прежний JSON-массив выбора ПЛК сохраняется для совместимости.
}

// HandleAODiagnosticGenerate строит HMI-кадры диагностики выбранных ПЛК по AO-карте.
// Проверяет выбор/контекст, резервирует графические ID и кадры, затем сохраняет пакет через output.Store.
func (s *Service) HandleAODiagnosticGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := ioao.ReadAOMappingUpload(w, r, "diagnostic")
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeHMIContext(rawContext)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var selection aoDiagnosticRequest
	values := r.MultipartForm.Value["diagnostic"]
	if len(values) != 1 {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле diagnostic"))
		return
	}
	if err := decodeDiagnosticObject(values[0], &selection); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := aomap.Parse(data)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	if err := ioao.CheckAOMappingSize(plan); err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := generator.PrepareAODiagnosticPlans(plan, selection.ControllerNames, ctx)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var t11Count, cardCount, frameCount int
	for _, part := range plans {
		t11Count += part.T11Count
		cardCount += part.CardCount
		frameCount += part.FrameCount
	}
	if t11Count > limits.MaxDocumentObjects || cardCount > limits.MaxDocumentCards {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("диагностика превышает предел примитивов или карточек одного запроса"))
		return
	}
	items := make([]output.ControllerResult, len(plans))
	_, err = s.Allocator.WithDiagnosticReservation(t11Count, cardCount, frameCount, func(ids generator.DiagnosticIDRange) error {
		for index, part := range plans {
			result, err := s.Generator.GenerateAODiagnostic(part, ctx, ids)
			if err != nil {
				return err
			}
			items[index] = output.ControllerResult{ControllerName: part.ControllerName, FileName: aoDiagnosticOutputName(fileName, part.ControllerName), Result: result}
			ids.T11Start += int64(part.T11Count)
			ids.CardStart += int64(part.CardCount)
			ids.PageStart += int64(part.FrameCount)
		}
		return nil
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			transport.WriteError(w, http.StatusInternalServerError, err)
		} else {
			transport.WriteError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.Output.WriteControllerBatch(w, "diagnostic", items, plan.Warnings)
}
