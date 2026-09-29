// Сервис ST предоставляет текстовые генераторы программ с конфигурацией текущего запроса.
package st

import (
	"scheme-xml-generator/internal/config"
)

type Generator struct{ Config config.Config }
