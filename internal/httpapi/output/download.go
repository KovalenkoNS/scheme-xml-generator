// Выдача байтов фактически сохранённого результата по безопасному имени.
package output

import (
	"fmt"
	"path/filepath"

	"io"
	"net/http"

	"mime"
	"os"

	"strings"

	"scheme-xml-generator/internal/httpapi/transport"

	"net/url"
)

// HandleDownload выдаёт фактический сохранённый файл предпросмотру или скачиванию.
// Проверяет одно безопасное имя XML/XLS, открывает каталог вывода и копирует байты с нужным content-type.
func (s *Store) HandleDownload(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil || filepath.Base(name) != name || !supportedOutputFile(name) {
		transport.WriteError(w, http.StatusBadRequest, fmt.Errorf("недопустимое имя файла"))
		return
	}
	path := filepath.Join(s.outputDir, name)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		transport.WriteError(w, http.StatusNotFound, fmt.Errorf("файл не найден"))
		return
	}
	if err != nil {
		transport.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()
	if strings.EqualFold(filepath.Ext(name), ".xls") {
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
	} else {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	_, _ = io.Copy(w, file)
}
