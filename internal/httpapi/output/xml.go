// Сохранение одиночного XML и возвращаемый HTTP-контракт.
package output

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	"scheme-xml-generator/internal/httpapi/transport"
)

type GenerateResponse struct {
	FileName string              `json:"fileName"`
	URL      string              `json:"url"`
	BaseName string              `json:"baseName"`
	Summary  xmlartifact.Summary `json:"summary"`
	Warnings []string            `json:"warnings"`
}

// WriteGeneratedResult сохраняет одиночный XML-результат FBD и формирует ссылку интерфейсу.
// Под блокировкой выбирает свободное имя, атомарно пишет XML и возвращает summary/warnings.
func (s *Store) WriteGeneratedResult(w http.ResponseWriter, requestedName, fallbackName string, result xmlartifact.Result) {
	fileName := SafeOutputName(requestedName, result.BaseName, fallbackName)
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	fileName, err := uniqueOutputName(s.outputDir, fileName)
	if err != nil {
		transport.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if err := writeAtomic(filepath.Join(s.outputDir, fileName), result.XML); err != nil {
		transport.WriteError(w, http.StatusInternalServerError, fmt.Errorf("записать результат: %w", err))
		return
	}
	s.logger.Printf("generated %s with %d POU and %d signals", fileName, result.Summary.POUCount, result.Summary.SignalCount)
	transport.WriteJSON(w, http.StatusCreated, GenerateResponse{FileName: fileName, URL: "/api/output/" + url.PathEscape(fileName), BaseName: result.BaseName, Summary: result.Summary, Warnings: result.Warnings})
}
