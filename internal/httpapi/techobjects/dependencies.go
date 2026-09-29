// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package techobjectapi

import (
	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct{ Output *output.Store }
