// Чтение локального источника через ограниченный multipart.
package iosource

import (
	"fmt"
	"net/http"
	"path/filepath"
	ioupload "scheme-xml-generator/internal/httpapi/io/upload"
	"scheme-xml-generator/internal/iomap"
	"strings"
)

// ReadIOUpload читает исходный IO XLSX для предварительного просмотра и диагностики.
// Использует общий ограниченный multipart-декодер, дополнительно проверяет расширение исходного файла.
func ReadIOUpload(w http.ResponseWriter, r *http.Request, fields ...string) ([]byte, string, string, error) {
	data, name, ctx, err := ioupload.ReadMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", fields...)
	if err == nil && !strings.EqualFold(filepath.Ext(r.MultipartForm.File["file"][0].Filename), ".xlsx") {
		err = fmt.Errorf("для диагностики ПЛК нужен Excel IO в формате .xlsx, а не TXT или перекладка")
	}
	return data, name, ctx, err
}
