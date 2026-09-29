// Безопасное подключение загруженной XML-библиотеки.
package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"scheme-xml-generator/internal/httpapi/transport"
	"scheme-xml-generator/internal/library"
	"strings"
)

// HandleLibraryImport подключает загруженную XML-библиотеку из компактного диалога интерфейса.
// Проверяет multipart и размер, передаёт импорт Repository и возвращает обновлённый каталог без перезаписи библиотек.
func (s *Service) HandleLibraryImport(w http.ResponseWriter, r *http.Request) {
	const multipartExtra int64 = 128 << 10
	if r.ContentLength > library.MaxLibraryBytes+multipartExtra {
		transport.WriteError(w, http.StatusRequestEntityTooLarge, library.ErrLibraryTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, library.MaxLibraryBytes+multipartExtra)
	defer r.Body.Close()
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		status := http.StatusBadRequest
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			status = http.StatusRequestEntityTooLarge
		}
		transport.WriteError(w, status, fmt.Errorf("не удалось прочитать XML-файл библиотеки: %w", err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("выберите один XML-файл библиотеки"))
		return
	}
	header := r.MultipartForm.File["file"][0]
	if !strings.EqualFold(filepath.Ext(header.Filename), ".xml") {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("библиотека должна быть файлом XML"))
		return
	}
	if header.Size > library.MaxLibraryBytes {
		transport.WriteError(w, http.StatusRequestEntityTooLarge, library.ErrLibraryTooLarge)
		return
	}
	file, err := header.Open()
	if err != nil {
		transport.WriteError(w, http.StatusInternalServerError, fmt.Errorf("не удалось открыть загруженную библиотеку"))
		return
	}
	defer file.Close()
	result, err := s.Repository.Import(file)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, library.ErrInvalidLibrary) {
			status = http.StatusBadRequest
		} else if errors.Is(err, library.ErrLibraryTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		transport.WriteError(w, status, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	transport.WriteJSON(w, status, result)
}
