// Резервирование ID и выполнение заранее проверенного пакета ST по ПЛК.
package moduleassignment

import (
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/httpapi/output"
	"strings"
)

// generateAssignments передаёт каждый готовый план ST-компоненту в одной reservation.
// Возвращает полный набор XML-результатов либо ошибку; запись файлов выполняет отдельный output.Store.
func (s *Service) generateAssignments(plan assignmentPlan) ([]output.ControllerResult, error) {
	plans, ctx, requirements, total := plan.Plans, plan.Context, plan.Requirements, plan.Total
	fileName, request := plan.Input.FileName, plan.Input.Request
	items := make([]output.ControllerResult, len(plans))
	_, err := s.Allocator.WithReservation(total.T11Count, total.CardCount, total.POUCount, generator.ReservationOptions{}, func(ids generator.IDRange) error {
		for index, plan := range plans {
			result, generateErr := s.ST.GenerateModuleAssignments(plan, ctx, ids)
			if generateErr != nil {
				return generateErr
			}
			base := strings.TrimSuffix(output.SafeOutputName(fileName, "MODULES", "mapping"), ".xml")
			controller := strings.TrimSuffix(output.SafeOutputName(plan.ControllerName, "PLC", "mapping"), ".xml")
			modeSuffix := strings.ToUpper(request.Kind)
			items[index] = output.ControllerResult{ControllerName: plan.ControllerName, FileName: base + "_" + modeSuffix + "_" + controller + ".xml", Result: result,
				AssignmentCount: plan.AssignmentCount, RepeatedAssignmentCount: plan.RepeatedAssignmentCount}
			ids.T11Start += int64(requirements[index].T11Count)
			ids.CardStart += int64(requirements[index].CardCount)
			ids.POUID += int64(requirements[index].POUCount)
		}
		return nil
	})
	return items, err
}
