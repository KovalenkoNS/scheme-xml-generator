package generator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/aomap"
)

func aoSTTestIDs(first int64, count int) []*int64 {
	ids := make([]*int64, count)
	for index := range ids {
		value := first + int64(index)
		ids[index] = &value
	}
	return ids
}

func aoSTTestRequest(plan *aomap.Plan) AOSTRequest {
	request := AOSTRequest{}
	next := map[string]int64{}
	for _, group := range plan.Groups {
		request.POUs = append(request.POUs, AOSTPOURequest{GroupKey: group.Key, ModuleCount: len(group.Modules), ModuleIDs: aoSTTestIDs(next[group.FCS], len(group.Modules))})
		next[group.FCS] += int64(len(group.Modules))
	}
	return request
}

func parseAOSTDocument(t *testing.T, data []byte) outputAOSTDocument {
	t.Helper()
	var doc outputAOSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAOSTActualMapRetainsAllAssignmentsAndPartitionsFCS(t *testing.T) {
	source, err := aomap.Parse(aoMapReadFixture(t, "AO_excell_import_scheme.txt"))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := PrepareAOSTPlans(source, aoSTTestRequest(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 8 {
		t.Fatalf("FCS files = %d, want 8", len(plans))
	}
	modules, assignments, repeats, pous := 0, 0, 0, 0
	for _, plan := range plans {
		modules += plan.ModuleCount
		assignments += plan.AssignmentCount
		repeats += plan.RepeatedAssignmentCount
		pous += len(plan.POUs)
		result, err := (Generator{}).GenerateAOST(plan, DefaultAOMappingContext(), IDRange{POUID: 500})
		if err != nil {
			t.Fatal(err)
		}
		if result.Summary.SignalCount != plan.AssignmentCount || result.Summary.IOModuleCount != plan.ModuleCount || result.Summary.POUCount != len(plan.POUs) || result.Summary.Blocks != 0 || result.Summary.Cards != 0 || result.Summary.T11First != 0 || result.Summary.CardFirst != 0 {
			t.Fatalf("wrong ST summary %+v", result.Summary)
		}
		for _, forbidden := range []string{"<ISAGraf", "<PARAMS", "<GrObj", "<ISAOBJSINFO", "<ISACARDSINFO", "<FONTSTYLES"} {
			if bytes.Contains(result.XML, []byte(forbidden)) {
				t.Errorf("ST contains forbidden FBD section %s", forbidden)
			}
		}
		doc := parseAOSTDocument(t, result.XML)
		for index, pou := range doc.POUS.Items {
			if pou.IsFBD != "0" || pou.Name != plan.POUs[index].Name || !strings.HasSuffix(pou.Name, "_channels") {
				t.Fatalf("incorrect native POU %+v", pou)
			}
			for _, module := range plan.POUs[index].Modules {
				for _, channel := range module.Channels {
					line := fmt.Sprintf("_IO_QU%d_%d.ValueDINT := REAL_TO_DINT(%s.OUT, %s, %s);", module.ID, channel.Channel, channel.Tag, channel.Min, channel.Max)
					if !strings.Contains(pou.Code, line) {
						t.Fatalf("missing assignment %s (Duplicate=%v)", line, channel.Duplicate)
					}
				}
			}
		}
	}
	if modules != 128 || assignments != 512 || repeats != 203 || pous != 21 {
		t.Fatalf("actual ST map: modules=%d assignments=%d repeats=%d POU=%d", modules, assignments, repeats, pous)
	}
}

func TestAOSTCodeMatchesNativeExports(t *testing.T) {
	source, err := aomap.Parse(aoMapReadFixture(t, "AO_excell_import_scheme.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range []struct {
		prefix string
		first  int64
		count  int
	}{{"A11", 24, 14}, {"A12", 38, 4}} {
		t.Run(example.prefix, func(t *testing.T) {
			request := AOSTRequest{POUs: []AOSTPOURequest{{GroupKey: "3000_D_SC_B01:" + example.prefix, ModuleCount: example.count, ModuleIDs: aoSTTestIDs(example.first, example.count)}}}
			plans, err := PrepareAOSTPlans(source, request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (Generator{}).GenerateAOST(plans[0], DefaultAOMappingContext(), IDRange{POUID: 1000})
			if err != nil {
				t.Fatal(err)
			}
			nativePath := filepath.Join("..", "..", "output", "AO_"+example.prefix+"_channels.xml")
			native, err := os.ReadFile(nativePath)
			if os.IsNotExist(err) {
				t.Skipf("native development export %s not present", nativePath)
			}
			if err != nil {
				t.Fatal(err)
			}
			expected, actual := parseAOSTDocument(t, native), parseAOSTDocument(t, result.XML)
			// A12's export has one extra empty line before END_PROGRAM; all
			// nonempty lines, including every repeated assignment, must match.
			normalizeBlankLines := func(code string) string {
				for strings.Contains(code, "\n\n\n") {
					code = strings.ReplaceAll(code, "\n\n\n", "\n\n")
				}
				return code
			}
			if normalizeBlankLines(actual.POUS.Items[0].Code) != normalizeBlankLines(expected.POUS.Items[0].Code) {
				t.Fatalf("ST differs from native %s\nactual:\n%s\nexpected:\n%s", example.prefix, actual.POUS.Items[0].Code, expected.POUS.Items[0].Code)
			}
			if actual.Common != expected.Common {
				t.Fatal("native Common changed")
			}
			if !bytes.Contains(result.XML, []byte("\r\n\r\n_IO_QU")) || !bytes.HasPrefix(result.XML, utf8BOM) {
				t.Fatal("ST must preserve the application's UTF-8 BOM and CRLF text conventions")
			}
		})
	}
}

func TestAOSTExtraModulesRangesSnapshotAndNumericOrder(t *testing.T) {
	source, err := aomap.Parse([]byte(aoMapTestHeader + aoMapTestRow("FCS1", "A11-05", 0, "_FIRST") + aoMapTestRow("FCS1", "A11-02", 0, "_SECOND")))
	if err != nil {
		t.Fatal(err)
	}
	// Request IDs apply to numeric module order, irrespective of the source slice.
	source.Groups[0].Modules[0], source.Groups[0].Modules[1] = source.Groups[0].Modules[1], source.Groups[0].Modules[0]
	source.Groups[0].Modules[0].Channels[0].Min = "-1.25e1"
	source.Groups[0].Modules[0].Channels[0].Max = "+250.5"
	before := fmt.Sprintf("%#v", source.Groups)
	request := AOSTRequest{POUs: []AOSTPOURequest{{GroupKey: source.Groups[0].Key, ModuleCount: 4, ModuleIDs: aoSTTestIDs(0, 4)}}}
	plans, err := PrepareAOSTPlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%#v", source.Groups); got != before {
		t.Fatal("preparation mutated source")
	}
	modules := plans[0].POUs[0].Modules
	if got := []string{modules[0].Name, modules[1].Name, modules[2].Name, modules[3].Name}; !reflect.DeepEqual(got, []string{"A11_02", "A11_05", "A11_06", "A11_07"}) {
		t.Fatalf("wrong ordering/additional names %v", got)
	}
	result, err := (Generator{}).GenerateAOST(plans[0], DefaultAOMappingContext(), IDRange{POUID: 1})
	if err != nil {
		t.Fatal(err)
	}
	code := parseAOSTDocument(t, result.XML).POUS.Items[0].Code
	if !strings.Contains(code, "_IO_QU1_0.ValueDINT := REAL_TO_DINT(_FIRST.OUT, -1.25e1, +250.5);") || !strings.Contains(code, "_IO_QU2_3.ValueDINT := REAL_TO_DINT(_FCS1_A11_06_3.OUT, 0.0, 100.0);") {
		t.Fatal("source scale or extra reserve changed", code)
	}
	plans[0].POUs[0].Modules[0].Channels[0].Tag = "_CHANGED"
	if got := fmt.Sprintf("%#v", source.Groups); got != before {
		t.Fatal("prepared plan aliases source channels")
	}
}

func TestAOSTRejectsInvalidRequestsBeforeGeneration(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*aomap.Plan, *AOSTRequest)
	}{
		{"no selections", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs = nil }},
		{"unknown POU", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs[0].GroupKey = "missing" }},
		{"repeat selection", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs = append(r.POUs, r.POUs[0]) }},
		{"shrink modules", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs[0].ModuleCount = 0; r.POUs[0].ModuleIDs = nil }},
		{"ID count mismatch", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs[0].ModuleCount = 2 }},
		{"null ID", func(_ *aomap.Plan, r *AOSTRequest) { r.POUs[0].ModuleIDs[0] = nil }},
		{"negative ID", func(_ *aomap.Plan, r *AOSTRequest) { *r.POUs[0].ModuleIDs[0] = -1 }},
		{"overflow ID", func(_ *aomap.Plan, r *AOSTRequest) { *r.POUs[0].ModuleIDs[0] = maxTransportID + 1 }},
		{"repeat ID across POU", func(_ *aomap.Plan, r *AOSTRequest) { *r.POUs[1].ModuleIDs[0] = *r.POUs[0].ModuleIDs[0] }},
		{"blank FCS", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].FCS = "" }},
		{"bad scale", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Min = "NaN" }},
		{"scale injection", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Max = "1);BAD();(" }},
		{"scale overflow", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Max = "1e999" }},
		{"scale equal", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Max = "0.0" }},
		{"tag injection", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Tag = "_TAG);BAD(" }},
		{"channel order", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels[0].Channel = 2 }},
		{"empty module", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].Channels = nil }},
		{"wrong template", func(p *aomap.Plan, _ *AOSTRequest) { p.Groups[0].Modules[0].ObjectType = "Other" }},
		{"repeat module", func(p *aomap.Plan, r *AOSTRequest) {
			p.Groups[0].Modules = append(p.Groups[0].Modules, p.Groups[0].Modules[0])
			r.POUs[0].ModuleCount = 2
			r.POUs[0].ModuleIDs = aoSTTestIDs(10, 2)
		}},
		{"limit per POU", func(_ *aomap.Plan, r *AOSTRequest) {
			r.POUs[0].ModuleCount = 4097
			r.POUs[0].ModuleIDs = aoSTTestIDs(0, 4097)
		}},
		{"limit total", func(_ *aomap.Plan, r *AOSTRequest) {
			for i := range r.POUs {
				r.POUs[i].ModuleCount = 2049
				r.POUs[i].ModuleIDs = aoSTTestIDs(int64(i*2049), 2049)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := aoMapSmallPlan(t, 2)
			request := aoSTTestRequest(source)
			test.edit(source, &request)
			if _, err := PrepareAOSTPlans(source, request); err == nil {
				t.Fatal("accepted invalid ST request")
			}
		})
	}
}

func TestAOSTIDsAreControllerScopedAndRepeatedTagsNotFlagDependent(t *testing.T) {
	source, err := aomap.Parse([]byte(aoMapTestHeader + aoMapTestRow("FCS1", "A11-00", 0, "_TAG") + aoMapTestRow("FCS1", "A12-00", 0, "_TAG") + aoMapTestRow("FCS2", "A11-00", 0, "_TAG")))
	if err != nil {
		t.Fatal(err)
	}
	for g := range source.Groups {
		for m := range source.Groups[g].Modules {
			for c := range source.Groups[g].Modules[m].Channels {
				source.Groups[g].Modules[m].Channels[c].Duplicate = false
			}
		}
	}
	plans, err := PrepareAOSTPlans(source, aoSTTestRequest(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].POUs[0].Modules[0].ID != 0 || plans[1].POUs[0].Modules[0].ID != 0 || plans[0].RepeatedAssignmentCount != 1 || plans[1].RepeatedAssignmentCount != 0 {
		t.Fatalf("wrong controller scopes: %+v", plans)
	}
}

func TestAOSTContextAndTransportRanges(t *testing.T) {
	source := aoMapSmallPlan(t, 2)
	plans, err := PrepareAOSTPlans(source, aoSTTestRequest(source))
	if err != nil {
		t.Fatal(err)
	}
	ctx := DefaultAOMappingContext()
	ctx.Project, ctx.ControllerID, ctx.ResourceID, ctx.POUNumber, ctx.GroupID = "example&project", "0", "0", "0", "0"
	result, err := (Generator{}).GenerateAOST(plans[0], ctx, IDRange{POUID: maxTransportID - 1})
	if err != nil {
		t.Fatal(err)
	}
	doc := parseAOSTDocument(t, result.XML)
	if doc.Common.Project != "example&project" || doc.Common.ControllerID != "0" || doc.POUS.Items[0].Number != "0" || doc.POUS.Items[1].Number != "1" {
		t.Fatalf("overrides lost: %+v", doc)
	}
	if _, err := (Generator{}).GenerateAOST(plans[0], ctx, IDRange{POUID: maxTransportID}); err == nil {
		t.Fatal("POU ID overflow allowed")
	}
	if _, err := (Generator{}).GenerateAOST(plans[0], ctx, IDRange{}); err == nil {
		t.Fatal("zero POU ID allowed")
	}
	ctx.POUNumber = fmt.Sprint(maxTransportID)
	if _, err := (Generator{}).GenerateAOST(plans[0], ctx, IDRange{POUID: 1}); err == nil {
		t.Fatal("POUNum overflow allowed")
	}
}
