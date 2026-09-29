// Границы лексики тегов и имён ПЛК в поддержанных адаптерах таблиц.
package fields

import (
	"regexp"
)

const IdentifierText = `[A-Za-z_][A-Za-z0-9_]{0,159}`
const PhysicalModuleText = `A[0-9]{1,6}[-_][0-9]{1,4}`

var ControllerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)
