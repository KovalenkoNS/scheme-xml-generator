// Library FBD renderer checks for template fidelity, SCADA naming, identity and XML dialect.
package fbd

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/library"
	"strings"
	"testing"
)

type parsedOutput struct {
	Blocks []struct {
		Type   string `xml:"GROBJTYPE,attr"`
		Info   string `xml:"Info,attr"`
		Params struct {
			Text        string  `xml:"T11Text,attr"`
			CO          string  `xml:"CO,attr"`
			CardID      string  `xml:"cardId,attr"`
			ISAObjectID string  `xml:"IsaObjId,attr"`
			Initial     *string `xml:"IV"`
		} `xml:"Params"`
	} `xml:"POUS>OnePOU>ISAGraf>Blocks>Block"`
	Links []struct {
		ConvertTo string `xml:"ConvertTo,attr"`
		Points    struct {
			Value string `xml:"PL,attr"`
		} `xml:"PointList"`
	} `xml:"POUS>OnePOU>ISAGraf>Links>Link"`
	Graphics []struct {
		ID string `xml:"SourceT11ID,attr"`
	} `xml:"POUS>OnePOU>GrObj>OnePrim"`
	Cards []struct {
		ID   string `xml:"ID,attr"`
		Info string `xml:"Info,attr"`
	} `xml:"ISACARDSINFO>rec"`
	Fonts []struct {
		ID string `xml:"ID,attr"`
	} `xml:"FONTSTYLES>rec"`
	ISAObjects []struct {
		ID string `xml:"ID,attr"`
	} `xml:"ISAOBJSINFO>rec"`
}

type parsedPOUHeader struct {
	POU struct {
		Name        string `xml:"NAME,attr"`
		Description string `xml:"Disc,attr"`
	} `xml:"POUS>OnePOU"`
}

// loadTestRepository loads an isolated copy of the AD3 library so renderer tests resolve real template/type
// definitions without mutating source files.
func loadTestRepository(t *testing.T) (*library.Repository, library.Catalog) {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "libraries", "Library AD3_v2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "Library AD3_v2.xml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(directory)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	return repository, catalog
}

// TestGenerateAllEightTemplates renders every template in the AD3 catalog and checks source-derived blocks, links,
// cards, types, fonts and lexical metadata.
func TestGenerateAllEightTemplates(t *testing.T) {
	repository, catalog := loadTestRepository(t)
	gen := Generator{Config: config.Default()}
	expectedISAObjects := map[string]int{"17509": 4, "17510": 1, "18402": 5, "18555": 3, "18556": 5, "18558": 6, "19962": 7, "19963": 7}
	expectedFonts := map[string]int{"17509": 0, "17510": 0, "18402": 0, "18555": 2, "18556": 0, "18558": 0, "19962": 0, "19963": 2}
	for index, summary := range catalog.Templates {
		summary := summary
		t.Run(summary.ID+"_"+summary.Name, func(t *testing.T) {
			ref, ok := repository.Resolve(summary.Key)
			if !ok {
				t.Fatal("template not resolved")
			}
			result, err := gen.Generate(ref, fbdrequest.Request{ObjectName: "_TEST_OBJECT", NameMode: "base"}, xmlidentity.IDRange{T11Start: 3100000 + int64(index)*1000, CardStart: 910000 + int64(index)*100, POUID: 110000 + int64(index)})
			if err != nil {
				t.Fatal(err)
			}
			var parsed parsedOutput
			if err := xml.Unmarshal(result.XML, &parsed); err != nil {
				t.Fatal(err)
			}
			if len(parsed.Blocks) != summary.BlockCount || len(parsed.Links) != summary.LinkCount || len(parsed.Graphics) != summary.GraphicCount {
				t.Fatalf("output counts=%d/%d/%d want %d/%d/%d", len(parsed.Blocks), len(parsed.Links), len(parsed.Graphics), summary.BlockCount, summary.LinkCount, summary.GraphicCount)
			}
			if len(parsed.Cards) != summary.CardCount {
				t.Fatalf("cards=%d want %d", len(parsed.Cards), summary.CardCount)
			}
			if len(parsed.ISAObjects) != expectedISAObjects[summary.ID] {
				t.Errorf("ISAOBJSINFO=%d want %d", len(parsed.ISAObjects), expectedISAObjects[summary.ID])
			}
			if len(parsed.Fonts) != expectedFonts[summary.ID] {
				t.Errorf("FONTSTYLES=%d want %d", len(parsed.Fonts), expectedFonts[summary.ID])
			}
			assertSCADADialectOutput(t, result.XML, len(parsed.Fonts))
			assertCardOwnersPrecedeFields(t, parsed)
			operatorNames := map[string]string{"-3": "+", "-4": "/", "-33": "OR"}
			for _, block := range parsed.Blocks {
				if block.Type != "35" {
					continue
				}
				want, ok := operatorNames[block.Params.ISAObjectID]
				if !ok || block.Info != want {
					t.Errorf("GT35 IsaObjId=%s Info=%q want %q", block.Params.ISAObjectID, block.Info, want)
				}
			}
			if (summary.ID == "17509" || summary.ID == "18402" || summary.ID == "18556" || summary.ID == "18558" || summary.ID == "19962") && !bytes.Contains(result.XML, []byte("*")) {
				t.Error("branch/initial marker was lost")
			}
			if summary.ID == "17509" || summary.ID == "18402" || summary.ID == "18556" || summary.ID == "18558" || summary.ID == "19962" {
				found99 := false
				for _, link := range parsed.Links {
					if link.ConvertTo == "99" {
						found99 = true
					}
				}
				if !found99 {
					t.Error("CT=99 was lost")
				}
			}
			if summary.ID == "19962" {
				if !bytes.Contains(result.XML, []byte("<IV>*")) {
					t.Error("leading * in card INITIALVALUE was lost")
				}
				emptyCardlessIV := 0
				for _, block := range parsed.Blocks {
					if block.Type == "37" && block.Params.CardID == "0" && block.Params.Initial != nil && *block.Params.Initial == "" {
						emptyCardlessIV++
					}
				}
				if emptyCardlessIV != 4 {
					t.Errorf("empty cardless GT37 IV=%d want 4", emptyCardlessIV)
				}
			}
		})
	}
}

// TestADRNameAndKnownSignature checks an ADR library template retains canonical object naming, independent POU name
// and expected function-block signatures.
func TestADRNameAndKnownSignature(t *testing.T) {
	repository, catalog := loadTestRepository(t)
	var key string
	for _, item := range catalog.Templates {
		if item.ID == "19963" {
			key = item.Key
		}
	}
	ref, ok := repository.Resolve(key)
	if !ok {
		t.Fatal("template 19963 not found")
	}
	preview, err := PreviewName(ref, "_1110_LZIA_10101_ADR", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if preview.BaseName != "_1110_LZIA_10101" || preview.MatchedPrefix != "_ADR" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	result, err := (Generator{Config: config.Default()}).Generate(ref, fbdrequest.Request{ObjectName: "_1110_LZIA_10101_ADR", POUName: "ADR_test", NameMode: "auto", Description: "Описание карточек"}, xmlidentity.IDRange{T11Start: 3200000, CardStart: 920000, POUID: 120000})
	if err != nil {
		t.Fatal(err)
	}
	var parsed parsedOutput
	if err := xml.Unmarshal(result.XML, &parsed); err != nil {
		t.Fatal(err)
	}
	var header parsedPOUHeader
	if err := xml.Unmarshal(result.XML, &header); err != nil {
		t.Fatal(err)
	}
	if header.POU.Name != "ADR_test" {
		t.Fatalf("OnePOU NAME=%q want ADR_test", header.POU.Name)
	}
	if header.POU.Description != "" {
		t.Fatalf("OnePOU Disc=%q want empty; object description belongs to cards", header.POU.Description)
	}
	if result.Summary.POUName != "ADR_test" {
		t.Fatalf("summary POU name=%q", result.Summary.POUName)
	}
	foundADR, foundHLB17 := false, false
	for _, block := range parsed.Blocks {
		if block.Info == "_1110_LZIA_10101_ADR" {
			foundADR = true
		}
		if block.Info == "_1110_LZIA_10101_HLB" && block.Params.CO == "17" {
			foundHLB17 = true
		}
		if strings.Contains(block.Info, "_ADR_ADR") {
			t.Errorf("duplicated suffix in %s", block.Info)
		}
	}
	if !foundADR || !foundHLB17 {
		t.Fatalf("ADR=%v HLB17=%v", foundADR, foundHLB17)
	}
}

// TestPOUNameIsIndependentFromObjectName verifies FBD POU naming is independent of its signal object and rejects an
// invalid explicit POU identifier.
func TestPOUNameIsIndependentFromObjectName(t *testing.T) {
	repository, catalog := loadTestRepository(t)
	ref, ok := repository.Resolve(catalog.Templates[0].Key)
	if !ok {
		t.Fatal("template not resolved")
	}
	gen := Generator{Config: config.Default()}
	result, err := gen.Generate(ref, fbdrequest.Request{ObjectName: "_1110_LZIA_10101", NameMode: "base"}, xmlidentity.IDRange{T11Start: 3300000, CardStart: 930000, POUID: 130000})
	if err != nil {
		t.Fatal(err)
	}
	var parsed parsedOutput
	if err := xml.Unmarshal(result.XML, &parsed); err != nil {
		t.Fatal(err)
	}
	var header parsedPOUHeader
	if err := xml.Unmarshal(result.XML, &header); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(header.POU.Name, "1110_LZIA_10101") || !strings.HasPrefix(header.POU.Name, "POU_") {
		t.Fatalf("default OnePOU NAME=%q must be independent from object name", header.POU.Name)
	}
	if _, err := gen.Generate(ref, fbdrequest.Request{ObjectName: "_1110_LZIA_10101", POUName: "1110_invalid", NameMode: "base"}, xmlidentity.IDRange{T11Start: 3400000, CardStart: 940000, POUID: 140000}); err == nil {
		t.Fatal("POU name starting with a digit was accepted")
	}
}

// TestObjectRootUsesSCADACanonicalUppercase checks library FBD canonicalizes the object root while preserving
// case-sensitive member suffixes.
func TestObjectRootUsesSCADACanonicalUppercase(t *testing.T) {
	repository, catalog := loadTestRepository(t)
	var key string
	for _, item := range catalog.Templates {
		if item.ID == "17510" {
			key = item.Key
		}
	}
	ref, ok := repository.Resolve(key)
	if !ok {
		t.Fatal("template 17510 not found")
	}
	preview, err := PreviewName(ref, "_1110_LZIA_10101_main", "base")
	if err != nil {
		t.Fatal(err)
	}
	if preview.BaseName != "_1110_LZIA_10101_MAIN" {
		t.Fatalf("canonical base=%q", preview.BaseName)
	}
	result, err := (Generator{Config: config.Default()}).Generate(ref, fbdrequest.Request{ObjectName: "_1110_LZIA_10101_main", POUName: "AD3_test", NameMode: "base"}, xmlidentity.IDRange{T11Start: 3500000, CardStart: 950000, POUID: 150000})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.XML, []byte(`Info="_1110_LZIA_10101_MAIN"`)) || !bytes.Contains(result.XML, []byte(`Info="_1110_LZIA_10101_MAIN.Out"`)) {
		t.Fatal("root card identifier was not uppercased while member suffix case was preserved")
	}
}

// TestRejectsInvalidEndpointDirection passes an unknown direction token to FBD endpoint rewriting and requires
// validation to reject it.
func TestRejectsInvalidEndpointDirection(t *testing.T) {
	idMap := map[string]string{"1": "100"}
	if _, err := rewriteEndpoint("1|sideways|X|0,0,0,0", idMap); err == nil {
		t.Fatal("invalid direction accepted")
	}
}

// TestNormalizeBoolRejectsUnknownLexeme checks supported SCADA boolean spellings normalize consistently and unknown
// values cannot silently become false.
func TestNormalizeBoolRejectsUnknownLexeme(t *testing.T) {
	for input, want := range map[string]string{"": "false", "0": "false", "false": "false", "1": "true", "-1": "true", "TRUE": "true"} {
		got, err := normalizeBool(input)
		if err != nil || got != want {
			t.Errorf("normalizeBool(%q)=(%q,%v), want %q", input, got, err, want)
		}
	}
	if _, err := normalizeBool("sometimes"); err == nil {
		t.Fatal("unknown boolean lexeme was silently converted to false")
	}
}

// TestLegacyCardInfoMustBeUnique checks library card identity validation rejects names differing only in case.
func TestLegacyCardInfoMustBeUnique(t *testing.T) {
	cards := []*generatedCard{
		{Source: library.ISAObject{ID: "1"}, Info: "_OBJECT_DUP"},
		{Source: library.ISAObject{ID: "2"}, Info: "_object_dup"},
	}
	if err := validateUniqueCardInfo(cards); err == nil {
		t.Fatal("duplicate legacy Card.Info was accepted")
	}
}

// assertCardOwnersPrecedeFields walks generated FBD blocks to ensure each card's owner precedes its dependent field
// references.
func assertCardOwnersPrecedeFields(t *testing.T, parsed parsedOutput) {
	t.Helper()
	cardInfo := make(map[string]string, len(parsed.Cards))
	for _, card := range parsed.Cards {
		cardInfo[card.ID] = card.Info
	}
	seen := make(map[string]bool)
	for _, block := range parsed.Blocks {
		cardID := block.Params.CardID
		if cardID == "" || cardID == "0" {
			continue
		}
		if seen[cardID] {
			continue
		}
		seen[cardID] = true
		if block.Info != cardInfo[cardID] || block.Params.Text != "" {
			t.Errorf("first block for card %s is dependent Info=%q T11Text=%q; owner Info=%q must precede fields", cardID, block.Info, block.Params.Text, cardInfo[cardID])
		}
	}
	for cardID := range cardInfo {
		if !seen[cardID] {
			t.Errorf("card %s has no block owner", cardID)
		}
	}
}

// assertSCADADialectOutput checks generated XML keeps the BOM, compact empty elements, required Gotos and correct
// optional font-section behavior.
func assertSCADADialectOutput(t *testing.T, data []byte, fontCount int) {
	t.Helper()
	if !bytes.HasPrefix(data, append(append([]byte{}, xmlcodec.Utf8BOM...), []byte(xml.Header)...)) {
		t.Error("missing UTF-8 BOM or XML header")
	}
	if bytes.Contains(data, []byte("\n  <")) || bytes.Contains(data, []byte("&#xA;")) {
		t.Error("output contains pretty-print whitespace or encoded PARAMS newlines")
	}
	if expanded := xmlcodec.ExpandedEmptyElement(data); expanded != "" {
		t.Errorf("expanded empty element <%s></%s> found", expanded, expanded)
	}
	if !bytes.Contains(data, []byte("<Gotos/>")) {
		t.Error("required <Gotos/> is missing")
	}
	if fontCount == 0 && bytes.Contains(data, []byte("<FONTSTYLES")) {
		t.Error("empty FONTSTYLES must be omitted")
	}
}

// TestGeneratorEnforcesServerSideTextLengths passes oversized object names, descriptions and KLPath values directly
// to FBD APIs and requires rejection.
func TestGeneratorEnforcesServerSideTextLengths(t *testing.T) {
	ref := loadTemplateByID(t, "17510")
	ids := xmlidentity.IDRange{T11Start: 4_800_000, CardStart: 880_000, POUID: 280_000}
	if _, err := PreviewName(ref, strings.Repeat("A", 161), "base"); err == nil {
		t.Fatal("object name longer than 160 characters was accepted")
	}
	if _, err := (Generator{Config: config.Default()}).Generate(ref, fbdrequest.Request{ObjectName: "VALID", POUName: "VALID_POU", NameMode: "base", Description: strings.Repeat("D", 501)}, ids); err == nil {
		t.Fatal("description longer than 500 characters was accepted")
	}
	if _, err := (Generator{Config: config.Default()}).Generate(ref, fbdrequest.Request{ObjectName: "VALID", POUName: "VALID_POU", NameMode: "base", ClusterPath: strings.Repeat("K", 501)}, ids); err == nil {
		t.Fatal("KLPath longer than 500 characters was accepted")
	}
}
