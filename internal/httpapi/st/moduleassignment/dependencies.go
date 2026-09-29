// Зависимости предметного HTTP-компонента, задаваемые сборщиком приложения.
package moduleassignment

import (
	"scheme-xml-generator/internal/generator"

	"scheme-xml-generator/internal/httpapi/output"
)

type Service struct {
	Allocator *generator.Allocator
	Output    *output.Store
	ST        MappedGenerator
}

// MappedGenerator передаёт проверенную IO-перекладку соответствующему предметному HTTP-компоненту.
// Преобразование XML принадлежит FBD/ST; загрузка карты и общий preflight остаются в iomapping.
type MappedGenerator interface {
	GenerateModuleAssignments(generator.ControllerPlan, generator.ProgramContext, generator.IDRange) (generator.Result, error)
}
