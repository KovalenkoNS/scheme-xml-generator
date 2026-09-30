// Подготовка модулей проверяет план, назначение и диапазоны перед ST-экспортом.
package planning

import (
	"fmt"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
)

// PrepareControllerGeneration validates a selected plan and its destination before dispatch.
// ST callers receive a normalized context and exact ID requirements.
func PrepareControllerGeneration(plan *stassignment.ControllerPlan, ctx programcontext.ProgramContext, ids xmlidentity.IDRange) (programcontext.ProgramContext, xmlidentity.DocumentRequirements, error) {
	req, err := ValidateControllerPlan(plan)
	if err != nil {
		return ctx, req, err
	}
	ctx, err = addressing.NormalizeProgramContext(ctx, req.POUCount)
	if err != nil {
		return ctx, req, err
	}
	ctx.PhysicalProfile, err = addressing.EffectiveModuleProfile(ctx, plan.Kind)
	if err != nil {
		return ctx, req, err
	}
	if plan.Kind == "st" {
		for _, pou := range plan.POUs {
			if pou.Kind == "DI" && (ctx.ControllerTypeName != cpuprofile.ControllerCPU850 || ctx.PhysicalProfile != addressing.PhysicalProfileMeasurement) {
				return ctx, req, fmt.Errorf("DI ST: подтверждён только %s с профилем %s; legacy DI и его статус требуют нативного образца", cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement)
			}
		}
	}
	if ids.POUID < 1 || ids.POUID > xmlidentity.MaxTransportID-int64(req.POUCount)+1 {
		return ctx, req, fmt.Errorf("Модули: диапазон POU ID выходит за signed 32-bit")
	}
	return ctx, req, nil
}
