// Разрешённые форматы файлов каталога результатов.
package output

import (
	"path/filepath"

	"strings"
)

// supportedOutputFile ограничивает расширения каталога выдачи результатов генератора.
// Возвращает true только для XML/XLS, не разрешая HTML или исполняемые расширения.
func supportedOutputFile(name string) bool {
	extension := filepath.Ext(name)
	return strings.EqualFold(extension, ".xml") || strings.EqualFold(extension, ".xls")
}
