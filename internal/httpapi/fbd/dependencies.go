// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package fbd

import (
	"scheme-xml-generator/internal/generator/allocation"
	fbdgen "scheme-xml-generator/internal/generator/fbd"
	"scheme-xml-generator/internal/httpapi/output"
	"scheme-xml-generator/internal/library"
)

type Service struct {
	Repository *library.Repository
	Generator  *fbdgen.Generator
	Allocator  *allocation.Allocator
	Output     *output.Store
}
