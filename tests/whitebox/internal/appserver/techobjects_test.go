// Technology-object XLS HTTP integration, selected controller output and atomic file rollback.
package appserver

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"scheme-xml-generator/internal/techobjects"
)

// techObjectsUnicode encodes expected workbook text as UTF-16LE bytes for independent checks of generated native
// XLS files.
func techObjectsUnicode(value string) []byte {
	units := utf16.Encode([]rune(value))
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[2*i:], unit)
	}
	return data
}

// TestTechObjectsPreviewAndRenamedBatchWithoutXMLState previews module inventory and generates renamed PLC XLS
// files without a library or XML allocator; checks native bytes and downloads.
func TestTechObjectsPreviewAndRenamedBatchWithoutXMLState(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	// The XLS feature must work without a library or an XML ID allocator.
	application.allocator = nil
	application.repository = nil
	data := plcDiagnosticWorkbook(t)
	preview := httptest.NewRecorder()
	application.Handler().ServeHTTP(preview, plcDiagnosticMultipart(t, "/api/techobjects/preview", "IO.xlsx", data, nil))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview %d: %s", preview.Code, preview.Body.String())
	}
	var inventory struct {
		Controllers []techobjects.ControllerPreview `json:"controllers"`
		Warnings    []string                        `json:"warnings"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Controllers) != 2 || inventory.Controllers[0].ModuleCount != 2 || inventory.Controllers[0].ReserveCount != 18 || inventory.Controllers[0].ObjectCount != 20 || inventory.Controllers[0].Types.AI != 1 || inventory.Controllers[0].Types.AO != 1 {
		t.Fatalf("wrong inventory %+v", inventory)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
	options := `{"controllers":[{"key":"FCS1:3000_D_SC_B01"},{"key":"FCS8:3000_D_SC_B07","name":"3000_D_SC_TEST"}],"resourceNumber":2}`
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/techobjects/generate", "IO.xlsx", data, map[string]string{"objects": options, "fileName": "objects.xls"}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate %d: %s", response.Code, response.Body.String())
	}
	var batch techObjectsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 2 || batch.Summary.ModuleCount != 4 || batch.Summary.ReserveCount != 36 || batch.Summary.ObjectCount != 40 {
		t.Fatalf("wrong batch %+v", batch)
	}
	if batch.Files[0].ControllerName != "3000_D_SC_B01" || batch.Files[1].ControllerName != "3000_D_SC_TEST" {
		t.Fatalf("selected names not preserved: %+v", batch.Files)
	}
	for _, file := range batch.Files {
		if file.FileName != "objects_"+file.ControllerName+".xls" || file.Summary.ModuleCount != 2 || file.Summary.ObjectCount != 20 {
			t.Fatalf("wrong file %+v", file)
		}
		content, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(content, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) || !bytes.Contains(content, techObjectsUnicode(file.ControllerName)) {
			t.Fatalf("not a native XLS for %s", file.ControllerName)
		}
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
		if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/vnd.ms-excel" || !bytes.Equal(download.Body.Bytes(), content) {
			t.Fatalf("invalid XLS download: %d %s", download.Code, download.Header().Get("Content-Type"))
		}
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("XLS generation touched XML state: %v", err)
	}
}

// TestTechObjectsSubsetDefaultsAndExistingFilesPreserved generates a selected XLS subset using default names,
// preserving existing output and restricting listing/download extensions.
func TestTechObjectsSubsetDefaultsAndExistingFilesPreserved(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := map[string][]byte{"TechObjects_3000_D_SC_B07_2.xls": []byte("existing XLS"), "existing.xml": []byte("<existing/>"), "hidden.txt": []byte("private")}
	for name, content := range existing {
		if err := os.WriteFile(filepath.Join(outputDir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	options := `{"controllers":[{"key":"FCS8:3000_D_SC_B07"}]}`
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/techobjects/generate", "IO.xlsx", plcDiagnosticWorkbook(t), map[string]string{"objects": options}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate %d: %s", response.Code, response.Body.String())
	}
	var batch techObjectsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 1 || batch.Files[0].FileName != "TechObjects_3000_D_SC_B07_2-2.xls" || batch.Files[0].ControllerName != "3000_D_SC_B07_2" {
		t.Fatalf("selection/default name/collision: %+v", batch)
	}
	for name, want := range existing {
		got, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("changed existing %s: %v", name, err)
		}
	}
	listing := httptest.NewRecorder()
	application.Handler().ServeHTTP(listing, httptest.NewRequest(http.MethodGet, "/api/outputs", nil))
	var listed struct {
		Files []outputFile `json:"files"`
	}
	if err := json.Unmarshal(listing.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listing.Code != http.StatusOK || len(listed.Files) != 3 {
		t.Fatalf("listing must contain XML and XLS only: %d %s", listing.Code, listing.Body.String())
	}
	for _, name := range []string{"hidden.txt", "fake.xlsx", "fake.xls.exe", "fake.html"} {
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/api/output/"+name, nil))
		if download.Code != http.StatusBadRequest {
			t.Fatalf("extension accepted %s: %d", name, download.Code)
		}
	}
	xml := httptest.NewRecorder()
	application.Handler().ServeHTTP(xml, httptest.NewRequest(http.MethodGet, "/api/output/existing.xml", nil))
	if xml.Code != http.StatusOK || xml.Header().Get("Content-Type") != "application/xml; charset=utf-8" || xml.Body.String() != "<existing/>" {
		t.Fatal("existing XML download changed")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("XLS generation touched state: %v", err)
	}
}

// TestTechObjectsRejectInvalidInputsBeforeOutput rejects malformed controller choices and workbook uploads before
// XLS files or XML state are written.
func TestTechObjectsRejectInvalidInputsBeforeOutput(t *testing.T) {
	workbook := plcDiagnosticWorkbook(t)
	valid := `{"controllers":[{"key":"FCS1:3000_D_SC_B01"}]}`
	for _, raw := range []string{
		"", "null", "[]", "{}", `{"controllers":[]}`, `{"controllers":null}`,
		`{"controllers":[null]}`, `{"controllers":[{"key":null}]}`,
		`{"controllers":[{"key":"FCS1:3000_D_SC_B01","name":null}]}`,
		`{"controllers":[{"key":"FCS1:3000_D_SC_B01","unexpected":true}]}`,
		`{"controllers":[{"key":"missing"}]}`,
		`{"controllers":[{"key":"FCS1:3000_D_SC_B01"},{"key":"FCS1:3000_D_SC_B01"}]}`,
		strings.TrimSuffix(valid, "}") + `,"resourceNumber":null}`,
		strings.TrimSuffix(valid, "}") + `,"resourceNumber":0}`,
		strings.TrimSuffix(valid, "}") + `,"resourceNumber":"1"}`,
		strings.TrimSuffix(valid, "}") + `,"extra":1}`,
		valid + " {}", valid + " trailing",
	} {
		t.Run(raw, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/techobjects/generate", "IO.xlsx", workbook, map[string]string{"objects": raw}))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("accepted invalid options %q: %d %s", raw, response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
	for _, test := range []struct {
		name   string
		data   []byte
		fields map[string]string
	}{
		{"IO.xls", workbook, map[string]string{"objects": valid}},
		{"IO.xlsx", []byte("invalid workbook"), map[string]string{"objects": valid}},
		{"IO.xlsx", workbook, nil},
		{"IO.xlsx", workbook, map[string]string{"objects": valid, "context": `{}`}},
	} {
		application, statePath, outputDir := aoTestApplication(t)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/techobjects/generate", test.name, test.data, test.fields))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid upload accepted: %d %s", response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
}

// TestTechObjectsBatchRollsBackOnlyNewFiles forces a later XLS write failure and checks rollback removes new batch
// files while preserving pre-existing output.
func TestTechObjectsBatchRollsBackOnlyNewFiles(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(outputDir, "result.xls")
	if err := os.WriteFile(existingPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	application.output.WriteTechObjectsBatch(response, []techObjectsBatchItem{
		{ControllerName: "ONE", FileName: "result.xls", Data: []byte("new")},
		// The first existing file is deliberately used as a parent directory to
		// force the second write to fail consistently on Windows and Unix.
		{ControllerName: "TWO", FileName: filepath.Join("result.xls", "child.xls"), Data: []byte("new")},
	}, nil)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("batch failure status %d: %s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "result.xls" {
		t.Fatalf("partial batch was not rolled back: %v %v", entries, err)
	}
	data, err := os.ReadFile(existingPath)
	if err != nil || string(data) != "original" {
		t.Fatalf("rollback changed existing file: %q %v", data, err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("failed XLS batch touched state: %v", err)
	}
}
