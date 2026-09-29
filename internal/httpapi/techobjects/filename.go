// Правила безопасного и различимого имени сохраняемого результата.
package techobjectapi

import (
	"scheme-xml-generator/internal/httpapi/output"
	"strings"
)

// techObjectsOutputName формирует имя XLS технологических объектов с идентификатором ПЛК.
// Очищает пользовательскую часть общим правилом output и заменяет расширение на .xls.
func techObjectsOutputName(requested, controllerName string) string {
	if strings.TrimSpace(requested) == "" {
		requested = "TechObjects"
	}
	base := strings.TrimSuffix(output.SafeOutputName(requested, "TechObjects", ""), ".xml")
	controller := strings.TrimSuffix(output.SafeOutputName(controllerName, "PLC", ""), ".xml")
	return base + "_" + controller + ".xls"
}
