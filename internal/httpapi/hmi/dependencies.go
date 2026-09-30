// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package hmi

import (
	"scheme-xml-generator/internal/generator/allocation"
	hmigen "scheme-xml-generator/internal/generator/hmi"
	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Generator *hmigen.Generator
	Allocator *allocation.Allocator
	Output    *output.Store
}
