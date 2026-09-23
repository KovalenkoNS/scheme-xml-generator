package iomap

import (
	"archive/zip"
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

const testRelationshipsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const testDocumentRelationshipsNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

func workbookFixtureParts(sheet string) map[string]string {
	return map[string]string{
		"_rels/.rels":                `<Relationships xmlns="` + testRelationshipsNS + `"><Relationship Id="book" Type="` + testDocumentRelationshipsNS + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns:r="` + testDocumentRelationshipsNS + `"><sheets><sheet name="IO" sheetId="1" r:id="sheet"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + testRelationshipsNS + `"><Relationship Id="sheet" Type="` + testDocumentRelationshipsNS + `/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="strings" Type="` + testDocumentRelationshipsNS + `/sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>97</t></si><si><r><t>_3107_</t></r><r><rPr><b/></rPr><t>TV_64101A</t></r><rPh sb="0" eb="4"><t>DO NOT INCLUDE</t></rPh></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><dimension ref="A1:XFD1048576"/><sheetData>` + sheet + `</sheetData></worksheet>`,
	}
}

func zipFixture(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for name, data := range parts {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestReadWorkbookValuesAndOriginalRows(t *testing.T) {
	parts := workbookFixtureParts(`<row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2" t="inlineStr"><is><r><t xml:space="preserve"> AI </t></r><r><t>&amp; AO</t></r></is></c><c r="C2"><v>1.25E+02</v></c><c r="D2"><f>5+7</f><v>12</v></c><c r="E2" t="str"><f>IF(1=1,&quot;&quot;,&quot;x&quot;)</f><v/></c><c r="F2" s="2" t="s"/><c r="G2" s="2" t="s"><v/></c><c r="H2" t="b"><v>0</v></c></row><row r="7"><c r="B7" t="s"><v>0</v></c></row><row r="9"/>`)
	sheets, err := ReadWorkbook(zipFixture(t, parts))
	if err != nil {
		t.Fatal(err)
	}
	want := []Sheet{{Name: "IO", Rows: []Row{
		{Number: 2, Cells: map[string]string{"A": "_3107_TV_64101A", "B": " AI & AO", "C": "1.25E+02", "D": "12", "E": "", "H": "0"}},
		{Number: 7, Cells: map[string]string{"B": "97"}},
		{Number: 9, Cells: map[string]string{}},
	}}}
	if !reflect.DeepEqual(sheets, want) {
		t.Fatalf("got %#v, want %#v", sheets, want)
	}
}

func TestReadWorkbookRelationshipOrderAndNonstandardParts(t *testing.T) {
	parts := map[string]string{
		"_rels/.rels":                `<Relationships><Relationship Id="b" Type="urn:example/officeDocument" Target="/book/custom.xml"/></Relationships>`,
		"book/custom.xml":            `<workbook xmlns:r="` + testDocumentRelationshipsNS + `"><sheets><sheet name="Second physical" r:id="b"/><sheet name="First physical" r:id="a"/></sheets></workbook>`,
		"book/_rels/custom.xml.rels": `<Relationships><Relationship Id="a" Type="urn:example/worksheet" Target="../sheets/one.xml"/><Relationship Id="b" Type="urn:example/worksheet" Target="/sheets/two%20words.xml"/></Relationships>`,
		"sheets/one.xml":             `<worksheet><sheetData><row><c t="inlineStr"><is><t>one</t></is></c></row></sheetData></worksheet>`,
		"sheets/two words.xml":       `<worksheet><sheetData><row r="4"><c><v>2</v></c><c><v>3</v></c></row></sheetData></worksheet>`,
	}
	sheets, err := ReadWorkbook(zipFixture(t, parts))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Second physical" || sheets[0].Rows[0].Cells["A"] != "2" || sheets[0].Rows[0].Cells["B"] != "3" || sheets[1].Rows[0].Cells["A"] != "one" {
		t.Fatalf("bad ordered/custom workbook: %#v", sheets)
	}
}

func TestReadWorkbookRejectsCellErrors(t *testing.T) {
	for _, tc := range []struct {
		name, content, contains string
	}{
		{"uncached", `<row r="2"><c r="B2"><f>1+1</f></c></row>`, "сохранённого результата"},
		{"empty_cached_numeric", `<row r="2"><c r="B2"><f>1+1</f><v/></c></row>`, "сохранённого результата"},
		{"error", `<row r="2"><c r="B2" t="e"><v>#REF!</v></c></row>`, "#REF!"},
		{"shared_index", `<row r="2"><c r="B2" t="s"><v>99</v></c></row>`, "sharedStrings"},
		{"negative_shared_index", `<row r="2"><c r="B2" t="s"><v>-1</v></c></row>`, "sharedStrings"},
		{"unknown_type", `<row r="2"><c r="B2" t="alien"><v>abc</v></c></row>`, "неизвестный тип"},
		{"duplicate_value", `<row r="2"><c r="B2"><v>1</v><v>2</v></c></row>`, "повторный элемент"},
		{"malformed_value", `<row r="2"><c r="B2"><v><a>2</a></v></c></row>`, "некорректная вложенность"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadWorkbook(zipFixture(t, workbookFixtureParts(tc.content)))
			if err == nil || !strings.Contains(err.Error(), tc.contains) || !strings.Contains(err.Error(), `лист "IO"`) || !strings.Contains(err.Error(), "B2") {
				t.Fatalf("expected contextual error %q, got %v", tc.contains, err)
			}
		})
	}
}

func TestReadWorkbookRejectsMalformedRowsAndReferences(t *testing.T) {
	for _, content := range []string{
		`<row r="1"><c r="A2"><v>1</v></c></row>`,
		`<row r="0"/>`,
		`<row r="100001"/>`,
		`<row r="1"/><row r="1"/>`,
		`<row r="2"/><row r="1"/>`,
		`<row r="1"><c r="A1"/><c r="A1"/></row>`,
		`<row r="1"><c r="a1"/></row>`,
		`<row r="1"><c r="$A$1"/></row>`,
		`<row r="1"><c r="A01"/></row>`,
		`<row r="1"><c r="SS1"/></row>`,
		`<row r="1"><c r="A+1"/></row>`,
	} {
		if _, err := ReadWorkbook(zipFixture(t, workbookFixtureParts(content))); err == nil {
			t.Errorf("accepted malformed row %s", content)
		}
	}
	parts := workbookFixtureParts(`<row r="100000"><c r="SR100000"><v>1</v></c></row>`)
	if _, err := ReadWorkbook(zipFixture(t, parts)); err != nil {
		t.Fatalf("inclusive limits rejected: %v", err)
	}
}

func TestReadWorkbookRejectsUnsafeOrBrokenPackages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]string)
	}{
		{"zip_traversal", func(p map[string]string) { p["../bad.xml"] = "x" }},
		{"zip_absolute", func(p map[string]string) { p["/bad.xml"] = "x" }},
		{"zip_backslash", func(p map[string]string) { p[`xl\bad.xml`] = "x" }},
		{"zip_unclean", func(p map[string]string) { p["xl/../bad.xml"] = "x" }},
		{"missing_sheet", func(p map[string]string) { delete(p, "xl/worksheets/sheet1.xml") }},
		{"missing_relationship", func(p map[string]string) { delete(p, "xl/_rels/workbook.xml.rels") }},
		{"duplicate_relationship", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = `<Relationships><Relationship Id="sheet" Type="urn:worksheet" Target="a.xml"/><Relationship Id="sheet" Type="urn:worksheet" Target="b.xml"/></Relationships>`
		}},
		{"external_sheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = `<Relationships><Relationship Id="sheet" Type="urn:x/worksheet" Target="https://example.com/data.xml" TargetMode="External"/></Relationships>`
		}},
		{"url_sheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = `<Relationships><Relationship Id="sheet" Type="urn:x/worksheet" Target="https://example.com/data.xml"/></Relationships>`
		}},
		{"escaped_sheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = `<Relationships><Relationship Id="sheet" Type="urn:x/worksheet" Target="../../escape.xml"/></Relationships>`
		}},
		{"encoded_escape", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = `<Relationships><Relationship Id="sheet" Type="urn:x/worksheet" Target="%2e%2e/%2e%2e/escape.xml"/></Relationships>`
		}},
		{"doctype", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = `<!DOCTYPE worksheet><worksheet/>` }},
		{"wrong_root", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = `<notWorksheet/>` }},
		{"multiple_roots", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = `<worksheet/><worksheet/>` }},
		{"empty_xml", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = `` }},
		{"truncated_xml", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = `<worksheet><sheetData>` }},
		{"deep_xml", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = `<worksheet>` + strings.Repeat("<nested>", 129) + strings.Repeat("</nested>", 129) + `</worksheet>`
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := workbookFixtureParts(`<row r="1"><c r="A1"><v>1</v></c></row>`)
			tc.change(parts)
			if _, err := ReadWorkbook(zipFixture(t, parts)); err == nil {
				t.Fatal("accepted unsafe or broken package")
			}
		})
	}
}

func TestReadWorkbookArchiveLimitsAndDuplicatePart(t *testing.T) {
	if _, err := ReadWorkbook(make([]byte, MaxWorkbookBytes+1)); err == nil || !strings.Contains(err.Error(), "16 МиБ") {
		t.Fatalf("expected compressed limit: %v", err)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for i := 0; i < 2; i++ {
		f, err := w.Create("same.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte("<x/>")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWorkbook(buffer.Bytes()); err == nil || !strings.Contains(err.Error(), "повторяющаяся часть") {
		t.Fatalf("expected duplicate part rejection: %v", err)
	}
	// Set the declared ZIP uncompressed length to exceed the budget without
	// allocating 128 MiB or relying on a compressed bomb in the test suite.
	data := zipFixture(t, map[string]string{"oversized.xml": "<x/>"})
	central := bytes.Index(data, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("missing ZIP central directory")
	}
	n := uint32(maxExpandedBytes + 1)
	for i := 0; i < 4; i++ {
		data[central+24+i] = byte(n >> (8 * i))
	}
	if _, err := ReadWorkbook(data); err == nil || !strings.Contains(err.Error(), "128 МиБ") {
		t.Fatalf("expected expanded size limit: %v", err)
	}
}

func TestReadWorkbookNativeFullIOSmoke(t *testing.T) {
	data, err := os.ReadFile("../../output/Full_IO.xlsx")
	if os.IsNotExist(err) {
		t.Skip("native development example not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	sheets, err := ReadWorkbook(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) == 0 || len(sheets[0].Rows) < 2 {
		t.Fatal("missing native rows")
	}
	var row2 *Row
	for i := range sheets[0].Rows {
		if sheets[0].Rows[i].Number == 2 {
			row2 = &sheets[0].Rows[i]
			break
		}
	}
	if row2 == nil {
		t.Fatal("native row 2 missing")
	}
	if row2.Cells["F"] != "" || row2.Cells["G"] != "" {
		t.Fatalf("blank styled F2/G2 became shared strings: F=%q G=%q", row2.Cells["F"], row2.Cells["G"])
	}
	t.Logf("native workbook: %d sheet(s), first sheet %q, %d rows", len(sheets), sheets[0].Name, len(sheets[0].Rows))
}
