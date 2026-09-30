// Shared export-context regressions prevent private AO settings from leaking into other workflows.
package contracts_test

import (
	"scheme-xml-generator/internal/generator/addressing"
	hmigen "scheme-xml-generator/internal/generator/hmi"
	"testing"
)

// TestModuleAndHMIContextsDoNotInheritAOProject checks generic exports start without a private AO project path.
// Mutating an AO request also cannot alter later module-ST or HMI defaults.
func TestModuleAndHMIContextsDoNotInheritAOProject(t *testing.T) {
	module := addressing.DefaultModuleContext()
	hmi := hmigen.DefaultHMIContext()
	ao := addressing.DefaultAOContext()
	ao.Project = "request-specific AO project"
	if module.Project != "" || hmi.Project != "" {
		t.Fatalf("private project leaked into generic context: module=%q hmi=%q", module.Project, hmi.Project)
	}
	if addressing.DefaultModuleContext() != module || hmigen.DefaultHMIContext() != hmi {
		t.Fatal("AO request changed generic context defaults")
	}
	if ao.Project == addressing.DefaultAOContext().Project {
		t.Fatal("AO defaults alias request state")
	}
}
