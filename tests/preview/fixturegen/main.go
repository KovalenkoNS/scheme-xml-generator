// Generate real saved XML with the public generator for browser verification.
// Reads the installed library; writes only into the explicit .artifacts test directory.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/config"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	fbdgen "scheme-xml-generator/internal/generator/fbd"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	hmigen "scheme-xml-generator/internal/generator/hmi"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	stgen "scheme-xml-generator/internal/generator/st"
	"scheme-xml-generator/internal/library"
	"strings"
)

// main prepares saved library FBD, ST and HMI outputs in a bounded test directory.
// These files feed browser visual acceptance without changing user output or allocator state.
func main() {
	if len(os.Args) != 2 {
		panic("usage: fixturegen .artifacts/preview-fixtures")
	}
	root, err := filepath.Abs(".artifacts")
	if err != nil {
		panic(err)
	}
	target, err := filepath.Abs(os.Args[1])
	if err != nil {
		panic(err)
	}
	if !strings.HasPrefix(strings.ToLower(target), strings.ToLower(root)+string(os.PathSeparator)) {
		panic("fixture output must stay inside .artifacts")
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		panic(err)
	}
	repository := library.NewRepository("libraries")
	catalog, err := repository.Refresh()
	if err != nil {
		panic(err)
	}
	var ref *library.TemplateRef
	for _, item := range catalog.Templates {
		if item.LibraryFile == "all_lb_sinopec.xml" && item.ID == "12772" {
			ref, _ = repository.Resolve(item.Key)
			break
		}
	}
	if ref == nil {
		panic("actual all_lb_sinopec DIO-1 library template is required for visual verification")
	}
	gen := fbdgen.Generator{Config: config.Default()}
	pous := []fbdrequest.ResolvedPOU{}
	for i := 0; i < 2; i++ {
		pous = append(pous, fbdrequest.ResolvedPOU{Request: fbdrequest.POURequest{Name: fmt.Sprintf("PLC715_DI_%d", i+1)}, Signals: []fbdrequest.ResolvedSignal{{Ref: ref, Request: fbdrequest.SignalRequest{TemplateKey: ref.Key, ObjectName: fmt.Sprintf("_PREVIEW_DI_%d", i+1), NameMode: "base", Invert: i == 0}}}})
	}
	fbd, err := gen.GenerateDocument(fbdrequest.Request{}, pous, xmlidentity.IDRange{T11Start: 10000, CardStart: 20000, POUID: 30000})
	if err != nil {
		panic(err)
	}
	write(target, "fbd.xml", fbd.XML)
	writeModuleFixture(gen, ref, target)
	header := "FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта\n"
	source := header
	for _, module := range []string{"A11-00", "A12-00"} {
		source += fmt.Sprintf("3000_D_SC_B01\tCAB-1\t%s\t0\t_IO_QU*%s*_0.ValueDINT := REAL_TO_DINT(_PREVIEW_AO.OUT, 0.0, 100.0);\t%s\t\tAO\tAN_v1\n", module, module, module)
	}
	plan, err := aomap.Parse([]byte(source))
	if err != nil {
		panic(err)
	}
	request := stgen.AOSTRequest{}
	for i, group := range plan.Groups {
		id := int64(i + 1)
		request.POUs = append(request.POUs, stgen.AOSTPOURequest{GroupKey: group.Key, ModuleCount: len(group.Modules), ModuleIDs: []*int64{&id}})
	}
	stPlans, err := stgen.PrepareAOSTPlans(plan, request)
	if err != nil {
		panic(err)
	}
	st, err := (stgen.Generator{Config: config.Default()}).GenerateAOST(stPlans[0], addressing.DefaultAOContext(), xmlidentity.IDRange{POUID: 40000})
	if err != nil {
		panic(err)
	}
	write(target, "st.xml", st.XML)
	ctx := hmigen.DefaultHMIContext()
	hmiPlans, err := hmigen.PrepareAODiagnosticPlans(plan, []string{"3000_D_SC_B01"}, ctx)
	if err != nil {
		panic(err)
	}
	hmi, err := (hmigen.Generator{Config: config.Default()}).GenerateAODiagnostic(hmiPlans[0], ctx, xmlidentity.DiagnosticIDRange{T11Start: 50000, CardStart: 60000, PageStart: 70000})
	if err != nil {
		panic(err)
	}
	write(target, "hmi.xml", hmi.XML)
	fmt.Printf("Saved generator outputs: %s (actual library FBD, native ST and HMI)\n", target)
}

// writeModuleFixture builds the requested 32-channel DO graph from the real DIO-1 library.
// Its synthetic PLC/module/tag values are test inputs, not application defaults.
func writeModuleFixture(gen fbdgen.Generator, ref *library.TemplateRef, target string) {
	id := int64(17)
	channels := make([]fbdgen.LibraryDOChannelRequest, 32)
	for i := range channels {
		channels[i] = fbdgen.LibraryDOChannelRequest{Channel: i, Tag: fmt.Sprintf("_3101_MXI_%04dA_DDVH", 6001+i), Invert: true}
	}
	request := fbdgen.LibraryDORequest{TemplateKey: ref.Key, PLCName: "2202_S_RC_C03", Context: &fbdrequest.GenerationContext{ControllerTypeName: cpuprofile.ControllerCPU850}, POUs: []fbdgen.LibraryDOPOURequest{{Name: "DO_A70", Modules: []fbdgen.LibraryDOModuleRequest{{Name: "A70-02", ID: &id, Channels: channels}}}}}
	plan, err := gen.PrepareLibraryDO(ref, request)
	if err != nil {
		panic(err)
	}
	result, err := gen.GenerateLibraryDO(plan, xmlidentity.IDRange{T11Start: 100000, CardStart: 200000, POUID: 300000})
	if err != nil {
		panic(err)
	}
	write(target, "module-do.xml", result.XML)
}

// write saves one verified fixture output after main has bounded the test directory.
func write(directory, name string, data []byte) {
	if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
		panic(err)
	}
}
