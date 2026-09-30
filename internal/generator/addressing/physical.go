// Import addressing validates transport values and confirmed physical driver profiles.
package addressing

import (
	"fmt"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	programcontext "scheme-xml-generator/internal/generator/program"
	"strings"
)

const PhysicalProfileLegacy = "legacy-iu-qu"

const PhysicalProfileMeasurement = "measurement-quality"

// CPU850 exports contain both legacy IU/QU addresses and Measurement/Quality
// addresses. Keep the physical profile separate from the controller identity.
// The software-only FBD graph does not consume this profile, but unknown names
// are still rejected so a typo cannot silently change a later ST generation.
func EffectiveModuleProfile(ctx programcontext.ProgramContext, mode string) (string, error) {
	profile := strings.TrimSpace(ctx.PhysicalProfile)
	if profile == "" {
		profile = PhysicalProfileLegacy
		if ctx.ControllerTypeName == cpuprofile.ControllerCPU850 {
			profile = PhysicalProfileMeasurement
		}
	}
	switch profile {
	case PhysicalProfileLegacy:
		return profile, nil
	case PhysicalProfileMeasurement:
		if mode == "st" && ctx.ControllerTypeName != cpuprofile.ControllerCPU850 {
			return "", fmt.Errorf("Модули: физический профиль %s для ST подтверждён только для %s; для %s используйте %s", profile, cpuprofile.ControllerCPU850, ctx.ControllerTypeName, PhysicalProfileLegacy)
		}
		return profile, nil
	default:
		return "", fmt.Errorf("Модули: неизвестный физический профиль %q; ожидается %s или %s", profile, PhysicalProfileLegacy, PhysicalProfileMeasurement)
	}
}
