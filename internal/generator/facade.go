// Package generator owns the compatibility API that dispatches to subject generators.
package generator

import (
	_ "embed"
	"scheme-xml-generator/internal/domain/assignments"
	iomap "scheme-xml-generator/internal/domain/inventory"
	"scheme-xml-generator/internal/generator/addressing"
	cpuprofile "scheme-xml-generator/internal/generator/controller"

	"scheme-xml-generator/internal/config"
	aomap "scheme-xml-generator/internal/domain/analogoutput"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/fbd"
	"scheme-xml-generator/internal/generator/hmi"
	"scheme-xml-generator/internal/generator/planning"
	"scheme-xml-generator/internal/generator/st"
	"scheme-xml-generator/internal/library"
)

type Generator struct{ Config config.Config }

type Allocator = allocation.Allocator

type ReservationOptions = allocation.ReservationOptions

// NewAllocator Restores the persistent ID cursors used by HTTP generation.
// The allocation component receives the state path and default ranges; load errors are returned.
func NewAllocator(path string, defaults config.IDDefaults) (*allocation.Allocator, error) {
	return allocation.NewAllocator(path, defaults)
}

type AllocatorPersistenceError = allocation.AllocatorPersistenceError

type HMIContext = hmi.HMIContext

// DefaultHMIContext Returns neutral HMI import values shared by AO and PLC diagnostics.
// HMI defaults are independent from the software FBD request context.
func DefaultHMIContext() hmi.HMIContext { return hmi.DefaultHMIContext() }

type AODiagnosticPlan = hmi.AODiagnosticPlan

type AODiagnosticFrame = hmi.AODiagnosticFrame

// PrepareAODiagnosticPlans Validates selected AO controllers before diagnostic page IDs are reserved.
// The HMI planner returns independent controller plans or a source/context error.
func PrepareAODiagnosticPlans(source *aomap.Plan, selectedControllers []string, ctx hmi.HMIContext) ([]hmi.AODiagnosticPlan, error) {
	return hmi.PrepareAODiagnosticPlans(source, selectedControllers, ctx)
}

type ProgramContext = contracts.ProgramContext

// DefaultAOContext Provides the native AO import destination used by temporary table workflows.
// The profile component supplies transport metadata without reading user configuration.
func DefaultAOContext() contracts.ProgramContext {
	return addressing.DefaultAOContext()
}

type AOSTRequest = st.AOSTRequest

type AOSTPOURequest = st.AOSTPOURequest

type AOSTPlan = st.AOSTPlan

type AOSTPOU = st.AOSTPOU

type AOSTModule = st.AOSTModule

// PrepareAOSTPlans Validates AO module IDs and selected table groups before ST generation.
// The ST planner preserves repeated assignments and returns one snapshot per controller.
func PrepareAOSTPlans(source *aomap.Plan, request st.AOSTRequest) ([]st.AOSTPlan, error) {
	return st.PrepareAOSTPlans(source, request)
}

const ControllerCPU715 = cpuprofile.ControllerCPU715

const ControllerCPU850 = cpuprofile.ControllerCPU850

const PhysicalProfileLegacy = addressing.PhysicalProfileLegacy

const PhysicalProfileMeasurement = addressing.PhysicalProfileMeasurement

type GenerationContext = contracts.GenerationContext

// RequirementsForDocument Calculates a single ID reservation for resolved library POUs.
// The FBD component checks the exact template snapshots and includes physical bindings.
func RequirementsForDocument(pous []contracts.ResolvedPOU) (contracts.DocumentRequirements, error) {
	return fbd.RequirementsForDocument(pous)
}

// Requirements Counts graphical primitives and card owners in one library template.
// The FBD component returns the T11/card range sizes needed before generation.
func Requirements(ref *library.TemplateRef) (t11Count, cardCount int) { return fbd.Requirements(ref) }

// PreviewName Previews instance names before the user commits a library template request.
// FBD naming returns the normalized base, matched prefix and generated object names.
func PreviewName(ref *library.TemplateRef, objectName, mode string) (contracts.NamePreview, error) {
	return fbd.PreviewName(ref, objectName, mode)
}

// NormalizePOURequests Normalizes library signals and rejects retired physical IO requests before template lookup.
// FBD validation supplies effective templates and checks module topology without reserving IDs.
func NormalizePOURequests(requests []contracts.POURequest) ([]contracts.POURequest, error) {
	return fbd.NormalizePOURequests(requests)
}

// POUSignals Flattens one POU request for library-template resolution by HTTP handlers.
// The FBD contract selects either module signals or the legacy flat signal list.
func POUSignals(pou contracts.POURequest) []contracts.SignalRequest { return fbd.POUSignals(pou) }

type LibraryDORequest = fbd.LibraryDORequest

type LibraryDOPOURequest = fbd.LibraryDOPOURequest

type LibraryDOModuleRequest = fbd.LibraryDOModuleRequest

type LibraryDOChannelRequest = fbd.LibraryDOChannelRequest

type LibraryDOPlan = fbd.LibraryDOPlan

type PLCDiagnosticPlan = hmi.PLCDiagnosticPlan

// PreparePLCDiagnosticPlans Validates selected IO inventories before HMI page allocation.
// The HMI planner returns independent PLC diagnostic plans and rejects unsupported inventories.
func PreparePLCDiagnosticPlans(source *iomap.Plan, selected []iomap.Selection, ctx hmi.HMIContext) ([]hmi.PLCDiagnosticPlan, error) {
	return hmi.PreparePLCDiagnosticPlans(source, selected, ctx)
}

type ModuleMappingRequest = contracts.ModuleMappingRequest

type ModuleGroupRequest = contracts.ModuleGroupRequest

type ControllerPlan = contracts.ControllerPlan

type ModuleGroup = contracts.ModuleGroup

type PhysicalModule = contracts.PhysicalModule

// DefaultModuleContext Returns the confirmed module native-export destination.
// Planning defaults describe import metadata and do not infer physical module IDs.
func DefaultModuleContext() contracts.ProgramContext { return addressing.DefaultModuleContext() }

// PrepareModulePlans Validates selected module groups, reserve modules and explicit hardware IDs.
// Planning returns independent per-controller snapshots for later FBD or ST generation.
func PrepareModulePlans(source *assignments.Plan, request contracts.ModuleMappingRequest) ([]contracts.ControllerPlan, error) {
	return planning.PrepareModulePlans(source, request)
}

// RequirementsForController Counts transport IDs for a validated controller plan.
// Planning checks topology and duplicate roles before the allocator changes its cursors.
func RequirementsForController(plan contracts.ControllerPlan) (contracts.DocumentRequirements, error) {
	return planning.RequirementsForController(plan)
}

type Request = contracts.Request

type POURequest = contracts.POURequest

type IORequest = contracts.IORequest

type IOModuleRequest = contracts.IOModuleRequest

type PageRequest = contracts.PageRequest

type SignalRequest = contracts.SignalRequest

type ResolvedSignal = contracts.ResolvedSignal

type ResolvedPOU = contracts.ResolvedPOU

type DocumentRequirements = contracts.DocumentRequirements

type IDRange = contracts.IDRange

type DiagnosticIDRange = contracts.DiagnosticIDRange

type DiagnosticFrameSummary = contracts.DiagnosticFrameSummary

type NamePreview = contracts.NamePreview

type Summary = contracts.Summary

type SignalSummary = contracts.SignalSummary

type IOModuleSummary = contracts.IOModuleSummary

type POUSummary = contracts.POUSummary

type Result = contracts.Result

// GenerateAODiagnostic delegates the public API to the hmi generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) GenerateAODiagnostic(plan hmi.AODiagnosticPlan, ctx hmi.HMIContext, ids contracts.DiagnosticIDRange) (contracts.Result, error) {
	return (hmi.Generator{Config: g.Config}).GenerateAODiagnostic(plan, ctx, ids)
}

// GenerateAOST delegates the public API to the st generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) GenerateAOST(plan st.AOSTPlan, ctx contracts.ProgramContext, ids contracts.IDRange) (contracts.Result, error) {
	return (st.Generator{Config: g.Config}).GenerateAOST(plan, ctx, ids)
}

// GenerateDocument delegates the public API to the fbd generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) GenerateDocument(request contracts.Request, pous []contracts.ResolvedPOU, ids contracts.IDRange) (contracts.Result, error) {
	return (fbd.Generator{Config: g.Config}).GenerateDocument(request, pous, ids)
}

// Generate delegates the public API to the fbd generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) Generate(ref *library.TemplateRef, request contracts.Request, ids contracts.IDRange) (contracts.Result, error) {
	return (fbd.Generator{Config: g.Config}).Generate(ref, request, ids)
}

// PrepareLibraryDO delegates the public API to the fbd generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) PrepareLibraryDO(ref *library.TemplateRef, request fbd.LibraryDORequest) (*fbd.LibraryDOPlan, error) {
	return (fbd.Generator{Config: g.Config}).PrepareLibraryDO(ref, request)
}

// GenerateLibraryDO delegates the public API to the fbd generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) GenerateLibraryDO(plan *fbd.LibraryDOPlan, ids contracts.IDRange) (contracts.Result, error) {
	return (fbd.Generator{Config: g.Config}).GenerateLibraryDO(plan, ids)
}

// GeneratePLCDiagnostic delegates the public API to the hmi generator.
// It preserves request configuration and returns the domain result without mutation.
func (g Generator) GeneratePLCDiagnostic(plan hmi.PLCDiagnosticPlan, ctx hmi.HMIContext, ids contracts.DiagnosticIDRange) (contracts.Result, error) {
	return (hmi.Generator{Config: g.Config}).GeneratePLCDiagnostic(plan, ctx, ids)
}

// GenerateModuleMapping передаёт проверенный план физических назначений ST-генератору.
// Фиксированных FBD-ветвей нет; библиотечная генерация принимает отдельный план и обязательную библиотеку.
func (g Generator) GenerateModuleMapping(plan contracts.ControllerPlan, ctx contracts.ProgramContext, ids contracts.IDRange) (contracts.Result, error) {
	ctx, _, err := planning.PrepareControllerGeneration(&plan, ctx, ids)
	if err != nil {
		return contracts.Result{}, err
	}
	return st.GenerateModuleST(plan, ctx, ids)
}
