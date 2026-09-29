// Чтение локального источника через ограниченный multipart.
package ioao

import (
	"net/http"

	ioupload "scheme-xml-generator/internal/httpapi/io/upload"
)

// ReadAOMappingUpload читает TXT-загрузку AO для предметных обработчиков генерации.
// Передаёт формат, предел 2 МиБ и дополнительные поля общему multipart-декодеру IO.
func ReadAOMappingUpload(w http.ResponseWriter, r *http.Request, additionalFields ...string) ([]byte, string, string, error) {
	return ioupload.ReadMappingUpload(w, r, MaxAOMappingFileBytes, "TXT", "2", additionalFields...)
}
