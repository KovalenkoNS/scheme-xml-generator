// Multi-signal and multi-POU library FBD contracts for allocation, shared definitions and independent settings.
package fbd

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/contracts"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
	"strconv"
	"testing"
)

// TestGenerateDocumentTwoSignalsInOnePOU renders two library signals in one POU and checks shared type definitions,
// transport ranges, placement and explicit POU settings.
func TestGenerateDocumentTwoSignalsInOnePOU(t *testing.T) {
	ref := loadAD3V2Template(t)
	groupID, pouNumber := int64(19022), int64(9)
	width, height := 2500, 1200
	background, dparams := "15461355", "3"
	resolved := []contracts.ResolvedPOU{{
		Request: contracts.POURequest{
			Name: "AD3_GROUP", Description: "Два сигнала", GroupID: &groupID, POUNumber: &pouNumber,
			Page: contracts.PageRequest{Width: &width, Height: &height, BackgroundColor: &background, DParams: &dparams},
		},
		Signals: []contracts.ResolvedSignal{
			{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_TEST_AD3_A", NameMode: "base"}},
			{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_TEST_AD3_B", NameMode: "base"}},
		},
	}}
	requirements, err := RequirementsForDocument(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if requirements != (contracts.DocumentRequirements{T11Count: 18, CardCount: 6, POUCount: 1, SignalCount: 2}) {
		t.Fatalf("requirements=%+v", requirements)
	}

	result, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4100000, CardStart: 810000, POUID: 210000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.POUCount != 1 || result.Summary.SignalCount != 2 || result.Summary.Blocks != 14 || result.Summary.Graphics != 4 || result.Summary.Cards != 6 {
		t.Fatalf("summary=%+v", result.Summary)
	}
	if got := result.Summary.POUs[0].Signals[1].OffsetY - result.Summary.POUs[0].Signals[0].OffsetY; got != 520 {
		t.Fatalf("automatic AD3_v2 vertical step=%d want 520", got)
	}

	document := parseGeneratedDocument(t, result.XML)
	if len(document.POUS.Items) != 1 {
		t.Fatalf("POUs=%d want 1", len(document.POUS.Items))
	}
	pou := document.POUS.Items[0]
	if pou.Name != "AD3_GROUP" || pou.Description != "Два сигнала" || pou.GroupID != "19022" || pou.Number != "9" {
		t.Fatalf("POU header=%+v", pou)
	}
	pageHeight, _ := strconv.Atoi(pou.Params.Height)
	if pou.Params.Width != "2500" || pageHeight < 1200 || pou.Params.Background != background {
		t.Fatalf("POU params=%+v", pou.Params)
	}
	if len(pou.ISAGraf.Blocks.Items) != 14 || len(pou.Graphics.Items) != 4 || len(document.ISACards.Items) != 6 || len(document.ISAObjects.Items) != 1 {
		t.Fatalf("counts blocks=%d graphics=%d cards=%d types=%d", len(pou.ISAGraf.Blocks.Items), len(pou.Graphics.Items), len(document.ISACards.Items), len(document.ISAObjects.Items))
	}
	assertDocumentTransportIDs(t, document, 4100000, 18, 810000, 6)
}

// TestGenerateDocumentTwoPOUsHaveIndependentSettingsAndCoordinates renders separate POUs with different IDs/context
// and verifies local coordinates restart while document types remain shared.
func TestGenerateDocumentTwoPOUsHaveIndependentSettingsAndCoordinates(t *testing.T) {
	ref := loadAD3V2Template(t)
	firstID, secondID := int64(220001), int64(220009)
	firstGroup, secondGroup := int64(11), int64(12)
	firstNumber, secondNumber := int64(21), int64(22)
	firstWidth, secondWidth := 2100, 3100
	firstBackground, secondBackground := "100", "200"
	resolved := []contracts.ResolvedPOU{
		{
			Request: contracts.POURequest{Name: "AD3_POU_A", POUID: &firstID, GroupID: &firstGroup, POUNumber: &firstNumber, Page: contracts.PageRequest{Width: &firstWidth, BackgroundColor: &firstBackground}},
			Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_POU_A_SIGNAL", NameMode: "base"}}},
		},
		{
			Request: contracts.POURequest{Name: "AD3_POU_B", POUID: &secondID, GroupID: &secondGroup, POUNumber: &secondNumber, Page: contracts.PageRequest{Width: &secondWidth, BackgroundColor: &secondBackground}},
			Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_POU_B_SIGNAL", NameMode: "base"}}},
		},
	}
	result, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4200000, CardStart: 820000, POUID: 220000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.POUCount != 2 || result.Summary.SignalCount != 2 || result.Summary.Blocks != 14 || result.Summary.Graphics != 4 || result.Summary.Cards != 6 {
		t.Fatalf("summary=%+v", result.Summary)
	}
	document := parseGeneratedDocument(t, result.XML)
	if len(document.POUS.Items) != 2 {
		t.Fatalf("POUs=%d want 2", len(document.POUS.Items))
	}
	first, second := document.POUS.Items[0], document.POUS.Items[1]
	if first.ID != "220001" || second.ID != "220009" || first.GroupID != "11" || second.GroupID != "12" || first.Number != "21" || second.Number != "22" {
		t.Fatalf("POU headers first=%+v second=%+v", first, second)
	}
	if first.Params.Width != "2100" || second.Params.Width != "3100" || first.Params.Background != "100" || second.Params.Background != "200" {
		t.Fatalf("POU params first=%+v second=%+v", first.Params, second.Params)
	}
	firstOwner := findBlock(t, first, "_POU_A_SIGNAL")
	secondOwner := findBlock(t, second, "_POU_B_SIGNAL")
	if firstOwner.Graphics.X != secondOwner.Graphics.X || firstOwner.Graphics.Y != secondOwner.Graphics.Y {
		t.Fatalf("local coordinate spaces did not restart: first=%+v second=%+v", firstOwner.Graphics, secondOwner.Graphics)
	}
	if len(document.ISAObjects.Items) != 1 {
		t.Fatalf("global ISA type definitions=%d want 1", len(document.ISAObjects.Items))
	}
	assertDocumentTransportIDs(t, document, 4200000, 18, 820000, 6)
}

// TestGenerateDocumentMixesLinkedAndLinklessTemplatesInOnePOU combines two library templates and checks their
// blocks, links and card/transport counts survive one-POU rendering.
func TestGenerateDocumentMixesLinkedAndLinklessTemplatesInOnePOU(t *testing.T) {
	linked := loadTemplateByID(t, "19963")
	linkless := loadTemplateByID(t, "17510")
	pouNumber := int64(61)
	resolved := []contracts.ResolvedPOU{{
		Request: contracts.POURequest{Name: "MIXED_POU", POUNumber: &pouNumber},
		Signals: []contracts.ResolvedSignal{
			{Ref: linked, Request: contracts.SignalRequest{TemplateKey: linked.Key, ObjectName: "_MIXED_ADR", NameMode: "base"}},
			{Ref: linkless, Request: contracts.SignalRequest{TemplateKey: linkless.Key, ObjectName: "_MIXED_AD3", NameMode: "base"}},
		},
	}}
	result, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4_500_000, CardStart: 850_000, POUID: 250_000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.POUCount != 1 || result.Summary.SignalCount != 2 || result.Summary.Blocks != 41 || result.Summary.Links != 27 || result.Summary.Graphics != 10 || result.Summary.Cards != 10 {
		t.Fatalf("mixed summary=%+v", result.Summary)
	}
	document := parseGeneratedDocument(t, result.XML)
	if len(document.POUS.Items) != 1 || len(document.POUS.Items[0].ISAGraf.Links.Items) != 27 {
		t.Fatalf("mixed document POU/links=%d/%d", len(document.POUS.Items), len(document.POUS.Items[0].ISAGraf.Links.Items))
	}
	if result.Summary.T11First != 4_500_000 || result.Summary.T11Last != 4_500_077 || len(document.ISACards.Items) != 10 {
		t.Fatalf("mixed transport ranges=%+v cards=%d", result.Summary, len(document.ISACards.Items))
	}
}

// TestGenerateDocumentRejectsDuplicateGlobalCardInfo checks two POUs cannot introduce case-insensitive duplicate
// document-level card identities.
func TestGenerateDocumentRejectsDuplicateGlobalCardInfo(t *testing.T) {
	ref := loadAD3V2Template(t)
	firstNumber, secondNumber := int64(31), int64(32)
	resolved := []contracts.ResolvedPOU{
		{Request: contracts.POURequest{Name: "POU_A", POUNumber: &firstNumber}, Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_DUPLICATE", NameMode: "base"}}}},
		{Request: contracts.POURequest{Name: "POU_B", POUNumber: &secondNumber}, Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_duplicate", NameMode: "base"}}}},
	}
	if _, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4300000, CardStart: 830000, POUID: 230000}); err == nil {
		t.Fatal("duplicate global Card.Info was accepted")
	}
}

// TestGenerateDocumentWithOnlyManualPOUsDoesNotNeedAutomaticPOURange renders with an explicit POU ID and verifies
// no valid automatic POU cursor is required.
func TestGenerateDocumentWithOnlyManualPOUsDoesNotNeedAutomaticPOURange(t *testing.T) {
	ref := loadAD3V2Template(t)
	pouID := int64(42)
	resolved := []contracts.ResolvedPOU{{
		Request: contracts.POURequest{Name: "MANUAL_POU", POUID: &pouID},
		Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_MANUAL_SIGNAL", NameMode: "base"}}},
	}}
	result, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4_600_000, CardStart: 860_000, POUID: contracts.MaxTransportID + 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.POUID != pouID {
		t.Fatalf("POU ID=%d want %d", result.Summary.POUID, pouID)
	}
}

// TestGenerateDocumentUsesOnlyActualPOUIDsAtSigned32Boundary checks mixed manual/automatic POU allocation accepts
// the final valid automatic ID without reserving unused IDs.
func TestGenerateDocumentUsesOnlyActualPOUIDsAtSigned32Boundary(t *testing.T) {
	ref := loadAD3V2Template(t)
	manualPOU := int64(1)
	resolved := []contracts.ResolvedPOU{
		{Request: contracts.POURequest{Name: "AUTO_LAST"}, Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_AUTO_LAST", NameMode: "base"}}}},
		{Request: contracts.POURequest{Name: "MANUAL_FIRST", POUID: &manualPOU}, Signals: []contracts.ResolvedSignal{{Ref: ref, Request: contracts.SignalRequest{TemplateKey: ref.Key, ObjectName: "_MANUAL_FIRST", NameMode: "base"}}}},
	}
	result, err := (Generator{Config: config.Default()}).GenerateDocument(contracts.Request{}, resolved, contracts.IDRange{T11Start: 4_700_000, CardStart: 870_000, POUID: contracts.MaxTransportID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.POUs[0].POUID != contracts.MaxTransportID || result.Summary.POUs[1].POUID != manualPOU {
		t.Fatalf("unexpected POU IDs: %+v", result.Summary.POUs)
	}
}

// TestLegacyGenerateStillProducesOnePOU checks the earlier single-signal library API still returns exactly one POU
// with its requested name.
func TestLegacyGenerateStillProducesOnePOU(t *testing.T) {
	ref := loadAD3V2Template(t)
	result, err := (Generator{Config: config.Default()}).Generate(ref, contracts.Request{ObjectName: "_LEGACY_AD3", POUName: "LEGACY_POU", NameMode: "base"}, contracts.IDRange{T11Start: 4400000, CardStart: 840000, POUID: 240000})
	if err != nil {
		t.Fatal(err)
	}
	document := parseGeneratedDocument(t, result.XML)
	if len(document.POUS.Items) != 1 || result.Summary.POUCount != 1 || result.Summary.SignalCount != 1 || len(result.Summary.POUs) != 1 {
		t.Fatalf("legacy summary=%+v POUs=%d", result.Summary, len(document.POUS.Items))
	}
	if document.POUS.Items[0].Name != "LEGACY_POU" || result.Summary.POUName != "LEGACY_POU" {
		t.Fatalf("legacy POU name was not preserved")
	}
}

// loadAD3V2Template selects the AD3_v2 template from the actual library fixture for multi-signal document tests.
func loadAD3V2Template(t *testing.T) *library.TemplateRef {
	return loadTemplateByID(t, "17510")
}

// loadTemplateByID loads an isolated AD3 catalog and returns a resolved template by its source ID, failing if the
// fixture lacks it.
func loadTemplateByID(t *testing.T, templateID string) *library.TemplateRef {
	t.Helper()
	source := filepath.Join("..", "..", "libraries", "Library AD3_v2.xml")
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
	for _, item := range catalog.Templates {
		if item.ID == templateID {
			ref, ok := repository.Resolve(item.Key)
			if !ok {
				t.Fatalf("template %s reference was not resolved", templateID)
			}
			return ref
		}
	}
	t.Fatalf("template %s was not found", templateID)
	return nil
}

// parseGeneratedDocument decodes generated FBD XML into its serialization model for document-level assertions.
func parseGeneratedDocument(t *testing.T, data []byte) xmlmodel.OutputDocument {
	t.Helper()
	var document xmlmodel.OutputDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

// findBlock locates a generated FBD block by Info and fails explicitly when an expected object is absent.
func findBlock(t *testing.T, pou xmlmodel.OutputPOU, info string) xmlmodel.OutputBlock {
	t.Helper()
	for _, block := range pou.ISAGraf.Blocks.Items {
		if block.Info == info {
			return block
		}
	}
	t.Fatalf("block %s not found", info)
	return xmlmodel.OutputBlock{}
}

// assertDocumentTransportIDs checks every emitted T11/card ID is unique and exactly fills the reserved document
// ranges.
func assertDocumentTransportIDs(t *testing.T, document xmlmodel.OutputDocument, t11Start int64, t11Count int, cardStart int64, cardCount int) {
	t.Helper()
	t11IDs := make(map[int64]struct{}, t11Count)
	for _, pou := range document.POUS.Items {
		for _, block := range pou.ISAGraf.Blocks.Items {
			value, err := strconv.ParseInt(block.T11ID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := t11IDs[value]; exists {
				t.Fatalf("duplicate T11ID=%d", value)
			}
			t11IDs[value] = struct{}{}
		}
		for _, primitive := range pou.Graphics.Items {
			value, err := strconv.ParseInt(primitive.SourceT11ID, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := t11IDs[value]; exists {
				t.Fatalf("duplicate T11ID=%d", value)
			}
			t11IDs[value] = struct{}{}
		}
	}
	if len(t11IDs) != t11Count {
		t.Fatalf("serialized T11 IDs=%d want %d", len(t11IDs), t11Count)
	}
	for value := t11Start; value < t11Start+int64(t11Count); value++ {
		if _, exists := t11IDs[value]; !exists {
			t.Fatalf("T11ID=%d is missing", value)
		}
	}
	cardIDs := make(map[int64]struct{}, cardCount)
	for _, card := range document.ISACards.Items {
		value, err := strconv.ParseInt(card.ID, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := cardIDs[value]; exists {
			t.Fatalf("duplicate cardId=%d", value)
		}
		cardIDs[value] = struct{}{}
	}
	if len(cardIDs) != cardCount {
		t.Fatalf("card IDs=%d want %d", len(cardIDs), cardCount)
	}
	for value := cardStart; value < cardStart+int64(cardCount); value++ {
		if _, exists := cardIDs[value]; !exists {
			t.Fatalf("cardId=%d is missing", value)
		}
	}
}
