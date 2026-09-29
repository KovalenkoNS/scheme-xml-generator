// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package catalog

import (
	"scheme-xml-generator/internal/library"
)

type Service struct{ Repository *library.Repository }
