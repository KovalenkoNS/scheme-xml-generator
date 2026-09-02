package generator

import (
	"testing"

	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/library"
)

func TestGenerateUsesContentBoundsForAutomaticOffsetAndPageSize(t *testing.T) {
	settings := config.Default()
	settings.Page.Width = 1
	settings.Page.Height = 1
	settings.Page.MarginRight = 10
	settings.Page.MarginBottom = 10
	ref := layoutFixture()

	result, err := (Generator{Config: settings}).Generate(ref, Request{ObjectName: "_LAYOUT", POUName: "LAYOUT_POU", NameMode: "base"}, IDRange{T11Start: 1000, CardStart: 2000, POUID: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Summary.POUs[0].Signals[0]; got.OffsetX != 800 || got.OffsetY != 250 {
		t.Fatalf("auto offsets=%d/%d want 800/250", got.OffsetX, got.OffsetY)
	}
	document := parseGeneratedDocument(t, result.XML)
	pou := document.POUS.Items[0]
	block := pou.ISAGraf.Blocks.Items[0]
	if block.Graphics.X != "300" || block.Graphics.Y != "100" {
		t.Fatalf("shifted block=%s/%s want 300/100", block.Graphics.X, block.Graphics.Y)
	}
	if pou.Params.Width != "930" || pou.Params.Height != "380" {
		t.Fatalf("page=%s/%s want 930/380", pou.Params.Width, pou.Params.Height)
	}
}

func TestGeneratePreservesExplicitOffsetEvenForNegativeContent(t *testing.T) {
	settings := config.Default()
	settings.Page.Width = 1
	settings.Page.Height = 1
	settings.Page.MarginRight = 10
	settings.Page.MarginBottom = 10
	x, y := 10, 20
	result, err := (Generator{Config: settings}).Generate(layoutFixture(), Request{ObjectName: "_LAYOUT", POUName: "LAYOUT_EXPLICIT", NameMode: "base", OffsetX: &x, OffsetY: &y}, IDRange{T11Start: 1100, CardStart: 2100, POUID: 3100})
	if err != nil {
		t.Fatal(err)
	}
	document := parseGeneratedDocument(t, result.XML)
	block := document.POUS.Items[0].ISAGraf.Blocks.Items[0]
	if block.Graphics.X != "-490" || block.Graphics.Y != "-130" {
		t.Fatalf("explicit translation was changed: %s/%s", block.Graphics.X, block.Graphics.Y)
	}
}

func layoutFixture() *library.TemplateRef {
	template := &library.Template{
		ID: "layout", Name: "LAYOUT", Width: "100", Height: "100",
		Contents: library.Contents{Primitives: []library.Primitive{{
			ID: "1", ObjectType: "34", X: "-500", Y: "-150", Width: "620", Height: "270",
			Params: "[TEXT]=1\r\n[CI]=0\r\n[CO]=1\r\n[COMMENT]=false", CardID: "0", ISAObjectID: "-100",
		}}},
	}
	return &library.TemplateRef{
		Key: "layout", Library: &library.LoadedLibrary{Version: "29", FontStyles: map[string]library.FontStyle{}, TypeSignatures: map[string]library.Signature{}},
		Owner: &library.ObjectType{ID: "owner", Name: "OWNER"}, Template: template,
	}
}
