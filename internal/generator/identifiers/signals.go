// SCADA identifier rules validate the specific names and text supplied to generation.
package identifiers

import (
	"regexp"
)

// ControllerNamePattern проверяет имя ПЛК в планах ST/HMI и библиотечном DO-запросе.
// Ограничение идентификатора не зависит от заголовка исходной таблицы.
var ControllerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)

var ObjectNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,159}$`)

var NumberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
