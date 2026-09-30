// Совместимый IO-маршрут передаёт сюда ST-планы после общей проверки выбранных модулей.
package st

import (
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
)

// GenerateModuleAssignments вызывается маршрутом назначений модулей для ST-части выбранного ПЛК.
// Передаёт проверенный план, контекст и зарезервированные POU ID генератору и возвращает XML с STCODE.
func (s *Service) GenerateModuleAssignments(plan stassignment.ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange) (xmlartifact.Result, error) {
	return s.Generator.GenerateModuleMapping(plan, ctx, ids)
}
