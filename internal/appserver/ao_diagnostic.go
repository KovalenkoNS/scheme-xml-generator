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

type aoDiagnosticRequest struct {
	FCS []string `json:"fcs"`
}

func (s *Server) handleAODiagnosticGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := readAOMappingUpload(w, r, "diagnostic")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeAODiagnosticContext(rawContext)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var selection aoDiagnosticRequest
	values := r.MultipartForm.Value["diagnostic"]
	if len(values) != 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле diagnostic"))
		return
	}
	if err := decodeDiagnosticObject(values[0], &selection); err != nil {
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
	plans, err := generator.PrepareAODiagnosticPlans(plan, selection.FCS, ctx)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var t11Count, cardCount, frameCount int
	for _, part := range plans {
		t11Count += part.T11Count
		cardCount += part.CardCount
		frameCount += part.FrameCount
	}
	if t11Count > maxDocumentObjects || cardCount > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("диагностика превышает предел примитивов или карточек одного запроса"))
		return
	}
	items := make([]aoBatchItem, len(plans))
	_, err = s.allocator.WithDiagnosticReservation(t11Count, cardCount, frameCount, func(ids generator.DiagnosticIDRange) error {
		for index, part := range plans {
			result, err := s.generator.GenerateAODiagnostic(part, ctx, ids)
			if err != nil {
				return err
			}
			items[index] = aoBatchItem{FCS: part.FCS, FileName: aoDiagnosticOutputName(fileName, part.FCS), Result: result}
			ids.T11Start += int64(part.T11Count)
			ids.CardStart += int64(part.CardCount)
			ids.PageStart += int64(part.FrameCount)
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
	s.writeAOBatch(w, "diagnostic", items, plan.Warnings)
}

func decodeDiagnosticObject(raw string, target any) error {
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return fmt.Errorf("настройки диагностики должны быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("неверные настройки диагностики: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("после настроек диагностики обнаружены лишние данные")
	}
	return nil
}

func decodeAODiagnosticContext(raw string) (generator.AODiagnosticContext, error) {
	ctx := generator.DefaultAODiagnosticContext()
	if strings.TrimSpace(raw) == "" {
		return ctx, nil
	}
	if err := decodeDiagnosticObject(raw, &ctx); err != nil {
		return ctx, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return ctx, err
	}
	for key, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			return ctx, fmt.Errorf("поле контекста %s должно быть строкой, а не null", key)
		}
	}
	return ctx, nil
}

func aoDiagnosticOutputName(requestedName, fcs string) string {
	base := strings.TrimSuffix(safeOutputName(requestedName, "AO", "mapping"), ".xml")
	controller := strings.TrimSuffix(safeOutputName(fcs, "FCS", "mapping"), ".xml")
	return base + "_diagnostic_" + controller + ".xml"
}
