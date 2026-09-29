// Defaults for physical module ST export are independent of AO-map project settings.
package addressing

import (
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/controller"
)

// DefaultModuleContext supplies the confirmed module-ST import context before user overrides.
// IDs originate from the reference export; no AO project or RSU/PAZ/SOGO membership is inferred.
func DefaultModuleContext() contracts.ProgramContext {
	return contracts.ProgramContext{
		Version: "29", ControllerTypeName: controller.ControllerCPU850,
		ControllerID: "189311", ResourceID: "647", GroupID: "19912", POUNumber: "4",
	}
}
