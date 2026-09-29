// PLC HMI HTTP integration and independent XML-reference checks across the generated controller batch.
package appserver

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/iomap"
)

// plcDiagnosticMultipart constructs a named workbook upload and options for the PLC-diagnostic or assignment HTTP
// handler.
func plcDiagnosticMultipart(t *testing.T, path, name string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://127.0.0.1:3210")
	return request
}

// Tiny in-memory OOXML fixture: no application or development files are changed.
func plcDiagnosticWorkbook(t *testing.T) []byte {
	t.Helper()
	rows := [][]string{
		{"FCS", "Control cabinet", "ModuleType", "Main module", "Redundand module", "Channel", "Loop", "Tag No", "SCADA Tag"},
		{"FCS1", "3000_D_SC_B01", "AI16H", "A10-02", "", "0", "3000-PI-001", "3000-PT-001", "_TEST_B01_AI"},
		{"FCS1", "3000_D_SC_B01", "AOC4H", "A11-00", "", "0", "3000-TI-001", "3000-TY-001", "_TEST_B01_AO"},
		{"FCS8", "3000_D_SC_B07", "AI16H", "A70-02", "", "0", "3000-PI-002", "3000-PT-002", "_TEST_B07_AI"},
		{"FCS8", "3000_D_SC_B07", "AOC4H", "A71-00", "", "0", "3000-TI-002", "3000-TY-002", "_TEST_B07_AO"},
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, value := range row {
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>`, rune('A'+j), i+1)
			if err := xml.EscapeText(&sheet, []byte(value)); err != nil {
				t.Fatal(err)
			}
			sheet.WriteString(`</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts := map[string]string{
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="IO" sheetId="1" r:id="r1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{"_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml"} {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

type plcAPINode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr   `xml:",any,attr"`
	Text     string       `xml:",chardata"`
	Children []plcAPINode `xml:",any"`
}

// attr returns one XML attribute from the independent HMI test tree for reference validation.
func (n plcAPINode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// child returns a direct XML child from the HMI test tree, or an empty node when it is absent.
func (n plcAPINode) child(name string) plcAPINode {
	for _, c := range n.Children {
		if c.XMLName.Local == name {
			return c
		}
	}
	return plcAPINode{}
}

// all collects descendants with the requested XML name for independent HMI hierarchy and link checks.
func (n plcAPINode) all(name string) []plcAPINode {
	var result []plcAPINode
	var visit func(plcAPINode)
	visit = func(p plcAPINode) {
		if p.XMLName.Local == name {
			result = append(result, p)
		}
		for _, c := range p.Children {
			visit(c)
		}
	}
	visit(n)
	return result
}

// assertPLCAPIReferences walks generated PLC HMI XML independently of renderer structs, validating controller
// bindings, unique IDs and all internal references.
func assertPLCAPIReferences(t *testing.T, data []byte, controllerName, resource string, globalPages, globalOwners, globalCards map[string]bool) (int, int) {
	t.Helper()
	var doc plcAPINode
	// A generic tree makes this check independent of the generator's Go types.
	if !utf8.Valid(data) {
		t.Fatal("invalid UTF-8")
	}
	if err := xml.Unmarshal(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.XMLName.Local != "BufScada" || len(doc.child("Pages").Children) != 1 || len(doc.all("POUS")) != 0 {
		t.Fatal("not a single native PLC group")
	}
	root := doc.child("Pages").Children[0]
	if root.child("NAME").Text != controllerName || len(root.child("SUBPAGES").Children) != 2 {
		t.Fatal("missing PLC/panel nesting")
	}
	groups := map[string]string{}
	for _, group := range doc.child("GRPAGESINFO").Children {
		groups[group.attr("ID")] = group.attr("FullName")
	}
	cards := map[string]string{}
	for _, card := range doc.child("CARDSINFO").Children {
		id, info := card.attr("ID"), card.attr("CardInfo")
		assertFreshAOID(t, globalCards, id, "PLC card")
		cards[id] = info
		if !strings.HasPrefix(info, "2/"+controllerName+"/"+resource+"/") && info != "7///"+controllerName+"/(TENIX-CPU715)" {
			t.Fatalf("foreign PLC binding %s", info)
		}
	}
	params := map[string]bool{}
	for _, param := range doc.child("CARDPARAMSINFO").Children {
		id := param.attr("ID")
		assertFreshAOID(t, globalOwners, id, "parameter ID")
		params[id] = true
		if !strings.HasPrefix(param.attr("Info"), "2/"+controllerName+"/"+resource+"/") {
			t.Fatal("foreign ack PLC")
		}
	}
	symbols, pics := map[string]bool{}, map[string]bool{}
	for _, r := range doc.child("PAGEMSINFO").Children {
		symbols[r.attr("ID")] = true
	}
	for _, r := range doc.child("PICS").Children {
		pics[r.attr("ID")] = true
		if len(r.Text) < 10 {
			t.Fatal("empty embedded picture")
		}
	}
	pages := map[string]bool{}
	var pagePaths func(plcAPINode, string)
	pagePaths = func(page plcAPINode, parent string) {
		id := page.attr("ID")
		assertFreshAOID(t, globalPages, id, "PLC page")
		pages[id] = true
		path := parent + "\\" + page.child("NAME").Text
		if page.child("ID").Text != id || groups[id] != path {
			t.Fatal("page/group path mismatch")
		}
		for _, child := range page.child("SUBPAGES").Children {
			pagePaths(child, path)
		}
	}
	pagePaths(root, "Диагностика")
	for _, primitive := range doc.all("OnePrim") {
		assertFreshAOID(t, globalOwners, primitive.attr("SourceT11ID"), "primitive ID")
		if primitive.attr("OBJTYPE") == "8" {
			card := primitive.child("CardID").Text
			if !symbols[primitive.child("ObjMSID").Text] || card != "0" && cards[card] == "" {
				t.Fatal("dangling symbol/card")
			}
		}
		if primitive.attr("OBJTYPE") == "5" && !pics[primitive.attr("PicId")] {
			t.Fatal("dangling image")
		}
	}
	charts := 0
	for _, receptor := range doc.all("OneReceptor") {
		id := receptor.attr("FROMID")
		assertFreshAOID(t, globalOwners, id, "receptor ID")
		if receptor.attr("TYPEID") == "1" && !pages[receptor.attr("PARAM_INT")] {
			t.Fatal("dangling internal page receptor")
		}
		for _, chart := range receptor.all("OneChart") {
			chartID := chart.attr("ID")
			assertFreshAOID(t, globalOwners, chartID, "chart ID")
			if chart.child("ID").Text != chartID || chart.child("PID").Text != id || !params[chart.child("PARAMID").Text] {
				t.Fatal("dangling chart/parent/parameter")
			}
			charts++
		}
	}
	if charts != len(params) {
		t.Fatal("ack commands do not cover parameters")
	}
	return len(pages), len(doc.all("OnePrim"))
}

// TestPLCDiagnosticAPIPreviewAndRenamedControllerBatch previews inventory then generates renamed PLC HMI files,
// checking saved/downloaded bytes and retained physical tags.
func TestPLCDiagnosticAPIPreviewAndRenamedControllerBatch(t *testing.T) {
	data := plcDiagnosticWorkbook(t)
	application, statePath, outputDir := aoTestApplication(t)
	preview := httptest.NewRecorder()
	application.Handler().ServeHTTP(preview, plcDiagnosticMultipart(t, "/api/temporary/diagnostic/preview", "IO.xlsx", data, nil))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview %d: %s", preview.Code, preview.Body.String())
	}
	var plan iomap.Plan
	if err := json.Unmarshal(preview.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Controllers) != 2 || plan.ModuleCount != 4 || plan.SignalCount != 40 {
		t.Fatalf("wrong plan %+v", plan)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
	selected := []iomap.Selection{{Key: plan.Controllers[0].Key}, {Key: plan.Controllers[1].Key, Name: "3000_D_SC_TEST"}}
	payload, _ := json.Marshal(struct {
		Controllers []iomap.Selection `json:"controllers"`
	}{selected})
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/temporary/diagnostic/generate", "IO.xlsx", data, map[string]string{"fileName": "plc_batch", "diagnostic": string(payload), "context": `{"project":"PLC test","resourceNumber":"2"}`}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate %d: %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Kind != "diagnostic" || len(batch.Files) != 2 || batch.Summary.FrameCount != 10 || batch.Summary.SignalCount != 40 || batch.Summary.POUCount != 0 {
		t.Fatalf("wrong batch %+v", batch.Summary)
	}
	pages, owners, cards := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, file := range batch.Files {
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
		if download.Code != http.StatusOK {
			t.Fatal("failed download")
		}
		disk, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
		if err != nil || !bytes.Equal(disk, download.Body.Bytes()) {
			t.Fatal("disk/download mismatch")
		}
		frameCount, graphics := assertPLCAPIReferences(t, disk, file.ControllerName, "2", pages, owners, cards)
		if frameCount != file.Summary.FrameCount || graphics != file.Summary.Graphics {
			t.Fatal("summary/actual tree mismatch")
		}
		if file.ControllerName == "3000_D_SC_TEST" {
			if bytes.Contains(disk, []byte("3000_D_SC_B07_2")) || !bytes.Contains(disk, []byte("_TEST_B07_AI")) || !bytes.Contains(disk, []byte("_3000_D_SC_TEST_A70_02_1")) {
				t.Fatal("PLC rename changed real tags or left old reserves")
			}
		}
	}
	assertDiagnosticAllocatorState(t, statePath, int64(len(pages)), int64(len(owners)), int64(len(cards)))
}

// TestPLCDiagnosticAPIRejectsBeforeAllocation rejects invalid PLC selections, HMI contexts, uploads and origins
// before any output or ID allocation.
func TestPLCDiagnosticAPIRejectsBeforeAllocation(t *testing.T) {
	data := plcDiagnosticWorkbook(t)
	source, err := iomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	key := source.Controllers[0].Key
	valid := fmt.Sprintf(`{"controllers":[{"key":%q}]}`, key)
	application, statePath, outputDir := aoTestApplication(t)
	duplicateNames := fmt.Sprintf(`{"controllers":[{"key":%q,"name":"PLC_A"},{"key":%q,"name":"plc_a"}]}`, source.Controllers[0].Key, source.Controllers[1].Key)
	unknownAfterValid := fmt.Sprintf(`{"controllers":[{"key":%q},{"key":"unknown"}]}`, key)
	for _, raw := range []string{"", `null`, `{}`, `{"controllers":[]}`, `{"controllers":null}`, `{"controllers":[null]}`, `{"controllers":[{"key":"unknown"}]}`, fmt.Sprintf(`{"controllers":[{"key":%q},{"key":%q}]}`, key, key), fmt.Sprintf(`{"controllers":[{"key":%q,"name":"bad/name"}]}`, key), fmt.Sprintf(`{"controllers":[{"key":%q,"extra":true}]}`, key), valid + ` {}`, `{"controllers":[],"fcs":[]}`, duplicateNames, unknownAfterValid} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/temporary/diagnostic/generate", "IO.xlsx", data, map[string]string{"diagnostic": raw}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("selection %s: %d %s", raw, response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
	for _, ctx := range []string{`{"resourceNumber":"0"}`, `{"resourceNumber":null}`, `{"controllerId":"1"}`, `{"project":"bad\u0000value"}`, `{} {}`} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/temporary/diagnostic/generate", "IO.xlsx", data, map[string]string{"diagnostic": valid, "context": ctx}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("context %s: %d %s", ctx, response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
	for _, path := range []string{"/api/temporary/diagnostic/preview", "/api/temporary/diagnostic/generate"} {
		for _, file := range []struct {
			name string
			data []byte
		}{{"map.txt", []byte(smallAOMap)}, {"broken.xlsx", []byte("not a ZIP workbook")}, {"IO.txt", data}} {
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, path, file.name, file.data, map[string]string{"diagnostic": valid}))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s %s accepted: %d", path, file.name, response.Code)
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		}
		request := plcDiagnosticMultipart(t, path, "IO.xlsx", data, map[string]string{"diagnostic": valid})
		request.Header.Set("Origin", "https://foreign.invalid")
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("foreign origin: %d", response.Code)
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
}

// TestPLCDiagnosticAPIActualFullIOAllControllers generates all PLC HMI files from optional Full_IO.xlsx, validates
// each tree and confirms source workbook/XML samples remain unchanged.
func TestPLCDiagnosticAPIActualFullIOAllControllers(t *testing.T) {
	path := filepath.Join("..", "..", "output", "Full_IO.xlsx")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("local Full_IO fixture missing")
	}
	if err != nil {
		t.Fatal(err)
	}
	nativePath := filepath.Join("..", "..", "output", "3000_D_SC_B01.xml")
	nativeBefore, nativeErr := os.ReadFile(nativePath)
	source, err := iomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var selected []iomap.Selection
	for _, c := range source.Controllers {
		selected = append(selected, iomap.Selection{Key: c.Key})
	}
	prepared, err := generator.PreparePLCDiagnosticPlans(source, selected, generator.DefaultHMIContext())
	if err != nil {
		t.Fatal(err)
	}
	var frames, ids, cards int64
	for _, p := range prepared {
		frames += int64(p.FrameCount)
		ids += int64(p.T11Count)
		cards += int64(p.CardCount)
	}
	payload, _ := json.Marshal(struct {
		Controllers []iomap.Selection `json:"controllers"`
	}{selected})
	application, statePath, outputDir := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/temporary/diagnostic/generate", "Full_IO.xlsx", data, map[string]string{"fileName": "full_plc", "diagnostic": string(payload)}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate all %d: %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 8 || batch.Summary.FrameCount != int(frames) || frames != 344 || batch.Summary.IOModuleCount != 380 || batch.Summary.SignalCount != 5204 {
		t.Fatalf("unexpected full IO summary files=%d %+v (source=%d/%d)", len(batch.Files), batch.Summary, source.ModuleCount, source.SignalCount)
	}
	globalPages, globalOwners, globalCards := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, file := range batch.Files {
		disk, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
		if err != nil {
			t.Fatal(err)
		}
		count, graphics := assertPLCAPIReferences(t, disk, file.ControllerName, "1", globalPages, globalOwners, globalCards)
		if count != file.Summary.FrameCount || graphics != file.Summary.Graphics {
			t.Fatal("summary mismatch")
		}
		if file.ControllerName == "3000_D_SC_B01" && (count != 50 || graphics != 644) {
			t.Fatal("B01 native hierarchy changed")
		}
	}
	assertDiagnosticAllocatorState(t, statePath, frames, ids, cards)
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("source workbook changed")
	}
	if nativeErr == nil {
		after, err := os.ReadFile(nativePath)
		if err != nil || !bytes.Equal(nativeBefore, after) {
			t.Fatal("native XML fixture changed")
		}
	}
}

// TestPLCDiagnosticAPIRejectsOversizedUpload sends a workbook exceeding the parser limit to PLC-HMI preview and
// generation; both must reject it without side effects.
func TestPLCDiagnosticAPIRejectsOversizedUpload(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	data := make([]byte, iomap.MaxWorkbookBytes+1)
	for _, path := range []string{"/api/temporary/diagnostic/preview", "/api/temporary/diagnostic/generate"} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, path, "huge.xlsx", data, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("oversized workbook status=%d", response.Code)
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
}
