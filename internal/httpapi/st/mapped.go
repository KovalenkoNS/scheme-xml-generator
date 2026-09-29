// Совместимый IO-маршрут передаёт сюда ST-планы после общей проверки выбранных модулей.
package st

import "scheme-xml-generator/internal/generator"

// GenerateModuleAssignments вызывается IO-диспетчером для ST-части совместимого SKZ API.
// Передаёт проверенный план, контекст и зарезервированные POU ID генератору и возвращает XML с STCODE.
func (s *Service) GenerateModuleAssignments(plan generator.ControllerPlan, ctx generator.ProgramContext, ids generator.IDRange) (generator.Result, error) {
	return s.Generator.GenerateModuleMapping(plan, ctx, ids)
}
