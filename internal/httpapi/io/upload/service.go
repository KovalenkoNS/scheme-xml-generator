// Ограниченное чтение multipart для локальных IO-карт.
package ioupload

import (
	"fmt"
	"io"
	"net/http"
)

const MaxAOMultipartExtra int64 = 128 << 10

// ReadMappingUpload проверяет загруженную локальную карту на HTTP-границе IO.
// Возвращает байты, имя результата и контекст; ограничивает тело/поля, удаляет временные multipart-файлы.
func ReadMappingUpload(w http.ResponseWriter, r *http.Request, limit int64, format, sizeLabel string, additionalFields ...string) ([]byte, string, string, error) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, limit+MaxAOMultipartExtra)
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(limit); err != nil {
		return nil, "", "", fmt.Errorf("не удалось прочитать загрузку %s (не более %s МиБ): %w", format, sizeLabel, err)
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		return nil, "", "", fmt.Errorf("необходимо загрузить ровно один %s в поле file", format)
	}
	allowed := map[string]bool{"fileName": true, "context": true}
	for _, name := range additionalFields {
		allowed[name] = true
	}
	for name, values := range r.MultipartForm.Value {
		if !allowed[name] || len(values) != 1 {
			return nil, "", "", fmt.Errorf("неподдерживаемое или повторное поле загрузки %q", name)
		}
		if len(values[0]) > 64<<10 {
			return nil, "", "", fmt.Errorf("поле %s превышает 64 КиБ", name)
		}
	}
	value := func(name string) string {
		if values := r.MultipartForm.Value[name]; len(values) == 1 {
			return values[0]
		}
		return ""
	}
	fileName := value("fileName")
	rawContext := value("context")
	if len(fileName) > 512 || len(rawContext) > 8<<10 {
		return nil, "", "", fmt.Errorf("слишком длинное имя файла или контекст проекта")
	}
	header := r.MultipartForm.File["file"][0]
	if header.Size > limit {
		return nil, "", "", fmt.Errorf("%s превышает %s МиБ", format, sizeLabel)
	}
	file, err := header.Open()
	if err != nil {
		return nil, "", "", fmt.Errorf("не удалось открыть загруженный %s: %w", format, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("не удалось прочитать загруженный %s: %w", format, err)
	}
	if int64(len(data)) > limit {
		return nil, "", "", fmt.Errorf("%s превышает %s МиБ", format, sizeLabel)
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("загруженный %s пуст", format)
	}
	return data, fileName, rawContext, nil
}
