// Чтение локального источника через ограниченный multipart.
package modulemapping

import (
	"net/http"
	ioupload "scheme-xml-generator/internal/httpapi/io/upload"
	"scheme-xml-generator/internal/iomap"
)

// ReadModuleMappingUpload читает XLSX-карту для совместимых IO-маршрутов preview/generate.
// Передаёт предел 16 МиБ и разрешённое поле config общему multipart-декодеру.
func ReadModuleMappingUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, string, error) {
	return ioupload.ReadMappingUpload(w, r, iomap.MaxWorkbookBytes, "XLSX", "16", "config")
}
