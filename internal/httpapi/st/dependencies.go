// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package st

import (
	"scheme-xml-generator/internal/generator/allocation"
	stgen "scheme-xml-generator/internal/generator/st"
	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Generator *stgen.Generator
	Allocator *allocation.Allocator
	Output    *output.Store
}
