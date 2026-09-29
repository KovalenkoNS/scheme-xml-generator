// Пакетное сохранение XLS технологических объектов и откат новых файлов при ошибке.
package output

import (
	"errors"
	"fmt"
	"path/filepath"

	"net/http"

	"os"

	"scheme-xml-generator/internal/techobjects"

	"scheme-xml-generator/internal/httpapi/transport"

	"net/url"
)

type TechObjectsGeneratedFile struct {
	ControllerName string              `json:"fcs"` // Совместимый JSON alias имени ПЛК для XLS-результата.
	FileName       string              `json:"fileName"`
	URL            string              `json:"url"`
	Summary        techobjects.Summary `json:"summary"`
}

type TechObjectsResponse struct {
	Files    []TechObjectsGeneratedFile `json:"files"`
	Summary  techobjects.Summary        `json:"summary"`
	Warnings []string                   `json:"warnings"`
}

type TechObjectsBatchItem struct {
	ControllerName, FileName string
	Data                     []byte
	Summary                  techobjects.Summary
}

// WriteTechObjectsBatch сохраняет пакет XLS технологических объектов без изменения XML allocator.
// Назначает свободные имена, откатывает только новые файлы при сбое и возвращает сводку по ПЛК.
func (s *Store) WriteTechObjectsBatch(w http.ResponseWriter, items []TechObjectsBatchItem, warnings []string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	response := TechObjectsResponse{Files: make([]TechObjectsGeneratedFile, 0, len(items)), Warnings: append([]string{}, warnings...)}
	created := make([]string, 0, len(items))
	fail := func(err error) {
		for _, path := range created {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				s.logger.Printf("cannot roll back generated technological objects file %s: %v", path, removeErr)
				err = errors.Join(err, fmt.Errorf("не удалось удалить незавершённый результат %s: %w", filepath.Base(path), removeErr))
			}
		}
		transport.WriteError(w, http.StatusInternalServerError, err)
	}
	for _, item := range items {
		name, err := uniqueOutputName(s.outputDir, item.FileName)
		if err != nil {
			fail(err)
			return
		}
		path := filepath.Join(s.outputDir, name)
		if err := writeAtomic(path, item.Data); err != nil {
			fail(fmt.Errorf("записать XLS для %s: %w", item.ControllerName, err))
			return
		}
		created = append(created, path)
		response.Files = append(response.Files, TechObjectsGeneratedFile{ControllerName: item.ControllerName, FileName: name, URL: "/api/output/" + url.PathEscape(name), Summary: item.Summary})
		response.Summary.ModuleCount += item.Summary.ModuleCount
		response.Summary.ReserveCount += item.Summary.ReserveCount
		response.Summary.ObjectCount += item.Summary.ObjectCount
	}
	for _, file := range response.Files {
		s.logger.Printf("generated technological objects file %s for %s with %d modules and %d reserves", file.FileName, file.ControllerName, file.Summary.ModuleCount, file.Summary.ReserveCount)
	}
	transport.WriteJSON(w, http.StatusCreated, response)
}
