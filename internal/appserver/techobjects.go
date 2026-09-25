package appserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"scheme-xml-generator/internal/iomap"
	"scheme-xml-generator/internal/techobjects"
)

type techObjectsOptions struct {
	Controllers    []iomap.Selection `json:"controllers"`
	ResourceNumber int               `json:"resourceNumber"`
}

type techObjectsGeneratedFile struct {
	FCS      string              `json:"fcs"`
	FileName string              `json:"fileName"`
	URL      string              `json:"url"`
	Summary  techobjects.Summary `json:"summary"`
}

type techObjectsResponse struct {
	Files    []techObjectsGeneratedFile `json:"files"`
	Summary  techobjects.Summary        `json:"summary"`
	Warnings []string                   `json:"warnings"`
}

type techObjectsBatchItem struct {
	FCS, FileName string
	Data          []byte
	Summary       techobjects.Summary
}

func readTechObjectsUpload(w http.ResponseWriter, r *http.Request, generate bool) ([]byte, string, error) {
	data, name, _, err := readMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", "objects")
	if err != nil {
		return nil, "", err
	}
	if !strings.EqualFold(filepath.Ext(r.MultipartForm.File["file"][0].Filename), ".xlsx") {
		return nil, "", fmt.Errorf("для технологических объектов нужен исходный IO в формате .xlsx")
	}
	for field := range r.MultipartForm.Value {
		if field != "fileName" && !(generate && field == "objects") {
			return nil, "", fmt.Errorf("неподдерживаемое поле загрузки %q", field)
		}
	}
	return data, name, nil
}

func decodeTechObjectsOptions(raw string) (techObjectsOptions, error) {
	options := techObjectsOptions{ResourceNumber: 1}
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return options, fmt.Errorf("настройки технологических объектов должны быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&options); err != nil {
		return options, fmt.Errorf("неверные настройки технологических объектов: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return options, fmt.Errorf("после настроек технологических объектов обнаружены лишние данные")
	}
	// encoding/json accepts null for scalar strings and integers. Reject it in
	// every field, including controller entries, instead of silently using defaults.
	var fields any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return options, err
	}
	var rejectNull func(any) error
	rejectNull = func(value any) error {
		switch value := value.(type) {
		case nil:
			return fmt.Errorf("поля настроек технологических объектов не могут быть null")
		case map[string]any:
			for _, child := range value {
				if err := rejectNull(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := rejectNull(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := rejectNull(fields); err != nil {
		return options, err
	}
	return options, nil
}

func (s *Server) handleTechObjectsPreview(w http.ResponseWriter, r *http.Request) {
	data, _, err := readTechObjectsUpload(w, r, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.ParseInventory(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	controllers, err := techobjects.Preview(source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"controllers": controllers, "warnings": source.Warnings})
}

func (s *Server) handleTechObjectsGenerate(w http.ResponseWriter, r *http.Request) {
	data, name, err := readTechObjectsUpload(w, r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	values := r.MultipartForm.Value["objects"]
	if len(values) != 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("укажите выбранные ПЛК в поле objects"))
		return
	}
	options, err := decodeTechObjectsOptions(values[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	source, err := iomap.ParseInventory(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plans, err := techobjects.Prepare(source, options.Controllers, options.ResourceNumber)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Validate and serialize the whole batch before creating any output files.
	// This path is independent of XML generation and its persistent ID allocator.
	items := make([]techObjectsBatchItem, len(plans))
	for i, plan := range plans {
		content, err := techobjects.Generate(plan)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		items[i] = techObjectsBatchItem{FCS: plan.FCS, FileName: techObjectsOutputName(name, plan.FCS), Data: content, Summary: plan.Summary}
	}
	s.writeTechObjectsBatch(w, items, source.Warnings)
}

func techObjectsOutputName(requested, fcs string) string {
	if strings.TrimSpace(requested) == "" {
		requested = "TechObjects"
	}
	base := strings.TrimSuffix(safeOutputName(requested, "TechObjects", ""), ".xml")
	controller := strings.TrimSuffix(safeOutputName(fcs, "PLC", ""), ".xml")
	return base + "_" + controller + ".xls"
}

func (s *Server) writeTechObjectsBatch(w http.ResponseWriter, items []techObjectsBatchItem, warnings []string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	response := techObjectsResponse{Files: make([]techObjectsGeneratedFile, 0, len(items)), Warnings: append([]string{}, warnings...)}
	created := make([]string, 0, len(items))
	fail := func(err error) {
		for _, path := range created {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				s.logger.Printf("cannot roll back generated technological objects file %s: %v", path, removeErr)
				err = errors.Join(err, fmt.Errorf("не удалось удалить незавершённый результат %s: %w", filepath.Base(path), removeErr))
			}
		}
		writeError(w, http.StatusInternalServerError, err)
	}
	for _, item := range items {
		name, err := uniqueOutputName(s.outputDir, item.FileName)
		if err != nil {
			fail(err)
			return
		}
		path := filepath.Join(s.outputDir, name)
		if err := writeAtomic(path, item.Data); err != nil {
			fail(fmt.Errorf("записать XLS для %s: %w", item.FCS, err))
			return
		}
		created = append(created, path)
		response.Files = append(response.Files, techObjectsGeneratedFile{FCS: item.FCS, FileName: name, URL: "/api/output/" + url.PathEscape(name), Summary: item.Summary})
		response.Summary.ModuleCount += item.Summary.ModuleCount
		response.Summary.ReserveCount += item.Summary.ReserveCount
		response.Summary.ObjectCount += item.Summary.ObjectCount
	}
	for _, file := range response.Files {
		s.logger.Printf("generated technological objects file %s for %s with %d modules and %d reserves", file.FileName, file.FCS, file.Summary.ModuleCount, file.Summary.ReserveCount)
	}
	writeJSON(w, http.StatusCreated, response)
}
