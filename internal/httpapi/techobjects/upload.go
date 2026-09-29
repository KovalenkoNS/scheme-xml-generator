// Чтение локального источника через ограниченный multipart.
package techobjectapi

import (
	"fmt"
	"net/http"
	"path/filepath"

	ioupload "scheme-xml-generator/internal/httpapi/io/upload"
	"scheme-xml-generator/internal/iomap"

	"strings"
)

// readTechObjectsUpload читает исходный IO для preview или генерации технологических объектов.
// Проверяет XLSX и разрешённые поля режима, возвращает байты и имя будущего результата.
func readTechObjectsUpload(w http.ResponseWriter, r *http.Request, generate bool) ([]byte, string, error) {
	data, name, _, err := ioupload.ReadMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", "objects")
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
