package generator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"scheme-xml-generator/internal/aomap"
)

func aoDiagnosticTestSelection(source *aomap.Plan) []string {
	seen := map[string]bool{}
	var selection []string
	for _, group := range source.Groups {
		if !seen[group.FCS] {
			selection = append(selection, group.FCS)
			seen[group.FCS] = true
		}
	}
	return selection
}

func parseAODiagnosticDocument(t *testing.T, data []byte) outputAODiagnosticDocument {
	t.Helper()
	var doc outputAODiagnosticDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAODiagnosticNativeProfileWithoutDevelopmentFixture(t *testing.T) {
	source, err := aomap.Parse([]byte(aoMapTestHeader + aoMapTestRow("3000_D_SC_B01", "A12-00", 0, "_FROM_CURRENT_TXT")))
	if err != nil {
		t.Fatal(err)
	}
	ctx := DefaultAODiagnosticContext()
	plans, err := PrepareAODiagnosticPlans(source, []string{"3000_D_SC_B01"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Generator{}).GenerateAODiagnostic(plans[0], ctx, DiagnosticIDRange{T11Start: 590411, CardStart: 205414, PageStart: 5248})
	if err != nil {
		t.Fatal(err)
	}
	doc := parseAODiagnosticDocument(t, result.XML)
	if doc.XMLName.Local != "BufScada" || doc.Common != (outputAODiagnosticCommon{Version: "29", Project: DefaultAOMappingContext().Project}) || len(doc.Pages) != 1 || len(doc.Cards) != 4 {
		t.Fatalf("wrong diagnostic document %+v", doc)
	}
	page := doc.Pages[0]
	expectedPage := outputAODiagnosticPage{IDAttribute: "5248", ID: "5248", Name: "AO_B01_A12_00_AOC4H", TemplateID: "0", ForMarka: "0", Background: "536870913", DParams: "66", Height: "152", GridSize: "10", Width: "1200", Number: "0", PrintWidth: "600", PrintHeight: "800", PrintPageA4: "8", FrameNumber: "5", Srez: "1", PageLayers: page.PageLayers}
	if !reflect.DeepEqual(page, expectedPage) {
		t.Fatalf("native page parameters changed: %+v", page)
	}
	if len(page.PageLayers) != 1 || page.PageLayers[0].Number != "1" || page.PageLayers[0].Visible != "1" || page.PageLayers[0].Name != "[\ufba0\U000ecbab\u00f7\u0b68\u00fe]" || len(page.PageLayers[0].Primitives) != 5 {
		t.Fatal("native layer changed")
	}
	for index, primitive := range page.PageLayers[0].Primitives {
		expected := outputAODiagnosticPrimitive{T11ID: strconv.Itoa(590411 + index), X: "70", Y: strconv.Itoa(54 + 24*(index-1)), Width: "1080", Height: "24", ObjectType: "8", GroupNumber: "0", DrawType: "0", PenParams: "1", PenColor: "0", BrushColor: "16777215", GradColor: "536870911", Params: "[MODE]=-1\n[TEXT]=\n[FONTID]=0\n[USERFONT]=8;Arial;0;0;\n", ObjectMSID: "3679", CardID: strconv.Itoa(205413 + index)}
		if index == 0 {
			expected.X, expected.Y, expected.Width, expected.Height, expected.ObjectMSID, expected.CardID = "0", "0", "1200", "152", "3655", "0"
		}
		if primitive != expected {
			t.Fatalf("primitive %d changed: %+v", index, primitive)
		}
	}
	if !reflect.DeepEqual(doc.ColorStyles, []outputAODiagnosticColor{{ID: "2", Name: "Фон мнемосхемы", Color: "14935011"}}) || !reflect.DeepEqual(doc.PageMS, []outputAODiagnosticPageMS{{ID: "3679", Info: "2//(AN_v1)/(8x_Diag_МФК1500_HART_AO)"}, {ID: "3655", Info: "2//(AI_DIAG16_AD3v1_kvit)/(8x_AO_4_DIAG)"}}) {
		t.Fatal("native color/MS references changed")
	}
	for channel, card := range doc.Cards {
		tag := fmt.Sprintf("_3000_D_SC_B01_A12_00_%d", channel)
		if channel == 0 {
			tag = "_FROM_CURRENT_TXT"
		}
		if card.ID != strconv.Itoa(205414+channel) || card.Info != "2/3000_D_SC_B01/1/"+tag+"/(AN_v1)" {
			t.Fatalf("card %d does not use current map: %+v", channel, card)
		}
	}
	for _, text := range []string{"<EXDATA/>", "<DISC/>", "<LAYERS/>", "<SCRIPTCODE/>", "[MODE]=-1\r\n[TEXT]=\r\n[FONTID]=0\r\n[USERFONT]=8;Arial;0;0;\r\n"} {
		if !bytes.Contains(result.XML, []byte(text)) {
			t.Errorf("native lexical form missing: %q", text)
		}
	}
	for _, forbidden := range []string{"BufScadaPOUS", "<OnePOU", "<STCODE", "ISACARDSINFO", "ISAOBJSINFO", "<ISAGraf", "ControllerID=", "ResuorceID="} {
		if bytes.Contains(result.XML, []byte(forbidden)) {
			t.Errorf("panel document contains POU metadata: %s", forbidden)
		}
	}
	if result.Summary.FrameCount != 1 || result.Summary.Graphics != 5 || result.Summary.Cards != 4 || result.Summary.SignalCount != 4 || result.Summary.POUCount != 0 || result.Summary.POUID != 0 || result.Summary.Blocks != 0 || len(result.Summary.Frames) != 1 || result.Summary.Frames[0].ID != 5248 || len(result.Warnings) == 0 {
		t.Fatalf("wrong summary or missing import prerequisite warning: %+v", result)
	}
}

// Compare XML tokens as well as the typed model so an unmodelled native
// attribute/element cannot silently disappear. Only transport primitive IDs
// and the example's old card tags are variable; page/card IDs are fixed here.
func aoDiagnosticNativeTokens(t *testing.T, data []byte) []string {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	var tokens []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			tokens = append(tokens, "<"+token.Name.Local)
			for _, attr := range token.Attr {
				if attr.Name.Local != "SourceT11ID" && attr.Name.Local != "CardInfo" {
					tokens = append(tokens, attr.Name.Local+"="+attr.Value)
				}
			}
		case xml.EndElement:
			tokens = append(tokens, "</"+token.Name.Local)
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				tokens = append(tokens, string(token))
			}
		}
	}
	return tokens
}

func TestAODiagnosticMatchesNativeDevelopmentExport(t *testing.T) {
	nativePath := filepath.Join("..", "..", "output", "diagnostic.xml")
	native, err := os.ReadFile(nativePath)
	if os.IsNotExist(err) {
		t.Skipf("native development export %s not installed", nativePath)
	}
	if err != nil {
		t.Fatal(err)
	}
	source, err := aomap.Parse([]byte(aoMapTestHeader + aoMapTestRow("3000_D_SC_B01", "A12-00", 0, "_FRESH_MAP_TAG")))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := PrepareAODiagnosticPlans(source, []string{"3000_D_SC_B01"}, DefaultAODiagnosticContext())
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Generator{}).GenerateAODiagnostic(plans[0], DefaultAODiagnosticContext(), DiagnosticIDRange{T11Start: 590411, CardStart: 205414, PageStart: 5248})
	if err != nil {
		t.Fatal(err)
	}
	actualTokens, expectedTokens := aoDiagnosticNativeTokens(t, result.XML), aoDiagnosticNativeTokens(t, native)
	if !reflect.DeepEqual(actualTokens, expectedTokens) {
		t.Fatalf("native diagnostic profile differs\nactual: %v\nexpected: %v", actualTokens, expectedTokens)
	}
}

func TestAODiagnosticActualMapHasOneFilePerPLCAndOneFramePerModule(t *testing.T) {
	source, err := aomap.Parse(aoMapReadFixture(t, "AO_excell_import_scheme.txt"))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := PrepareAODiagnosticPlans(source, aoDiagnosticTestSelection(source), DefaultAODiagnosticContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 8 {
		t.Fatalf("got %d PLC files, want 8", len(plans))
	}
	ids := DiagnosticIDRange{T11Start: 5000000, CardStart: 900000, PageStart: 1000000}
	frames, signals, cards := 0, 0, 0
	allT11, allCards, allPages := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, plan := range plans {
		result, err := (Generator{}).GenerateAODiagnostic(plan, DefaultAODiagnosticContext(), ids)
		if err != nil {
			t.Fatal(err)
		}
		doc := parseAODiagnosticDocument(t, result.XML)
		byCard := map[string]string{}
		byTag := map[string]string{}
		for _, card := range doc.Cards {
			if allCards[card.ID] || !strings.HasPrefix(card.Info, "2/"+plan.FCS+"/1/") {
				t.Fatalf("card has wrong PLC or nonunique transport ID: %+v", card)
			}
			allCards[card.ID], byCard[card.ID] = true, card.Info
		}
		for index, page := range doc.Pages {
			frame := plan.Frames[index]
			if page.Name != frame.Name || allPages[page.ID] {
				t.Fatal("wrong page identity or repeated PageID")
			}
			allPages[page.ID] = true
			if strings.Contains(plan.FCS, "_SC_B07_1") && !strings.HasPrefix(page.Name, "AO_B07_1_") || strings.Contains(plan.FCS, "_SC_B07_2") && !strings.HasPrefix(page.Name, "AO_B07_2_") {
				t.Fatalf("controller suffix truncated: %s", page.Name)
			}
			for primitiveIndex, primitive := range page.PageLayers[0].Primitives {
				if allT11[primitive.T11ID] {
					t.Fatal("repeated SourceT11ID")
				}
				allT11[primitive.T11ID] = true
				if primitiveIndex == 0 {
					continue
				}
				tag := frame.Tags[primitiveIndex-1]
				if !strings.EqualFold(byCard[primitive.CardID], "2/"+plan.FCS+"/1/"+tag+"/(AN_v1)") {
					t.Fatalf("wrong binding for %s channel %d", page.Name, primitiveIndex-1)
				}
				key := strings.ToUpper(tag)
				if prior, exists := byTag[key]; exists && primitive.CardID != prior {
					t.Fatal("repeated displayed tag must share existing card reference")
				}
				byTag[key] = primitive.CardID
			}
		}
		frames, signals, cards = frames+plan.FrameCount, signals+plan.SignalCount, cards+plan.CardCount
		ids.T11Start += int64(plan.T11Count)
		ids.CardStart += int64(plan.CardCount)
		ids.PageStart += int64(plan.FrameCount)
	}
	if frames != 128 || signals != 512 || cards != 309 || len(allT11) != 640 || len(allPages) != 128 {
		t.Fatalf("lost or merged diagnostic positions: frames=%d rows=%d cards=%d graphics=%d", frames, signals, cards, len(allT11))
	}
}

func TestAODiagnosticSelectionSnapshotAndRepeatedTags(t *testing.T) {
	source, err := aomap.Parse([]byte(aoMapTestHeader + aoMapTestRow("FCS1", "A11-03", 0, "_shared") + aoMapTestRow("FCS1", "A11-02", 0, "_SHARED") + aoMapTestRow("FCS2", "A11-02", 0, "_SHARED")))
	if err != nil {
		t.Fatal(err)
	}
	before := fmt.Sprintf("%#v", source)
	// Deliberately absent/stale graphic annotations must not affect panel rows.
	plans, err := PrepareAODiagnosticPlans(source, []string{"FCS2", "FCS1"}, DefaultAODiagnosticContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].FCS != "FCS1" || plans[1].FCS != "FCS2" || plans[0].FrameCount != 2 || plans[0].SignalCount != 8 || plans[0].CardCount != 7 || plans[1].CardCount != 4 || plans[0].Frames[0].Name != "AO_FCS1_A11_02_AOC4H" || plans[0].Frames[1].Module != "A11_03" {
		t.Fatalf("wrong grouping, repeat policy, order or fallback name: %+v", plans)
	}
	plans[0].Frames[0].Tags[0] = "_CHANGED"
	if got := fmt.Sprintf("%#v", source); got != before {
		t.Fatal("prepared diagnostic snapshot aliases or mutates source")
	}
	selected, err := PrepareAODiagnosticPlans(source, []string{"FCS2"}, DefaultAODiagnosticContext())
	if err != nil || len(selected) != 1 || selected[0].FCS != "FCS2" || selected[0].FrameCount != 1 {
		t.Fatalf("selection not respected: %+v, %v", selected, err)
	}
}

func TestAODiagnosticRejectsInvalidSelectionAndSource(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*aomap.Plan, *[]string)
	}{
		{"empty selection", func(_ *aomap.Plan, s *[]string) { *s = nil }},
		{"unknown PLC", func(_ *aomap.Plan, s *[]string) { *s = []string{"missing"} }},
		{"repeat PLC", func(_ *aomap.Plan, s *[]string) { *s = []string{"FCS1", "FCS1"} }},
		{"empty source", func(p *aomap.Plan, _ *[]string) { p.Groups = nil }},
		{"duplicate group", func(p *aomap.Plan, _ *[]string) { p.Groups = append(p.Groups, p.Groups[0]) }},
		{"wrong prefix", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Prefix = "A11_bad" }},
		{"module wrong group", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules[0].Name = "A12_00" }},
		{"empty modules", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules = nil }},
		{"duplicate modules", func(p *aomap.Plan, _ *[]string) {
			p.Groups[0].Modules = append(p.Groups[0].Modules, p.Groups[0].Modules[0])
		}},
		{"wrong template", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules[0].ObjectType = "DI32" }},
		{"missing channel", func(p *aomap.Plan, _ *[]string) {
			p.Groups[0].Modules[0].Channels = p.Groups[0].Modules[0].Channels[:3]
		}},
		{"wrong channel", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules[0].Channels[0].Channel = 3 }},
		{"empty tag", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules[0].Channels[0].Tag = "" }},
		{"tag card-path injection", func(p *aomap.Plan, _ *[]string) { p.Groups[0].Modules[0].Channels[0].Tag = "_TAG/other" }},
		{"FCS path injection", func(p *aomap.Plan, s *[]string) { p.Groups[0].FCS = "FCS/other"; *s = []string{p.Groups[0].FCS} }},
		{"FCS empty label", func(p *aomap.Plan, s *[]string) { p.Groups[0].FCS = "3000_D_SC_"; *s = []string{p.Groups[0].FCS} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := aoMapSmallPlan(t, 1)
			selection := []string{"FCS1"}
			test.edit(source, &selection)
			if _, err := PrepareAODiagnosticPlans(source, selection, DefaultAODiagnosticContext()); err == nil {
				t.Fatal("invalid diagnostic request accepted")
			}
		})
	}
	if _, err := PrepareAODiagnosticPlans(nil, []string{"FCS1"}, DefaultAODiagnosticContext()); err == nil {
		t.Fatal("nil source accepted")
	}
}

func TestAODiagnosticContextAndSigned32BitBoundaries(t *testing.T) {
	source := aoMapSmallPlan(t, 1)
	ctx := DefaultAODiagnosticContext()
	plans, err := PrepareAODiagnosticPlans(source, []string{"FCS1"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Project, ctx.Version, ctx.ResourceNumber = "project&example<>", "0", "1"
	last := DiagnosticIDRange{T11Start: maxTransportID - 4, CardStart: maxTransportID - 3, PageStart: maxTransportID}
	result, err := (Generator{}).GenerateAODiagnostic(plans[0], ctx, last)
	if err != nil {
		t.Fatal(err)
	}
	doc := parseAODiagnosticDocument(t, result.XML)
	if doc.Common.Project != ctx.Project || doc.Common.Version != "0" || !strings.HasPrefix(doc.Cards[0].Info, "2/FCS1/1/") || doc.Pages[0].ID != strconv.FormatInt(maxTransportID, 10) || result.Summary.T11Last != maxTransportID || result.Summary.CardLast != maxTransportID {
		t.Fatal("explicit zero version, positive resource number or last valid ID not preserved")
	}
	for _, test := range []struct {
		name string
		edit func(*AODiagnosticContext, *DiagnosticIDRange)
	}{
		{"T11 overflow", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.T11Start++ }},
		{"card overflow", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.CardStart++ }},
		{"page overflow", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.PageStart++ }},
		{"T11 zero", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.T11Start = 0 }},
		{"card negative", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.CardStart = -1 }},
		{"page zero", func(_ *AODiagnosticContext, ids *DiagnosticIDRange) { ids.PageStart = 0 }},
		{"version overflow", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.Version = "2147483648" }},
		{"resource negative", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.ResourceNumber = "-1" }},
		{"resource zero", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.ResourceNumber = "0" }},
		{"resource path injection", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.ResourceNumber = "1/_BAD" }},
		{"project control", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.Project = "abc\x01" }},
		{"project UTF8", func(ctx *AODiagnosticContext, _ *DiagnosticIDRange) { ctx.Project = string([]byte{0xff}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, ids := DefaultAODiagnosticContext(), last
			test.edit(&ctx, &ids)
			if _, err := (Generator{}).GenerateAODiagnostic(plans[0], ctx, ids); err == nil {
				t.Fatal("invalid diagnostic context or transport range accepted")
			}
		})
	}
	ctx.Project = "bad\x01"
	if _, err := PrepareAODiagnosticPlans(source, []string{"FCS1"}, ctx); err == nil {
		t.Fatal("invalid context was not rejected before allocation")
	}
}

func TestAODiagnosticDirectPlanCannotBypassValidationOrCounts(t *testing.T) {
	source := aoMapSmallPlan(t, 1)
	plans, err := PrepareAODiagnosticPlans(source, []string{"FCS1"}, DefaultAODiagnosticContext())
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	plan.CardCount, plan.T11Count, plan.FrameCount, plan.SignalCount = 0, 0, 0, 0
	ids := DiagnosticIDRange{T11Start: 1000, CardStart: 2000, PageStart: 3000}
	result, err := (Generator{}).GenerateAODiagnostic(plan, DefaultAODiagnosticContext(), ids)
	if err != nil || result.Summary.FrameCount != 1 || result.Summary.Graphics != 5 || result.Summary.Cards != 4 {
		t.Fatalf("stale direct-plan counters were trusted: %+v %v", result.Summary, err)
	}
	plan.Frames = append(plan.Frames, plan.Frames[0])
	if _, err := (Generator{}).GenerateAODiagnostic(plan, DefaultAODiagnosticContext(), ids); err == nil {
		t.Fatal("duplicate page name accepted")
	}
	plan.Frames = plan.Frames[:1]
	plan.Frames[0].Name = "AO_wrong"
	if _, err := (Generator{}).GenerateAODiagnostic(plan, DefaultAODiagnosticContext(), ids); err == nil {
		t.Fatal("wrong frame name accepted")
	}
}
