// Подготовка модулей проверяет план, назначение и диапазоны перед ST-экспортом.
package planning

import (
	"fmt"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	cpuprofile "scheme-xml-generator/internal/generator/controller"
)

// PrepareControllerGeneration validates a selected plan and its destination before dispatch.
// ST callers receive a normalized context and exact ID requirements.
func PrepareControllerGeneration(plan *contracts.ControllerPlan, ctx contracts.ProgramContext, ids contracts.IDRange) (contracts.ProgramContext, contracts.DocumentRequirements, error) {
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
	if ids.POUID < 1 || ids.POUID > contracts.MaxTransportID-int64(req.POUCount)+1 {
		return ctx, req, fmt.Errorf("Модули: диапазон POU ID выходит за signed 32-bit")
	}
	return ctx, req, nil
}
