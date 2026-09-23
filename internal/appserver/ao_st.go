package appserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/generator"
)

// ST is a separate opt-in route: FBD output keeps its deduplication rules.
func (s *Server) handleAOSTGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := readAOMappingUpload(w, r, "st")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeAOMappingContext(rawContext)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request, err := decodeAOSTRequest(r.MultipartForm.Value["st"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := aomap.Parse(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := checkAOMappingSize(plan); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	controllerPlans, err := generator.PrepareAOSTPlans(plan, request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	pouCount := 0
	for _, part := range controllerPlans {
		pouCount += len(part.POUs)
	}
	items := make([]aoBatchItem, len(controllerPlans))
	// All validation and all ST documents succeed before IDs are committed.
	// ST has no graphic T11/card records: only the POU cursor advances.
	_, err = s.allocator.WithReservation(0, 0, pouCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		for index, part := range controllerPlans {
			result, err := s.generator.GenerateAOST(part, ctx, ids)
			if err != nil {
				return err
			}
			items[index] = aoBatchItem{FCS: part.FCS, FileName: aoSTOutputName(fileName, part.FCS), Result: result, AssignmentCount: part.AssignmentCount, RepeatedAssignmentCount: part.RepeatedAssignmentCount}
			ids.POUID += int64(len(part.POUs))
		}
		return nil
	})
	if err != nil {
		var persistenceErr *generator.AllocatorPersistenceError
		if errors.As(err, &persistenceErr) {
			writeError(w, http.StatusInternalServerError, err)
		} else {
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	s.writeAOBatch(w, "st", items, plan.Warnings)
}

func decodeAOSTRequest(values []string) (generator.AOSTRequest, error) {
	var request generator.AOSTRequest
	if len(values) != 1 || !strings.HasPrefix(strings.TrimSpace(values[0]), "{") {
		return request, fmt.Errorf("укажите настройки ST в JSON-объекте st")
	}
	decoder := json.NewDecoder(strings.NewReader(values[0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("неверные настройки ST: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return request, fmt.Errorf("после настроек ST обнаружены лишние данные")
	}
	return request, nil
}

func aoSTOutputName(requestedName, fcs string) string {
	base := strings.TrimSuffix(safeOutputName(requestedName, "AO", "mapping"), ".xml")
	controller := strings.TrimSuffix(safeOutputName(fcs, "FCS", "mapping"), ".xml")
	return base + "_ST_" + controller + ".xml"
}
