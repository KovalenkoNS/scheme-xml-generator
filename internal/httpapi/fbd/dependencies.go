// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package fbd

import (
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/library"

	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Repository *library.Repository
	Generator  *generator.Generator
	Allocator  *generator.Allocator
	Output     *output.Store
}
