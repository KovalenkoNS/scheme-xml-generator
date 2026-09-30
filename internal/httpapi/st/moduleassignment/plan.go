// Чистое планирование ST-перекладки с проверкой физических каналов и общего объёма.
package moduleassignment

import (
	"fmt"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/generator/planning"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	ioctx "scheme-xml-generator/internal/httpapi/io/context"
	"scheme-xml-generator/internal/httpapi/io/modulemapping"
	"scheme-xml-generator/internal/httpapi/limits"
	"scheme-xml-generator/internal/inputs/assignments"
)

type assignmentPlan struct {
	Input        assignmentInput
	Plans        []stassignment.ControllerPlan
	Context      programcontext.ProgramContext
	Requirements []xmlidentity.DocumentRequirements
	Total        xmlidentity.DocumentRequirements
	Warnings     []string
}

// prepareAssignmentPlan разбирает IO-карту и подготавливает выбранные ПЛК до изменения ID.
// Возвращает планы, контекст, суммарные требования и предупреждения; файлов и allocator не касается.
func prepareAssignmentPlan(input assignmentInput) (assignmentPlan, error) {
	data, request, rawContext := input.Data, input.Request, input.RawContext
	source, err := assignments.Parse(data)
	if err == nil {
		err = modulemapping.CheckModuleMappingSize(source)
	}
	if err != nil {
		return assignmentPlan{}, err
	}
	plans, err := planning.PrepareModulePlans(source, request)
	if err != nil {
		return assignmentPlan{}, err
	}
	contextMode := request.Kind
	ctx, err := ioctx.DecodeMappingContextWithDefault(rawContext, modulemapping.ContextForMode(plans[0].POUs[0].Kind, contextMode))
	if err != nil {
		return assignmentPlan{}, err
	}
	requirements := make([]xmlidentity.DocumentRequirements, len(plans))
	var total xmlidentity.DocumentRequirements
	for index, plan := range plans {
		requirements[index], err = planning.RequirementsForController(plan)
		if err != nil {
			return assignmentPlan{}, err
		}
		total.T11Count += requirements[index].T11Count
		total.CardCount += requirements[index].CardCount
		total.POUCount += requirements[index].POUCount
		total.SignalCount += requirements[index].SignalCount
	}
	if total.SignalCount > limits.MaxDocumentSignals {
		return assignmentPlan{}, fmt.Errorf("назначения модулей с дополнительными модулями превышает предел %d каналов", limits.MaxDocumentSignals)
	}
	if total.POUCount > limits.MaxDocumentPOUs {
		return assignmentPlan{}, fmt.Errorf("назначения модулей превышает предел %d POU ", limits.MaxDocumentPOUs)
	}
	if total.T11Count > limits.MaxDocumentObjects || total.CardCount > limits.MaxDocumentCards {
		return assignmentPlan{}, fmt.Errorf("назначения модулей превышает допустимое число объектов или POU")
	}

	return assignmentPlan{Input: input, Plans: plans, Context: ctx, Requirements: requirements, Total: total, Warnings: source.Warnings}, nil
}
