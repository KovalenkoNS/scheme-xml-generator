// PLC HMI renderer checks for rack hierarchy, native diagnostic symbols and reference integrity.
package hmi

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/iomap"
	"strings"
	"testing"
	"unicode/utf8"
)

// plcDiagnosticTestSource builds a synthetic PLC inventory with front/rear racks and AI/AO/DI/DO modules for HMI
// hierarchy checks.
func plcDiagnosticTestSource() *iomap.Plan {
	c := iomap.Controller{Key: "B01:cabinet", Name: "3000_D_SC_B01", SourceController: "3000_D_SC_B01", Cabinet: "C1", Racks: []iomap.Rack{{Name: "A10", Panel: "front", Order: 0}, {Name: "A12", Panel: "back", Order: 0}}}
	for _, spec := range []struct {
		rack     string
		slot     int
		kind     string
		capacity int
	}{{"A10", 2, "AI16H", 16}, {"A10", 3, "AOC4H", 4}, {"A10", 4, "AOC4H", 4}, {"A10", 5, "DI32", 32}, {"A10", 6, "DO32P", 32}, {"A12", 0, "AI16H", 16}} {
		m := iomap.Module{Name: fmt.Sprintf("%s_%02d", spec.rack, spec.slot), Rack: spec.rack, Slot: spec.slot, Type: spec.kind, Capacity: spec.capacity}
		for channel := 0; channel < spec.capacity; channel++ {
			kind := "D32V"
			if spec.kind == "AI16H" {
				kind = "AD3_v2"
			}
			if spec.kind == "AOC4H" {
				kind = "AN_v1"
			}
			ch := iomap.Channel{Channel: channel, Tag: fmt.Sprintf("_%s_%s_%d", c.Name, m.Name, channel), ObjectType: kind, Reserve: true, SourceRow: channel + 2}
			if spec.kind == "AOC4H" && channel == 0 {
				ch.Tag = "_3107_TV_64101A"
				ch.Reserve = false
				ch.Redundant = spec.slot == 4
			}
			m.Channels = append(m.Channels, ch)
		}
		c.Modules = append(c.Modules, m)
	}
	return &iomap.Plan{SheetName: "IO", Controllers: []iomap.Controller{c}}
}

// buildPLCDiagnosticTest prepares and renders the synthetic inventory using HMI defaults, returning its plan, XML
// bytes and decoded document.
func buildPLCDiagnosticTest(t *testing.T) (PLCDiagnosticPlan, contracts.Result, plcDiagnosticDocument) {
	t.Helper()
	ctx := DefaultHMIContext()
	plans, err := PreparePLCDiagnosticPlans(plcDiagnosticTestSource(), []iomap.Selection{{Key: "B01:cabinet"}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Generator{}).GeneratePLCDiagnostic(plans[0], ctx, contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000})
	if err != nil {
		t.Fatal(err)
	}
	var doc plcDiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	return plans[0], result, doc
}

// TestPLCDiagnosticHierarchyAndNativeProfile checks PLC HMI page nesting, channel geometry, AI/AO symbols, shared
// cards and acknowledgements against the synthetic inventory.
func TestPLCDiagnosticHierarchyAndNativeProfile(t *testing.T) {
	plan, result, doc := buildPLCDiagnosticTest(t)
	if plan.FrameCount != 7 || plan.T11Count != 97 || plan.CardCount != 46 || plan.SignalCount != 104 {
		t.Fatalf("wrong reservation %+v", plan)
	}
	if result.Summary.Graphics != 77 || result.Summary.IOModuleCount != 6 || result.Summary.POUCount != 0 {
		t.Fatalf("summary %+v", result.Summary)
	}
	if !utf8.Valid(result.XML) || bytes.Contains(result.XML, []byte("�")) || bytes.Contains(result.XML, []byte("POUS")) {
		t.Fatal("invalid encoding or program document")
	}
	if len(doc.Pages) != 1 || len(doc.Pages[0].Children.Pages) != 2 || len(doc.Pictures) != 3 || len(doc.Symbols) != 10 {
		t.Fatal("missing hierarchy/resources")
	}
	if err := validatePLCReferences(doc); err != nil {
		t.Fatal(err)
	}
	back, front := doc.Pages[0].Children.Pages[0], doc.Pages[0].Children.Pages[1]
	if !strings.HasSuffix(back.Name, "Задняя панель") || !strings.HasSuffix(front.Name, "Передняя панель") || front.TemplateID != "6580" || len(front.Children.Pages) != 3 || len(back.Children.Pages) != 1 {
		t.Fatal("wrong panels/child ownership")
	}
	ai := front.Children.Pages[0]
	if ai.Name != "AI_B01_A10_02_AI16H" || ai.Height != "448" || ai.DParams != "2" || ai.FrameNumber != "4" || len(ai.PageLayers[0].Primitives) != 17 {
		t.Fatalf("wrong AI page %+v", ai)
	}
	for i, p := range ai.PageLayers[0].Primitives[1:] {
		if p.Y != fmt.Sprint(54+24*i+2*(i/4)) || p.X != "70" || p.Width != "1080" || p.Height != "24" {
			t.Fatalf("wrong channel geometry %d %+v", i, p)
		}
		ms := "4730"
		if i%2 == 1 {
			ms = "4731"
		}
		if p.ObjectMSID != ms {
			t.Fatal("wrong AI alternating symbol")
		}
	}
	ao := front.Children.Pages[1]
	if ao.Name != "AO_B01_A10_03_AOC4H" || ao.Height != "152" || ao.DParams != "66" || ao.FrameNumber != "5" {
		t.Fatal("wrong AO profile")
	}
	if ao.PageLayers[0].Primitives[1].CardID != front.Children.Pages[2].PageLayers[0].Primitives[1].CardID {
		t.Fatal("redundant AO must share signal card without dropping physical row")
	}
	if len(doc.CardParams) != 6 {
		t.Fatal("ack must cover AI/AO/DI/DO")
	}
	var d32 int
	for _, param := range doc.CardParams {
		if strings.Contains(param.Info, "([]D32_КОМ. КВИТИРОВАТЬ)") {
			d32++
		}
	}
	if d32 != 2 {
		t.Fatal("wrong digital ack")
	}
	for _, p := range front.PageLayers[0].Primitives {
		if p.ObjectMSID == "3657" || p.ObjectMSID == "3658" {
			if p.Receptors != nil {
				t.Fatal("native DI/DO must have no invented child receptor")
			}
		}
	}
	if len(result.Warnings) < 2 {
		t.Fatal("missing external dependency and CPU convention warnings")
	}
}

// TestPLCDiagnosticFrameHeadersBindModuleDiagnostics checks every generated AI/AO child-frame header binds to its
// module diagnostic card after PLC naming changes.
func TestPLCDiagnosticFrameHeadersBindModuleDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name           string
		controllerName string
		resource       string
	}{
		{"original controller", "3000_D_SC_B01", "1"},
		{"renamed controller and resource", "3000_D_SC_B07_2", "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := DefaultHMIContext()
			ctx.ResourceNumber = tc.resource
			plans, err := PreparePLCDiagnosticPlans(plcDiagnosticTestSource(), []iomap.Selection{{Key: "B01:cabinet", Name: tc.controllerName}}, ctx)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (Generator{}).GeneratePLCDiagnostic(plans[0], ctx, contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000})
			if err != nil {
				t.Fatal(err)
			}
			var doc plcDiagnosticDocument
			if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
				t.Fatal(err)
			}
			cards := map[string]string{}
			for _, card := range doc.Cards {
				cards[card.ID] = card.Info
			}
			// Both redundant AO modules need their own diagnostic binding,
			// even though their first channel shares the same signal card.
			expected := map[string]string{
				"A10_02_AI16H": "3654",
				"A10_03_AOC4H": "3655",
				"A10_04_AOC4H": "3655",
				"A12_00_AI16H": "3654",
			}
			for _, panel := range doc.Pages[0].Children.Pages {
				moduleCards := map[string]string{}
				for _, p := range panel.PageLayers[0].Primitives {
					if p.ObjectMSID == "3625" || p.ObjectMSID == "3634" {
						if p.Receptors == nil || len(p.Receptors.Items) != 1 {
							t.Fatal("module has no child-frame link")
						}
						moduleCards[p.Receptors.Items[0].Int] = p.CardID
					}
				}
				for _, frame := range panel.Children.Pages {
					module := strings.TrimPrefix(frame.Name, "AI_")
					module = strings.TrimPrefix(module, "AO_")
					module = strings.TrimPrefix(module, strings.TrimPrefix(tc.controllerName, "3000_D_SC_")+"_")
					ms, exists := expected[module]
					if !exists {
						t.Fatalf("unexpected or repeated module frame %s", frame.Name)
					}
					delete(expected, module)
					header := frame.PageLayers[0].Primitives[0]
					if header.ObjectMSID != ms || header.CardID == "0" || header.CardID != moduleCards[frame.ID] {
						t.Fatalf("%s header: symbol=%s card=%s, want symbol=%s module card=%s", frame.Name, header.ObjectMSID, header.CardID, ms, moduleCards[frame.ID])
					}
					want := "2/" + tc.controllerName + "/" + tc.resource + "/_" + tc.controllerName + "_" + module + "/(AI_DIAG16_AD3v1_kvit)"
					if cards[header.CardID] != want {
						t.Fatalf("%s header binding = %q, want %q", frame.Name, cards[header.CardID], want)
					}
				}
			}
			if len(expected) != 0 {
				t.Fatalf("missing module frames: %v", expected)
			}
		})
	}
}

// TestPLCDiagnosticRenameSnapshotAndNoForeignBindings renames a selected PLC during HMI planning and verifies an
// independent snapshot preserves source tags without mutating inventory.
func TestPLCDiagnosticRenameSnapshotAndNoForeignBindings(t *testing.T) {
	source := plcDiagnosticTestSource()
	ctx := DefaultHMIContext()
	plans, err := PreparePLCDiagnosticPlans(source, []iomap.Selection{{Key: "B01:cabinet", Name: "3000_D_SC_B07_2"}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if source.Controllers[0].Modules[0].Channels[0].Tag != "_3000_D_SC_B01_A10_02_0" {
		t.Fatal("prepare mutated source")
	}
	source.Controllers[0].Modules[0].Channels[0].Tag = "CORRUPTED"
	source.Controllers[0].Racks[0].Panel = "invalid"
	result, err := (Generator{}).GeneratePLCDiagnostic(plans[0], ctx, contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result.XML, []byte("3000_D_SC_B01")) || bytes.Contains(result.XML, []byte("CORRUPTED")) || !bytes.Contains(result.XML, []byte("AO_B07_2_A10_03_AOC4H")) || !bytes.Contains(result.XML, []byte("_3107_TV_64101A/(AN_v1)")) {
		t.Fatal("wrong rename/deep snapshot")
	}
}

// TestPLCDiagnosticRejectsInvalidPlans rejects malformed inventory, stale reservation counts and external-ID
// collisions before PLC HMI serialization.
func TestPLCDiagnosticRejectsInvalidPlans(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*iomap.Plan)
	}{
		{"cpu collision", func(p *iomap.Plan) { m := &p.Controllers[0].Modules[0]; m.Slot = 0; m.Name = "A10_00" }},
		{"duplicate rack position", func(p *iomap.Plan) { p.Controllers[0].Racks[1].Panel = "front" }},
		{"unknown type", func(p *iomap.Plan) { p.Controllers[0].Modules[0].Type = "UNKNOWN" }},
		{"missing channel", func(p *iomap.Plan) { p.Controllers[0].Modules[0].Channels = p.Controllers[0].Modules[0].Channels[:15] }},
		{"nonsequential channel", func(p *iomap.Plan) { p.Controllers[0].Modules[0].Channels[1].Channel = 4 }},
		{"invalid real tag", func(p *iomap.Plan) { p.Controllers[0].Modules[1].Channels[0].Tag = "invalid/tag" }},
		{"wrong signal object", func(p *iomap.Plan) { p.Controllers[0].Modules[0].Channels[0].ObjectType = "AN_v1" }},
		{"duplicate module", func(p *iomap.Plan) {
			p.Controllers[0].Modules = append(p.Controllers[0].Modules, p.Controllers[0].Modules[0])
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := plcDiagnosticTestSource()
			tc.mutate(source)
			if _, err := PreparePLCDiagnosticPlans(source, []iomap.Selection{{Key: "B01:cabinet"}}, DefaultHMIContext()); err == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
	plan, _, _ := buildPLCDiagnosticTest(t)
	plan.T11Count--
	if _, err := (Generator{}).GeneratePLCDiagnostic(plan, DefaultHMIContext(), contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000}); err == nil {
		t.Fatal("accepted changed reservation")
	}
	plan.T11Count++
	if _, err := (Generator{}).GeneratePLCDiagnostic(plan, DefaultHMIContext(), contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 4705}); err == nil {
		t.Fatal("accepted collision with external group")
	}
}

// TestPLCDiagnosticReferenceValidationRejectsBrokenEdges corrupts HMI card, page and chart links and requires the
// internal reference validator to reject each dangling edge.
func TestPLCDiagnosticReferenceValidationRejectsBrokenEdges(t *testing.T) {
	_, _, doc := buildPLCDiagnosticTest(t)
	doc.Pages[0].Children.Pages[1].Children.Pages[1].PageLayers[0].Primitives[0].CardID = "0"
	if err := validatePLCReferences(doc); err == nil {
		t.Fatal("accepted unbound AO diagnostic header")
	}
	_, _, doc = buildPLCDiagnosticTest(t)
	front := &doc.Pages[0].Children.Pages[1]
	for i := range front.PageLayers[0].Primitives {
		p := &front.PageLayers[0].Primitives[i]
		if p.Receptors != nil && p.Receptors.Items[0].Type == "1" {
			p.Receptors.Items[0].Int = "9999999"
			break
		}
	}
	if err := validatePLCReferences(doc); err == nil {
		t.Fatal("accepted dangling page receptor")
	}
	_, _, doc = buildPLCDiagnosticTest(t)
	front = &doc.Pages[0].Children.Pages[1]
	for i := range front.PageLayers[0].Primitives {
		p := &front.PageLayers[0].Primitives[i]
		if p.Receptors != nil && p.Receptors.Items[0].Type == "3" {
			p.Receptors.Items[0].Charts.Items[0].ParentID = "7"
			break
		}
	}
	if err := validatePLCReferences(doc); err == nil {
		t.Fatal("accepted wrong chart parent")
	}
}

// TestPLCDiagnosticEmptyRearAndMoreThanTwoRacks renders an expanded front panel and empty rear panel, checking page
// nesting remains valid.
func TestPLCDiagnosticEmptyRearAndMoreThanTwoRacks(t *testing.T) {
	source := plcDiagnosticTestSource()
	source.Controllers[0].Racks[1].Panel = "front"
	source.Controllers[0].Racks[1].Order = 2
	plans, err := PreparePLCDiagnosticPlans(source, []iomap.Selection{{Key: "B01:cabinet"}}, DefaultHMIContext())
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Generator{}).GeneratePLCDiagnostic(plans[0], DefaultHMIContext(), contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000})
	if err != nil {
		t.Fatal(err)
	}
	var doc plcDiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Pages[0].Children.Pages[1].Height != "1400" || len(doc.Pages[0].Children.Pages[0].Children.Pages) != 0 {
		t.Fatal("wrong expanded/empty panel")
	}
}

// TestPLCDiagnosticMixedWidthRacksChooseNaturalFirstCPU checks numeric rack ordering selects one CPU location
// consistently and rejects a module colliding with that location.
func TestPLCDiagnosticMixedWidthRacksChooseNaturalFirstCPU(t *testing.T) {
	source := plcDiagnosticTestSource()
	c := &source.Controllers[0]
	c.Racks = []iomap.Rack{{Name: "A10", Panel: "front", Order: 0}, {Name: "A2", Panel: "back", Order: 0}}
	ai, ao := c.Modules[0], c.Modules[1]
	ai.Name, ai.Rack, ai.Slot = "A2_02", "A2", 2
	// A10 is not the first numeric rack; its slots 00/01 must remain available.
	ao.Name, ao.Rack, ao.Slot = "A10_00", "A10", 0
	c.Modules = []iomap.Module{ao, ai}
	ctx := DefaultHMIContext()
	plans, err := PreparePLCDiagnosticPlans(source, []iomap.Selection{{Key: c.Key}}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Controller.Racks[0].Name != "A2" || plans[0].Controller.Modules[0].Name != "A2_02" {
		t.Fatal("rack/module sorting is not natural numeric order")
	}
	result, err := (Generator{}).GeneratePLCDiagnostic(plans[0], ctx, contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 2000000, PageStart: 3000000})
	if err != nil {
		t.Fatal(err)
	}
	var doc plcDiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	cpuCount := 0
	for _, panel := range doc.Pages[0].Children.Pages {
		for _, p := range panel.PageLayers[0].Primitives {
			if p.ObjectMSID == "4234" {
				cpuCount++
				if !strings.HasSuffix(panel.Name, "Задняя панель") || p.X != "95" || p.Y != "90" {
					t.Fatal("CPU not placed in natural-first A2 rack")
				}
			}
		}
	}
	if cpuCount != 1 {
		t.Fatal("expected exactly one CPU")
	}
	c.Modules[1].Name, c.Modules[1].Slot = "A2_00", 0
	if _, err := PreparePLCDiagnosticPlans(source, []iomap.Selection{{Key: c.Key}}, ctx); err == nil {
		t.Fatal("allowed collision with CPU in natural-first A2 rack")
	}
}

// A supplied development workbook is optional; the self-contained tests above
// always run. This integration exercises every selected PLC, including naming
// collisions resolved by the IO parser and wholly spare physical modules.
func TestPLCDiagnosticFullIOIntegration(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "output", "Full_IO.xlsx"))
	if os.IsNotExist(err) {
		t.Skip("Full_IO development workbook is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	source, err := iomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var selected []iomap.Selection
	for _, controller := range source.Controllers {
		selected = append(selected, iomap.Selection{Key: controller.Key})
	}
	ctx := DefaultHMIContext()
	plans, err := PreparePLCDiagnosticPlans(source, selected, ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := contracts.DiagnosticIDRange{T11Start: 1000000, CardStart: 1000000, PageStart: 1000000}
	var modules, signals int
	for _, plan := range plans {
		result, err := (Generator{}).GeneratePLCDiagnostic(plan, ctx, ids)
		if err != nil {
			t.Fatalf("%s: %v", plan.ControllerName, err)
		}
		if result.Summary.T11Last != ids.T11Start+int64(plan.T11Count)-1 {
			t.Fatal("reservation mismatch")
		}
		modules += result.Summary.IOModuleCount
		signals += result.Summary.SignalCount
		t.Logf("%s: %d modules, %d signals, %d frames, %d primitives, %d transport IDs, %d cards", plan.ControllerName, result.Summary.IOModuleCount, result.Summary.SignalCount, plan.FrameCount, result.Summary.Graphics, plan.T11Count, plan.CardCount)
		ids.T11Start += int64(plan.T11Count)
		ids.CardStart += int64(plan.CardCount)
		ids.PageStart += int64(plan.FrameCount)
	}
	if modules != source.ModuleCount || signals != source.SignalCount {
		t.Fatalf("lost inventory: got %d/%d, want %d/%d", modules, signals, source.ModuleCount, source.SignalCount)
	}
}
