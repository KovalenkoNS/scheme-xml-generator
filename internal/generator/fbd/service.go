// Сервис FBD удерживает неизменяемые настройки запроса для предметных графических генераторов.
package fbd

import (
	"scheme-xml-generator/internal/config"
)

type Generator struct{ Config config.Config }
