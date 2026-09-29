// Атомарное сохранение XML-пакета по ПЛК и общий контракт результата.
package output

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"scheme-xml-generator/internal/generator"

	"os"

	"scheme-xml-generator/internal/httpapi/transport"

	"net/url"
)

type GeneratedControllerFile struct {
	ControllerName string `json:"fcs"` // Сохраняет прежнее имя поля результата HTTP.
	GenerateResponse
}

type BatchSummary struct {
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

type BatchResponse struct {
	Kind     string                    `json:"kind,omitempty"`
	Files    []GeneratedControllerFile `json:"files"`
	Summary  BatchSummary              `json:"summary"`
	Warnings []string                  `json:"warnings"`
}

type ControllerResult struct {
	ControllerName, FileName                                        string
	Result                                                          generator.Result
	SkippedDuplicateCount, AssignmentCount, RepeatedAssignmentCount int
}

// WriteControllerBatch сохраняет XML-пакет предметных FBD/ST/HMI обработчиков по ПЛК.
// Назначает свободные имена под общей блокировкой; при ошибке откатывает только новые файлы этого пакета.
func (s *Store) WriteControllerBatch(w http.ResponseWriter, kind string, items []ControllerResult, sourceWarnings []string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	response := BatchResponse{Kind: kind, Files: make([]GeneratedControllerFile, 0, len(items)), Warnings: []string{}}
	created := make([]string, 0, len(items))
	fail := func(err error) {
		for _, path := range created {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				s.logger.Printf("cannot roll back generated XML file %s: %v", path, removeErr)
				err = errors.Join(err, fmt.Errorf("не удалось удалить незавершённый результат %s: %w", filepath.Base(path), removeErr))
			}
		}
		transport.WriteError(w, http.StatusInternalServerError, err)
	}
	warnings := map[string]bool{}
	for _, warning := range sourceWarnings {
		if !warnings[warning] {
			response.Warnings = append(response.Warnings, warning)
			warnings[warning] = true
		}
	}
	for _, item := range items {
		controllerName, result := item.ControllerName, item.Result
		fileName, err := uniqueOutputName(s.outputDir, item.FileName)
		if err != nil {
			fail(err)
			return
		}
		path := filepath.Join(s.outputDir, fileName)
		if err := writeAtomic(path, result.XML); err != nil {
			fail(fmt.Errorf("записать XML для %s: %w", controllerName, err))
			return
		}
		created = append(created, path)
		response.Files = append(response.Files, GeneratedControllerFile{ControllerName: controllerName, GenerateResponse: GenerateResponse{
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
		s.logger.Printf("generated XML file %s for %s with %d POU and %d signals", file.FileName, file.ControllerName, file.Summary.POUCount, file.Summary.SignalCount)
	}
	transport.WriteJSON(w, http.StatusCreated, response)
}
