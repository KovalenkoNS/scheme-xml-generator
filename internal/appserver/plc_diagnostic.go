package appserver

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/iomap"
)

func readIOUpload(w http.ResponseWriter, r *http.Request, fields ...string) ([]byte, string, string, error) {
	data, name, ctx, err := readMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", fields...)
	if err == nil && !strings.EqualFold(filepath.Ext(r.MultipartForm.File["file"][0].Filename), ".xlsx") {
		err = fmt.Errorf("для диагностики ПЛК нужен Excel IO в формате .xlsx, а не TXT или перекладка")
	}
	return data, name, ctx, err
}

func (s *Server) handlePLCDiagnosticPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := readIOUpload(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := iomap.Parse(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handlePLCDiagnosticGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := readIOUpload(w, r, "diagnostic")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, err := decodeAODiagnosticContext(rawContext)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var selection struct {
		Controllers []iomap.Selection `json:"controllers"`
	}
	values := r.MultipartForm.Value["diagnostic"]
	if len(values) != 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле diagnostic"))
		return
	}
	if err = decodeDiagnosticObject(values[0], &selection); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.Parse(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := generator.PreparePLCDiagnosticPlans(source, selection.Controllers, ctx)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var t11, cards, pages int
	for _, plan := range plans {
		t11 += plan.T11Count
		cards += plan.CardCount
		pages += plan.FrameCount
	}
	if t11 > maxDocumentObjects || cards > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("диагностика IO превышает пределы объектов одного запроса"))
		return
	}
	items := make([]aoBatchItem, len(plans))
	_, err = s.allocator.WithDiagnosticReservation(t11, cards, pages, func(ids generator.DiagnosticIDRange) error {
		for i, plan := range plans {
			result, err := s.generator.GeneratePLCDiagnostic(plan, ctx, ids)
			if err != nil {
				return err
			}
			items[i] = aoBatchItem{FCS: plan.FCS, FileName: aoDiagnosticOutputName(fileName, plan.FCS), Result: result}
			ids.T11Start += int64(plan.T11Count)
			ids.CardStart += int64(plan.CardCount)
			ids.PageStart += int64(plan.FrameCount)
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
	s.writeAOBatch(w, "diagnostic", items, source.Warnings)
}
