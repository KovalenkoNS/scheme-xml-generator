// Каталог сохранённых XML/XLS результатов без изменения файлов.
package output

import (
	"net/http"

	"os"
	"sort"

	"scheme-xml-generator/internal/httpapi/transport"
	"time"

	"net/url"
)

type outputFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
}

// HandleOutputs перечисляет сохранённые результаты для интерфейса генератора.
// Возвращает только XML/XLS из каталога вывода с размером/датой/URL, упорядоченные по времени.
func (s *Store) HandleOutputs(w http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(s.outputDir)
	if err != nil {
		transport.WriteError(w, http.StatusInternalServerError, err)
		return
	}
	files := make([]outputFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || !supportedOutputFile(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, outputFile{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().Format(time.RFC3339), URL: "/api/output/" + url.PathEscape(entry.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].CreatedAt > files[j].CreatedAt })
	transport.WriteJSON(w, http.StatusOK, map[string]any{"files": files})
}
