// Import addressing validates transport values and confirmed physical driver profiles.
package addressing

import (
	"scheme-xml-generator/internal/generator/contracts"
)

// DefaultAOContext Возвращает исходные константы транспортного контекста AO/ST.
// Используется обработчиками как база перед применением параметров пользователя.
func DefaultAOContext() contracts.ProgramContext {
	return contracts.ProgramContext{
		Version: "29", Project: `otpscadafb3:e:\TAProject\ОАОН\СИБУР\Sinopec\SCADABD.GDB`,
		ControllerTypeName: "TENIX-CPU715", ControllerID: "189300", ResourceID: "637",
		GroupID: "19498", POUNumber: "36",
	}
}
