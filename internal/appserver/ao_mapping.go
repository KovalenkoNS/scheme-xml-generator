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

	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/generator"
)

const (
	maxAOMappingFileBytes int64 = 2 << 20
	maxAOMultipartExtra   int64 = 128 << 10
)

type aoGeneratedFile struct {
	FCS string `json:"fcs"`
	generateResponse
}

type aoBatchSummary struct {
	FrameCount              int `json:"frameCount,omitempty"`
	Graphics                int `json:"graphics,omitempty"`
	POUCount                int `json:"pouCount"`
	SignalCount             int `json:"signalCount"`
	Blocks                  int `json:"blocks"`
	Cards                   int `json:"cards"`
	IOModuleCount           int `json:"ioModuleCount"`
	SkippedDuplicateCount   int `json:"skippedDuplicateCount"`
	AssignmentCount         int `json:"assignmentCount,omitempty"`
	RepeatedAssignmentCount int `json:"repeatedAssignmentCount,omitempty"`
}

type aoGenerateResponse struct {
	Kind     string            `json:"kind,omitempty"`
	Files    []aoGeneratedFile `json:"files"`
	Summary  aoBatchSummary    `json:"summary"`
	Warnings []string          `json:"warnings"`
}

func (s *Server) handleAOProfile(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"context":     generator.DefaultAOMappingContext(),
		"description": "AN_v1 по A11_00_example.xml: отдельный XML на каждый FCS, POU AO_A11, AO_A12 и далее; четыре позиции на модуль, свободные каналы заполняются резервами. Повторный тег внутри FCS оставляет пустое место без блока и заглушки; основной вызов сохраняется в Main_module, если он указан в карте. Каждый XML импортируется в соответствующий ПЛК. Числовые ID переназначает SCADA; физические связи не создаются.",
	})
}

func (s *Server) handleAOPreview(w http.ResponseWriter, r *http.Request) {
	data, _, _, err := readAOMappingUpload(w, r)
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
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleAOGenerate(w http.ResponseWriter, r *http.Request) {
	data, fileName, rawContext, err := readAOMappingUpload(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	context, err := decodeAOMappingContext(rawContext)
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
	plans := aomap.SplitByFCS(plan)
	requirements := make([]generator.DocumentRequirements, len(plans))
	var total generator.DocumentRequirements
	for index, controllerPlan := range plans {
		requirements[index], err = generator.RequirementsForAOMap(controllerPlan)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		total.T11Count += requirements[index].T11Count
		total.CardCount += requirements[index].CardCount
		total.POUCount += requirements[index].POUCount
		total.SignalCount += requirements[index].SignalCount
	}
	if total.T11Count > maxDocumentObjects || total.CardCount > maxDocumentCards {
		writeError(w, http.StatusBadRequest, fmt.Errorf("карта AO слишком велика: не более %d блоков и %d карточек", maxDocumentObjects, maxDocumentCards))
		return
	}
	results := make([]generator.Result, len(plans))
	// Build every controller document before committing IDs or writing any file.
	// Cards are scoped to a controller, even when two FCS reuse the same tag.
	_, err = s.allocator.WithReservation(total.T11Count, total.CardCount, total.POUCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		for index, controllerPlan := range plans {
			var generateErr error
			results[index], generateErr = s.generator.GenerateAOMap(controllerPlan, context, ids)
			if generateErr != nil {
				return generateErr
			}
			ids.T11Start += int64(requirements[index].T11Count)
			ids.CardStart += int64(requirements[index].CardCount)
			ids.POUID += int64(requirements[index].POUCount)
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
	s.writeAOResults(w, fileName, plans, results, plan.Warnings)
}

func (s *Server) writeAOResults(w http.ResponseWriter, requestedName string, plans []*aomap.Plan, results []generator.Result, sourceWarnings []string) {
	items := make([]aoBatchItem, len(results))
	for index, result := range results {
		fcs := plans[index].Groups[0].FCS
		items[index] = aoBatchItem{FCS: fcs, FileName: aoOutputName(requestedName, fcs), Result: result, SkippedDuplicateCount: plans[index].DuplicateCount}
	}
	s.writeAOBatch(w, "", items, sourceWarnings)
}

type aoBatchItem struct {
	FCS, FileName                                                   string
	Result                                                          generator.Result
	SkippedDuplicateCount, AssignmentCount, RepeatedAssignmentCount int
}

// Each new result is written atomically. If a later write fails, remove only
// the files created by this batch, never pre-existing user files.
func (s *Server) writeAOBatch(w http.ResponseWriter, kind string, items []aoBatchItem, sourceWarnings []string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	response := aoGenerateResponse{Kind: kind, Files: make([]aoGeneratedFile, 0, len(items)), Warnings: []string{}}
	created := make([]string, 0, len(items))
	fail := func(err error) {
		for _, path := range created {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				s.logger.Printf("cannot roll back generated AO file %s: %v", path, removeErr)
				err = errors.Join(err, fmt.Errorf("не удалось удалить незавершённый результат %s: %w", filepath.Base(path), removeErr))
			}
		}
		writeError(w, http.StatusInternalServerError, err)
	}
	warnings := map[string]bool{}
	for _, warning := range sourceWarnings {
		if !warnings[warning] {
			response.Warnings = append(response.Warnings, warning)
			warnings[warning] = true
		}
	}
	for _, item := range items {
		fcs, result := item.FCS, item.Result
		fileName, err := uniqueOutputName(s.outputDir, item.FileName)
		if err != nil {
			fail(err)
			return
		}
		path := filepath.Join(s.outputDir, fileName)
		if err := writeAtomic(path, result.XML); err != nil {
			fail(fmt.Errorf("записать XML для %s: %w", fcs, err))
			return
		}
		created = append(created, path)
		response.Files = append(response.Files, aoGeneratedFile{FCS: fcs, generateResponse: generateResponse{
			FileName: fileName, URL: "/api/output/" + url.PathEscape(fileName), BaseName: result.BaseName,
			Summary: result.Summary, Warnings: result.Warnings,
		}})
		response.Summary.POUCount += result.Summary.POUCount
		response.Summary.FrameCount += result.Summary.FrameCount
		response.Summary.Graphics += result.Summary.Graphics
		response.Summary.SignalCount += result.Summary.SignalCount
		response.Summary.Blocks += result.Summary.Blocks
		response.Summary.Cards += result.Summary.Cards
		response.Summary.IOModuleCount += result.Summary.IOModuleCount
		response.Summary.SkippedDuplicateCount += item.SkippedDuplicateCount
		response.Summary.AssignmentCount += item.AssignmentCount
		response.Summary.RepeatedAssignmentCount += item.RepeatedAssignmentCount
		for _, warning := range result.Warnings {
			if !warnings[warning] {
				response.Warnings = append(response.Warnings, warning)
				warnings[warning] = true
			}
		}
	}
	for _, file := range response.Files {
		s.logger.Printf("generated AO file %s for %s with %d POU and %d signals", file.FileName, file.FCS, file.Summary.POUCount, file.Summary.SignalCount)
	}
	writeJSON(w, http.StatusCreated, response)
}

func aoOutputName(requestedName, fcs string) string {
	base := strings.TrimSuffix(safeOutputName(requestedName, "AO", "mapping"), ".xml")
	controller := strings.TrimSuffix(safeOutputName(fcs, "FCS", "mapping"), ".xml")
	// Sanitize the two parts separately so long user names cannot truncate FCS.
	return base + "_" + controller + ".xml"
}

func checkAOMappingSize(plan *aomap.Plan) error {
	if len(plan.Groups) > maxDocumentPOUs || plan.ModuleCount > maxDocumentModules || plan.ChannelCount > maxDocumentSignals {
		return fmt.Errorf("карта AO превышает пределы одного запроса: %d POU, %d модулей, %d каналов", maxDocumentPOUs, maxDocumentModules, maxDocumentSignals)
	}
	return nil
}

// The whole multipart body is bounded, including text fields and headers.
// Cleanup is registered before parsing so temporary files are removed on errors.
func readAOMappingUpload(w http.ResponseWriter, r *http.Request, additionalFields ...string) ([]byte, string, string, error) {
	return readMappingUpload(w, r, maxAOMappingFileBytes, "TXT", "2", additionalFields...)
}

func readMappingUpload(w http.ResponseWriter, r *http.Request, limit int64, format, sizeLabel string, additionalFields ...string) ([]byte, string, string, error) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, limit+maxAOMultipartExtra)
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(limit); err != nil {
		return nil, "", "", fmt.Errorf("не удалось прочитать загрузку %s (не более %s МиБ): %w", format, sizeLabel, err)
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		return nil, "", "", fmt.Errorf("необходимо загрузить ровно один %s в поле file", format)
	}
	allowed := map[string]bool{"fileName": true, "context": true}
	for _, name := range additionalFields {
		allowed[name] = true
	}
	for name, values := range r.MultipartForm.Value {
		if !allowed[name] || len(values) != 1 {
			return nil, "", "", fmt.Errorf("неподдерживаемое или повторное поле загрузки %q", name)
		}
		if len(values[0]) > 64<<10 {
			return nil, "", "", fmt.Errorf("поле %s превышает 64 КиБ", name)
		}
	}
	value := func(name string) string {
		if values := r.MultipartForm.Value[name]; len(values) == 1 {
			return values[0]
		}
		return ""
	}
	fileName := value("fileName")
	rawContext := value("context")
	if len(fileName) > 512 || len(rawContext) > 8<<10 {
		return nil, "", "", fmt.Errorf("слишком длинное имя файла или контекст проекта")
	}
	header := r.MultipartForm.File["file"][0]
	if header.Size > limit {
		return nil, "", "", fmt.Errorf("%s превышает %s МиБ", format, sizeLabel)
	}
	file, err := header.Open()
	if err != nil {
		return nil, "", "", fmt.Errorf("не удалось открыть загруженный %s: %w", format, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("не удалось прочитать загруженный %s: %w", format, err)
	}
	if int64(len(data)) > limit {
		return nil, "", "", fmt.Errorf("%s превышает %s МиБ", format, sizeLabel)
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("загруженный %s пуст", format)
	}
	return data, fileName, rawContext, nil
}

func decodeAOMappingContext(raw string) (generator.AOMappingContext, error) {
	return decodeMappingContextWithDefault(raw, generator.DefaultAOMappingContext())
}

func decodeMappingContextWithDefault(raw string, result generator.AOMappingContext) (generator.AOMappingContext, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return result, nil
	}
	if !strings.HasPrefix(raw, "{") {
		return result, fmt.Errorf("контекст проекта должен быть JSON-объектом")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("неверный контекст проекта: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return result, fmt.Errorf("после контекста проекта обнаружены лишние данные")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return result, fmt.Errorf("неверный контекст проекта: %w", err)
	}
	for name, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			return result, fmt.Errorf("поле контекста %s должно быть строкой, а не null", name)
		}
	}
	return result, nil
}
