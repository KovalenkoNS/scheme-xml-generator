// Сервис HMI предоставляет предметные генераторы диагностических кадров и панелей.
package hmi

import (
	"scheme-xml-generator/internal/config"
)

type Generator struct{ Config config.Config }
