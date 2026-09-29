// Лексическая проверка преобразованного Tag No исходного IO-листа.
package raw

import (
	"regexp"
	"scheme-xml-generator/internal/inputs/assignments/fields"
)

var rawIOTagPattern = regexp.MustCompile(`^` + fields.IdentifierText + `$`)
