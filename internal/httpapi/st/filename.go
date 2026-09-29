// Правила безопасного и различимого имени сохраняемого результата.
package st

import (
	"scheme-xml-generator/internal/httpapi/output"
	"strings"
)

// aoSTOutputName назначает имя ST-результата отдельного ПЛК.
// Очищает пользовательское имя и имя ПЛК, добавляет различимый суффикс _ST_ перед расширением XML.
func aoSTOutputName(requestedName, controllerName string) string {
	base := strings.TrimSuffix(output.SafeOutputName(requestedName, "AO", "mapping"), ".xml")
	controller := strings.TrimSuffix(output.SafeOutputName(controllerName, "FCS", "mapping"), ".xml")
	return base + "_ST_" + controller + ".xml"
}
