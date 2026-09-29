// Request-local CPU context checks for library FBD and shared program-context validation.
package fbd

import (
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	cpuprofile "scheme-xml-generator/internal/generator/controller"
	"strings"
	"testing"
)

// TestGenerationContextIsRequestLocal checks CPU overrides create independent generator settings while omitted
// context preserves configuration and unknown CPUs fail.
func TestGenerationContextIsRequestLocal(t *testing.T) {
	settings := config.Default()
	settings.Common.ControllerType = cpuprofile.ControllerCPU715
	g := Generator{Config: settings}
	for _, cpu := range []string{cpuprofile.ControllerCPU850, cpuprofile.ControllerCPU715} {
		copy, err := g.withGenerationContext(&contracts.GenerationContext{ControllerTypeName: " " + cpu + " "})
		if err != nil || copy.Config.Common.ControllerType != cpu {
			t.Fatalf("cpu=%s, copy=%+v, err=%v", cpu, copy.Config.Common, err)
		}
		if g.Config != settings {
			t.Fatal("request changed persistent generator settings")
		}
	}
	copy, err := g.withGenerationContext(nil)
	if err != nil || copy.Config != settings {
		t.Fatal("omitted context must preserve config")
	}
	for _, cpu := range []string{"", "850", "CPU715", "TENIX-CPU999", "TENIX-CPU850\x00"} {
		if _, err := g.withGenerationContext(&contracts.GenerationContext{ControllerTypeName: cpu}); err == nil {
			t.Fatalf("accepted unsupported explicit CPU %q", cpu)
		}
	}
}

// TestLibraryFBDContextBothControllers renders single and multi-POU library FBD for CPU715/850 and checks selected
// CPU metadata without changing shared settings.
func TestLibraryFBDContextBothControllers(t *testing.T) {
	ref := loadAD3V2Template(t)
	settings := config.Default()
	settings.Common.ControllerType = cpuprofile.ControllerCPU715
	g := Generator{Config: settings}
	ids := contracts.IDRange{T11Start: 4500000, CardStart: 950000, POUID: 250000}
	for _, cpu := range []string{cpuprofile.ControllerCPU715, cpuprofile.ControllerCPU850} {
		t.Run(cpu, func(t *testing.T) {
			context := &contracts.GenerationContext{ControllerTypeName: cpu}
			legacy, err := g.Generate(ref, contracts.Request{Context: context, ObjectName: "_CPU_SIGNAL", NameMode: "base"}, ids)
			if err != nil {
				t.Fatal(err)
			}
			document, err := g.GenerateDocument(contracts.Request{Context: context}, []contracts.ResolvedPOU{{
				Request: contracts.POURequest{Name: "CPU_POU"},
				Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{ObjectName: "_CPU_SIGNAL", NameMode: "base"}}},
			}}, ids)
			if err != nil {
				t.Fatal(err)
			}
			for _, result := range []contracts.Result{legacy, document} {
				if parseGeneratedDocument(t, result.XML).Common.ControllerType != cpu {
					t.Fatalf("CPU %s lost from generated Common", cpu)
				}
			}
		})
	}
	if g.Config != settings {
		t.Fatal("Generate/GenerateDocument changed settings")
	}
}

// TestMappingContextRejectsUnknownCPUAndPhysicalProfile checks shared program-context normalization accepts
// supported CPUs and rejects unknown CPU/address-profile names.
func TestMappingContextRejectsUnknownCPUAndPhysicalProfile(t *testing.T) {
	for _, cpu := range []string{cpuprofile.ControllerCPU715, cpuprofile.ControllerCPU850} {
		ctx := addressing.DefaultAOContext()
		ctx.ControllerTypeName = " " + cpu + " "
		got, err := addressing.NormalizeProgramContext(ctx, 1)
		if err != nil || got.ControllerTypeName != cpu {
			t.Fatalf("CPU=%s context=%+v error=%v", cpu, got, err)
		}
	}
	ctx := addressing.DefaultAOContext()
	ctx.ControllerTypeName = "TENIX-CPU999"
	if _, err := addressing.NormalizeProgramContext(ctx, 1); err == nil {
		t.Fatal("accepted unknown CPU")
	}
	ctx = addressing.DefaultAOContext()
	ctx.PhysicalProfile = "guessed-driver"
	if _, err := addressing.NormalizeProgramContext(ctx, 1); err == nil || !strings.Contains(err.Error(), "профиль") {
		t.Fatalf("accepted unknown physical profile: %v", err)
	}
}
