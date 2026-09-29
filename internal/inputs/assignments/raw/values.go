// Толкование пустых значений и резервов только в исходной IO-карте.
package raw

import (
	"strings"
)

// empty распознаёт отсутствие значения по контракту исходного IO-листа.
func empty(value string) bool { return value == "" || value == "-" }

// reserve распознаёт явную отметку резерва, не создавая имя сигнала.
func reserve(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "spare", "reserve", "резерв":
		return true
	}
	return false
}
