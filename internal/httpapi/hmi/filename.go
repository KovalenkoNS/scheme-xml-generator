// Правила безопасного и различимого имени сохраняемого результата.
package hmi

import (
	"scheme-xml-generator/internal/httpapi/output"
	"strings"
)

// aoDiagnosticOutputName назначает имя XML-диагностики отдельного ПЛК.
// Очищает обе части через output.Store и добавляет суффикс _diagnostic_ без путей пользователя.
func aoDiagnosticOutputName(requestedName, controllerName string) string {
	base := strings.TrimSuffix(output.SafeOutputName(requestedName, "AO", "mapping"), ".xml")
	controller := strings.TrimSuffix(output.SafeOutputName(controllerName, "FCS", "mapping"), ".xml")
	return base + "_diagnostic_" + controller + ".xml"
}
