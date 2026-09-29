// Синтаксис имён ПЛК/модулей в таблицах технологических объектов.
package techobjects

import (
	"regexp"
)

var plcName = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)

var moduleName = regexp.MustCompile(`^(A[0-9]{1,6})_([0-9]{2})$`)
