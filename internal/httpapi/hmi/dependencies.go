// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package hmi

import (
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Generator *generator.Generator
	Allocator *generator.Allocator
	Output    *output.Store
}
