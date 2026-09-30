// AO table-preview and retired fixed-FBD HTTP contracts; shared multipart fixtures use isolated storage.
package appserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/allocation"
	programcontext "scheme-xml-generator/internal/generator/program"
	"scheme-xml-generator/internal/library"
	"strings"
	"testing"
	"testing/fstest"
)

const smallAOMap = "FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта\n" +
	"FCS_MAIN\tCAB\tA11-00\t0\t_IO_QU*A11-00*_0.ValueDINT := REAL_TO_DINT(_TEST_AO.OUT, 0.0, 100.0);\tA11-00\t\tAO\tAN_v1\n"

// aoTestApplication creates an isolated HTTP server without library files for AO preview, ST/HMI and retired-route
// checks; returns its state/output paths.
func aoTestApplication(t *testing.T) (*Server, string, string) {
	t.Helper()
	temp := isolatedHTTPTemp(t)
	statePath := filepath.Join(temp, "data", "state.json")
	outputDir := filepath.Join(temp, "output")
	allocator, err := allocation.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	// No repository refresh and no library files: the native profile is standalone.
	application := New(library.NewRepository(filepath.Join(temp, "absent-libraries")), config.Default(), allocator, outputDir, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	return application, statePath, outputDir
}

// aoMultipartRequest wraps the supplied AO table and options in a local-origin multipart POST for the selected HTTP
// route.
func aoMultipartRequest(t *testing.T, path string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "AO_map.txt")
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

// assertNoAOOutputOrState checks that preview or a rejected HTTP request created neither allocator state nor
// generated files.
func assertNoAOOutputOrState(t *testing.T, statePath, outputDir string) {
	t.Helper()
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("preview/invalid generation changed state: %v", err)
	}
	if entries, err := os.ReadDir(outputDir); err == nil && len(entries) != 0 {
		t.Fatalf("preview/invalid generation created output: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// TestAOPreviewAndRetiredFBDWithoutLibraries checks AO defaults and table preview without a library, then
// requires 410 from the retired fixed-FBD route.
func TestAOPreviewAndRetiredFBDWithoutLibraries(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	profile := httptest.NewRecorder()
	application.Handler().ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/temporary/ao/profile", nil))
	if profile.Code != http.StatusOK {
		t.Fatalf("profile status=%d: %s", profile.Code, profile.Body.String())
	}
	var profileBody struct {
		Context     programcontext.ProgramContext `json:"context"`
		Description string                        `json:"description"`
	}
	if err := json.Unmarshal(profile.Body.Bytes(), &profileBody); err != nil {
		t.Fatal(err)
	}
	if profileBody.Context != addressing.DefaultAOContext() || profileBody.Description == "" {
		t.Fatalf("bad profile: %+v", profileBody)
	}

	preview := httptest.NewRecorder()
	application.Handler().ServeHTTP(preview, aoMultipartRequest(t, "/api/temporary/ao/preview", []byte(smallAOMap), nil))
	var plan aomap.Plan
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &plan) != nil {
		t.Fatalf("preview status=%d: %s", preview.Code, preview.Body.String())
	}
	if plan.GroupCount != 1 || plan.ModuleCount != 1 || plan.ChannelCount != 4 || plan.ReserveCount != 3 || plan.Groups[0].POUName != "AO_A11" {
		t.Fatalf("bad preview plan: %+v", plan)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), nil))
	assertLegacyFBDUnavailable(t, response)
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// TestAOActualMapPreviewAndRetiredFBD checks complete AO-map preview counts from the optional local
// fixture; fixed-FBD generation must return 410 without output.
func TestAOActualMapPreviewAndRetiredFBD(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "XML dev", "AO_excell_import_scheme.txt"))
	if os.IsNotExist(err) {
		t.Skip("local AO map fixture is absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	application, statePath, outputDir := aoTestApplication(t)
	preview := httptest.NewRecorder()
	application.Handler().ServeHTTP(preview, aoMultipartRequest(t, "/api/temporary/ao/preview", data, nil))
	var plan aomap.Plan
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &plan) != nil {
		t.Fatalf("preview status=%d: %s", preview.Code, preview.Body.String())
	}
	if plan.GroupCount != 21 || plan.ModuleCount != 128 || plan.RowCount != 410 || plan.ChannelCount != 512 || plan.ReserveCount != 102 || plan.UniqueTagCount != 207 || plan.DuplicateCount != 203 {
		t.Fatalf("fixture counts: groups=%d modules=%d rows=%d channels=%d reserves=%d unique=%d duplicates=%d", plan.GroupCount, plan.ModuleCount, plan.RowCount, plan.ChannelCount, plan.ReserveCount, plan.UniqueTagCount, plan.DuplicateCount)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", data, nil))
	assertLegacyFBDUnavailable(t, response)
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// assertFreshAOID records an XML transport identifier and rejects empty or repeated IDs across the generated
// controller batch.
func assertFreshAOID(t *testing.T, ids map[string]bool, id, kind string) {
	t.Helper()
	if id == "" || ids[id] {
		t.Fatalf("empty or repeated %s %q across generated files", kind, id)
	}
	ids[id] = true
}

// TestRetiredAOFBDRejectsRepeatedControllerNames проверяет запрет встроенного FBD даже для разных ПЛК.
// Повторные запросы с одинаковыми именами не меняют allocator и не создают старые AO-файлы.
func TestRetiredAOFBDRejectsRepeatedControllerNames(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	other := strings.SplitN(strings.ReplaceAll(smallAOMap, "FCS_MAIN", "FCS_OTHER"), "\n", 2)[1]
	for run := 0; run < 2; run++ {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap+other), map[string]string{"fileName": "shared.xml"}))
		assertLegacyFBDUnavailable(t, response)
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
}

// TestRetiredAOFBDRejectsDuplicateRows проверяет границу HTTP для AO-карты с дубликатами.
// Проверяются только 410 и отсутствие побочных действий; старые fixed-FBD утверждения исключены из исполняемой проверки.
func TestRetiredAOFBDRejectsDuplicateRows(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	duplicate := strings.SplitN(smallAOMap, "\n", 2)[1]
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap+duplicate), nil))
	assertLegacyFBDUnavailable(t, response)
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// TestRetiredAOFBDRejectsMultiControllerBeforeWrites submits a multi-controller AO table to the
// retired FBD endpoint; rejection must precede all file writes and allocation.
func TestRetiredAOFBDRejectsMultiControllerBeforeWrites(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	other := strings.SplitN(strings.ReplaceAll(smallAOMap, "FCS_MAIN", "FCS_OTHER"), "\n", 2)[1]
	other = strings.ReplaceAll(other, "0.0, 100.0", "0.0, 200.0")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap+other), nil))
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d want410: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// TestRetiredAOFBDPayloadsDoNotConsumeIDs checks 410 for retired AO-FBD payload variants and 403 for a foreign origin,
// with unchanged output and allocator state.
func TestRetiredAOFBDPayloadsDoNotConsumeIDs(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	tests := []struct {
		name   string
		data   []byte
		fields map[string]string
		origin string
		want   int
	}{
		{"empty file", nil, nil, "", http.StatusGone},
		{"invalid TXT", []byte("not a map"), nil, "", http.StatusGone},
		{"unknown context", []byte(smallAOMap), map[string]string{"context": `{"surprise":"value"}`}, "", http.StatusGone},
		{"null context", []byte(smallAOMap), map[string]string{"context": `null`}, "", http.StatusGone},
		{"null context field", []byte(smallAOMap), map[string]string{"context": `{"controllerId":null}`}, "", http.StatusGone},
		{"trailing JSON", []byte(smallAOMap), map[string]string{"context": `{} {}`}, "", http.StatusGone},
		{"numeric context", []byte(smallAOMap), map[string]string{"context": `{"controllerId":123}`}, "", http.StatusGone},
		{"out of range context", []byte(smallAOMap), map[string]string{"context": `{"controllerId":"2147483648"}`}, "", http.StatusGone},
		{"unknown form field", []byte(smallAOMap), map[string]string{"surprise": "value"}, "", http.StatusGone},
		{"oversize file", bytes.Repeat([]byte("x"), int(maxAOMappingFileBytes)+1), nil, "", http.StatusGone},
		{"oversize body", bytes.Repeat([]byte("x"), int(maxAOMappingFileBytes+maxAOMultipartExtra)+1), nil, "", http.StatusGone},
		{"remote origin", []byte(smallAOMap), nil, "https://example.com", http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := aoMultipartRequest(t, "/api/temporary/ao/generate", test.data, test.fields)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d: %s", response.Code, test.want, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
			if test.name == "oversize file" && request.MultipartForm != nil {
				for _, headers := range request.MultipartForm.File {
					for _, header := range headers {
						file, err := header.Open()
						if err == nil {
							file.Close()
							t.Error("multipart temporary file was not removed")
						}
					}
				}
			}
		})
	}
}

// TestRetiredAOFBDDoesNotReachAllocatorPersistence makes allocator persistence unwritable, then proves the retired
// AO-FBD route returns 410 before reaching that storage.
func TestRetiredAOFBDDoesNotReachAllocatorPersistence(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	if err := os.MkdirAll(statePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), nil))
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d want410: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// TestAOMultipartRequiresOneFile sends zero and two uploads to AO preview; the HTTP parser must reject both without
// creating output.
func TestAOMultipartRequiresOneFile(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("files%d", count), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			for i := 0; i < count; i++ {
				part, err := writer.CreateFormFile("file", fmt.Sprintf("map%d.txt", i))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(part, smallAOMap); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/temporary/ao/preview", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}
