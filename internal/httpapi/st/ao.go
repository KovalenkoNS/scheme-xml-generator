// Оркестрация предметной генерации по AO-карте.
package st

import (
	"errors"
	"scheme-xml-generator/internal/aomap"

	"net/http"
	"scheme-xml-generator/internal/generator"

	ioao "scheme-xml-generator/internal/httpapi/io/ao"

	"scheme-xml-generator/internal/httpapi/output"

	"scheme-xml-generator/internal/httpapi/transport"
)

// HandleAOSTGenerate выполняет отдельный ST-сценарий из AO-карты и настроек перекладки.
// Проверяет выбранные группы/ModuleID, резервирует только POU и сохраняет пакет XML со STCODE.
func (s *Service) HandleAOSTGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := ioao.ReadAOMappingUpload(w, r, "st")
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := ioao.DecodeProgramContext(rawContext)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	request, err := decodeAOSTRequest(r.MultipartForm.Value["st"])
	if err != nil {
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
	controllerPlans, err := generator.PrepareAOSTPlans(plan, request)
	if err != nil {
		transport.WriteError(w, http.StatusBadRequest, err)
		return
	}
	pouCount := 0
	for _, part := range controllerPlans {
		pouCount += len(part.POUs)
	}
	items := make([]output.ControllerResult, len(controllerPlans))
	// All validation and all ST documents succeed before IDs are committed.
	// ST has no graphic T11/card records: only the POU cursor advances.
	_, err = s.Allocator.WithReservation(0, 0, pouCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		for index, part := range controllerPlans {
			result, err := s.Generator.GenerateAOST(part, ctx, ids)
			if err != nil {
				return err
			}
			items[index] = output.ControllerResult{ControllerName: part.ControllerName, FileName: aoSTOutputName(fileName, part.ControllerName), Result: result, AssignmentCount: part.AssignmentCount, RepeatedAssignmentCount: part.RepeatedAssignmentCount}
			ids.POUID += int64(len(part.POUs))
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
	s.Output.WriteControllerBatch(w, "st", items, plan.Warnings)
}
