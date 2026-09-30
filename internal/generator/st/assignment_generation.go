// Выпуск ST физических назначений объединяет проверку плана и рендер одного документа.
package st

import (
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/generator/planning"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
)

// GenerateModuleMapping проверяет план ПЛК и контекст экспорта перед выпуском физических ST-присваиваний.
// Возвращает готовый XML и сводку; не меняет состояние allocator и не выбирает библиотечный FBD.
func (g Generator) GenerateModuleMapping(plan stassignment.ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange) (xmlartifact.Result, error) {
	ctx, _, err := planning.PrepareControllerGeneration(&plan, ctx, ids)
	if err != nil {
		return xmlartifact.Result{}, err
	}
	return GenerateModuleST(plan, ctx, ids)
}
