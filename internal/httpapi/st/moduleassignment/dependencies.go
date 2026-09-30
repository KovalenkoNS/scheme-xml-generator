// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package moduleassignment

import (
	"scheme-xml-generator/internal/generator/allocation"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Allocator *allocation.Allocator
	Output    *output.Store
	ST        MappedGenerator
}

// MappedGenerator передаёт проверенную IO-перекладку соответствующему предметному HTTP-компоненту.
// Преобразование XML принадлежит FBD/ST; загрузка карты и общий preflight остаются в iomapping.
type MappedGenerator interface {
	GenerateModuleAssignments(stassignment.ControllerPlan, programcontext.ProgramContext, xmlidentity.IDRange) (xmlartifact.Result, error)
}
