// Library-generation HTTP contracts for catalog, document requests, output downloads and limits.
package appserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/allocation"
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	"scheme-xml-generator/internal/library"
	"strings"
	"testing"
	"testing/fstest"
)

// TestAPIListsPreviewsGeneratesAndDownloads exercises the HTTP library catalog, signal preview, generated XML
// metadata and download bytes end to end.
func TestAPIListsPreviewsGeneratesAndDownloads(t *testing.T) {
	libraryDir := isolatedLibraryDirectory(t)
	repository := library.NewRepository(libraryDir)
	if _, err := repository.Refresh(); err != nil {
		t.Fatal(err)
	}
	temp := isolatedHTTPTemp(t)
	allocator, err := allocation.NewAllocator(filepath.Join(temp, "data", "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	var static fs.FS = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	application := New(repository, config.Default(), allocator, filepath.Join(temp, "output"), static, log.New(io.Discard, "", 0))
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/templates")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var catalog library.Catalog
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Templates) != 8 {
		t.Fatalf("templates=%d", len(catalog.Templates))
	}
	key := ""
	for _, item := range catalog.Templates {
		if item.ID == "19963" {
			key = item.Key
		}
	}
	if key == "" {
		t.Fatal("template 19963 missing")
	}

	previewPayload, _ := json.Marshal(map[string]any{"templateKey": key, "objectName": "_1110_LZIA_10101_ADR", "nameMode": "auto"})
	previewResponse := postJSON(t, server.URL+"/api/preview-name", server.URL, previewPayload)
	defer previewResponse.Body.Close()
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d: %s", previewResponse.StatusCode, readBody(previewResponse.Body))
	}
	var preview fbdrequest.NamePreview
	if err := json.NewDecoder(previewResponse.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if preview.BaseName != "_1110_LZIA_10101" {
		t.Fatalf("base=%s", preview.BaseName)
	}

	generatePayload, _ := json.Marshal(map[string]any{"templateKey": key, "objectName": "_1110_LZIA_10101_ADR", "pouName": "ADR_test", "nameMode": "auto", "description": "Тест", "klPath": "", "offsetX": 300, "offsetY": 100, "fileName": "test-output", "pouGroupId": 19022, "pouNumber": 48})
	generateResponse := postJSON(t, server.URL+"/api/generate", server.URL, generatePayload)
	defer generateResponse.Body.Close()
	if generateResponse.StatusCode != http.StatusCreated {
		t.Fatalf("generate status=%d: %s", generateResponse.StatusCode, readBody(generateResponse.Body))
	}
	var generated struct {
		FileName string              `json:"fileName"`
		URL      string              `json:"url"`
		Summary  xmlartifact.Summary `json:"summary"`
	}
	if err := json.NewDecoder(generateResponse.Body).Decode(&generated); err != nil {
		t.Fatal(err)
	}
	if generated.FileName != "test-output.xml" || generated.Summary.POUName != "ADR_test" || generated.Summary.POUGroupID != "19022" || generated.Summary.POUNumber != "48" || generated.Summary.Blocks != 34 || generated.Summary.Links != 27 || generated.Summary.Graphics != 8 {
		t.Fatalf("unexpected generate response: %+v", generated)
	}
	download, err := http.Get(server.URL + generated.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()
	data, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if download.StatusCode != http.StatusOK || !bytes.Contains(data, []byte("<BufScadaPOUS>")) || !bytes.Contains(data, []byte(`NAME="ADR_test"`)) || !bytes.Contains(data, []byte("_1110_LZIA_10101_ADR")) {
		t.Fatalf("bad download status=%d size=%d", download.StatusCode, len(data))
	}
}

// TestAPIGeneratesSeveralSignalsAndPOUsInOneFile generates multiple library signals and independent POUs in one
// file, checking allocation totals, names and saved XML.
func TestAPIGeneratesSeveralSignalsAndPOUsInOneFile(t *testing.T) {
	libraryDir := isolatedLibraryDirectory(t)
	repository := library.NewRepository(libraryDir)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, item := range catalog.Templates {
		if item.ID == "17510" {
			key = item.Key
			break
		}
	}
	if key == "" {
		t.Fatal("template AD3_v2 (17510) missing")
	}

	temp := isolatedHTTPTemp(t)
	allocator, err := allocation.NewAllocator(filepath.Join(temp, "data", "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	var static fs.FS = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	application := New(repository, config.Default(), allocator, filepath.Join(temp, "output"), static, log.New(io.Discard, "", 0))
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	payload, _ := json.Marshal(map[string]any{
		"fileName": "multi-pou-test",
		"pous": []any{
			map[string]any{
				"name": "AD3_POU_A", "description": "Первая POU", "pouId": 220001, "groupId": 19022, "pouNumber": 51,
				"page": map[string]any{"width": 2400, "height": 1800, "dparams": "3", "backgroundColor": "16777215", "marginRight": 250, "marginBottom": 250},
				"signals": []any{
					map[string]any{"templateKey": key, "objectName": "_POU_A_SIGNAL_1", "nameMode": "base"},
					map[string]any{"templateKey": key, "objectName": "_POU_A_SIGNAL_2", "nameMode": "base"},
				},
			},
			map[string]any{
				"name": "AD3_POU_B", "description": "Вторая POU", "pouId": 220002, "groupId": 19023, "pouNumber": 52,
				"page": map[string]any{"width": 2600, "height": 2000, "dparams": "7", "backgroundColor": "15461355", "marginRight": 300, "marginBottom": 320},
				"signals": []any{
					map[string]any{"templateKey": key, "objectName": "_POU_B_SIGNAL_1", "nameMode": "base", "offsetX": 450, "offsetY": 275},
				},
			},
		},
	})
	response := postJSON(t, server.URL+"/api/generate", server.URL, payload)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("generate status=%d: %s", response.StatusCode, readBody(response.Body))
	}
	var generated generateResponse
	if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
		t.Fatal(err)
	}
	if generated.FileName != "multi-pou-test.xml" || generated.Summary.POUCount != 2 || generated.Summary.SignalCount != 3 {
		t.Fatalf("unexpected document summary: %+v", generated)
	}
	if generated.Summary.Blocks != 21 || generated.Summary.Graphics != 6 || generated.Summary.Cards != 9 || len(generated.Summary.POUs) != 2 {
		t.Fatalf("unexpected generated counts: %+v", generated.Summary)
	}
	if generated.Summary.POUs[0].POUName != "AD3_POU_A" || generated.Summary.POUs[1].POUName != "AD3_POU_B" || generated.Summary.POUs[1].Signals[0].OffsetX != 450 || generated.Summary.POUs[1].Signals[0].OffsetY != 275 {
		t.Fatalf("POU settings were not kept independent: %+v", generated.Summary.POUs)
	}

	download, err := http.Get(server.URL + generated.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer download.Body.Close()
	data, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if download.StatusCode != http.StatusOK || bytes.Count(data, []byte("<OnePOU ")) != 2 {
		t.Fatalf("download does not contain two POU: status=%d size=%d", download.StatusCode, len(data))
	}
	for _, fragment := range [][]byte{[]byte(`NAME="AD3_POU_A"`), []byte(`NAME="AD3_POU_B"`), []byte("_POU_A_SIGNAL_1"), []byte("_POU_A_SIGNAL_2"), []byte("_POU_B_SIGNAL_1")} {
		if !bytes.Contains(data, fragment) {
			t.Fatalf("download misses %q", fragment)
		}
	}
}

// TestAPIDocumentRejectsRetiredIOBeforePhysicalValidation проверяет приоритет запрета старого формата.
// Некорректные типы, адреса и смешанные сигналы дают 410 до нормализации и разрешения библиотеки.
func TestAPIDocumentRejectsRetiredIOBeforePhysicalValidation(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := templateKeyByID(t, catalog, "17510")
	server := newDocumentTestServer(t, repository)
	defer server.Close()
	validSignal := func(index int) map[string]any {
		return map[string]any{"objectName": fmt.Sprintf("_AI_%d", index), "nameMode": "base"}
	}
	seventeenSignals := make([]any, 17)
	for index := range seventeenSignals {
		seventeenSignals[index] = validSignal(index)
	}

	tests := []struct {
		name string
		pous []any
	}{
		{
			name: "missing effective template",
			pous: []any{map[string]any{
				"name": "NO_TEMPLATE",
				"io":   map[string]any{"type": "AI", "modules": []any{map[string]any{"signals": []any{validSignal(0)}}}},
			}},
		},
		{
			name: "unknown IO type",
			pous: []any{map[string]any{
				"name": "BAD_TYPE", "defaultTemplateKey": key,
				"io": map[string]any{"type": "XX", "modules": []any{map[string]any{"signals": []any{validSignal(0)}}}},
			}},
		},
		{
			name: "module capacity overflow",
			pous: []any{map[string]any{
				"name": "BAD_CAPACITY", "defaultTemplateKey": key,
				"io": map[string]any{"type": "AI", "modules": []any{map[string]any{"signals": seventeenSignals}}},
			}},
		},
		{
			name: "duplicate module ID",
			pous: []any{map[string]any{
				"name": "DUPLICATE_ID", "defaultTemplateKey": key,
				"io": map[string]any{"type": "AI", "modules": []any{
					map[string]any{"id": 0, "signals": []any{validSignal(0)}},
					map[string]any{"id": 0, "signals": []any{validSignal(1)}},
				}},
			}},
		},
		{
			name: "negative module ID",
			pous: []any{map[string]any{
				"name": "NEGATIVE_ID", "defaultTemplateKey": key,
				"io": map[string]any{"type": "AI", "modules": []any{map[string]any{"id": -1, "signals": []any{validSignal(0)}}}},
			}},
		},
		{
			name: "module ID outside signed 32 bit",
			pous: []any{map[string]any{
				"name": "LARGE_ID", "defaultTemplateKey": key,
				"io": map[string]any{"type": "AI", "modules": []any{map[string]any{"id": int64(2147483648), "signals": []any{validSignal(0)}}}},
			}},
		},
		{
			name: "legacy and physical signals mixed",
			pous: []any{map[string]any{
				"name": "MIXED", "defaultTemplateKey": key,
				"signals": []any{map[string]any{"templateKey": key, "objectName": "_LEGACY", "nameMode": "base"}},
				"io":      map[string]any{"type": "AI", "modules": []any{map[string]any{"signals": []any{validSignal(0)}}}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"pous": test.pous})
			response := postJSON(t, server.URL+"/api/generate", server.URL, payload)
			defer response.Body.Close()
			if response.StatusCode != http.StatusGone {
				t.Fatalf("status=%d want 410: %s", response.StatusCode, readBody(response.Body))
			}
		})
	}
}

// TestAPIDocumentLegacySignalsRemainSupported checks the earlier flat library-signal wire shape still inherits its
// template and yields the expected document summary.
func TestAPIDocumentLegacySignalsRemainSupported(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := templateKeyByID(t, catalog, "17510")
	server := newDocumentTestServer(t, repository)
	defer server.Close()
	payload, _ := json.Marshal(map[string]any{
		"fileName": "legacy-document-contract",
		"pous": []any{map[string]any{
			"name": "LEGACY_DOCUMENT_POU", "defaultTemplateKey": key,
			"signals": []any{
				map[string]any{"templateKey": key, "objectName": "_LEGACY_DOCUMENT_SIGNAL", "nameMode": "base"},
				map[string]any{"objectName": "_FLAT_DEFAULT_SIGNAL", "nameMode": "base"},
			},
		}},
	})
	response := postJSON(t, server.URL+"/api/generate", server.URL, payload)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("legacy document status=%d: %s", response.StatusCode, readBody(response.Body))
	}
	var generated generateResponse
	if err := json.NewDecoder(response.Body).Decode(&generated); err != nil {
		t.Fatal(err)
	}
	if generated.Summary.POUCount != 1 || generated.Summary.SignalCount != 2 || generated.Summary.IOModuleCount != 0 {
		t.Fatalf("legacy document summary changed: %+v", generated.Summary)
	}
	if len(generated.Summary.POUs) != 1 || len(generated.Summary.POUs[0].Signals) != 2 || generated.Summary.POUs[0].Signals[1].TemplateKey != key {
		t.Fatalf("flat default template was not inherited: %+v", generated.Summary.POUs)
	}
}

// TestAPIDocumentRejectsLegacyFieldsAtDocumentLevel rejects mixed flat/document request shapes and empty POU lists
// before generation.
func TestAPIDocumentRejectsLegacyFieldsAtDocumentLevel(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.Templates[0].Key
	temp := isolatedHTTPTemp(t)
	allocator, err := allocation.NewAllocator(filepath.Join(temp, "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	application := New(repository, config.Default(), allocator, filepath.Join(temp, "output"), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	payload, _ := json.Marshal(map[string]any{
		"objectName": "wrong-level",
		"pous":       []any{map[string]any{"name": "POU_A", "signals": []any{map[string]any{"templateKey": key, "objectName": "RIGHT", "nameMode": "base"}}}},
	})
	response := postJSON(t, server.URL+"/api/generate", server.URL, payload)
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400: %s", response.StatusCode, readBody(response.Body))
	}
	emptyPayload, _ := json.Marshal(map[string]any{"fileName": "empty", "pous": []any{}})
	emptyResponse := postJSON(t, server.URL+"/api/generate", server.URL, emptyPayload)
	defer emptyResponse.Body.Close()
	if emptyResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty pous status=%d want 400: %s", emptyResponse.StatusCode, readBody(emptyResponse.Body))
	}
}

// TestAPIDocumentRejectsRequestLimitsBeforeGeneration checks HTTP POU/signal/object budgets and the retired
// IO-format guard before generator allocation.
func TestAPIDocumentRejectsRequestLimitsBeforeGeneration(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.Templates[0].Key
	temp := isolatedHTTPTemp(t)
	allocator, err := allocation.NewAllocator(filepath.Join(temp, "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	application := New(repository, config.Default(), allocator, filepath.Join(temp, "output"), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	tooManyPOUs := make([]any, maxDocumentPOUs+1)
	for index := range tooManyPOUs {
		tooManyPOUs[index] = map[string]any{}
	}
	pouPayload, _ := json.Marshal(map[string]any{"pous": tooManyPOUs})
	pouResponse := postJSON(t, server.URL+"/api/generate", server.URL, pouPayload)
	if pouResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("too many POU status=%d want 400: %s", pouResponse.StatusCode, readBody(pouResponse.Body))
	}
	pouResponse.Body.Close()

	tooManySignals := make([]any, maxDocumentSignals+1)
	for index := range tooManySignals {
		tooManySignals[index] = map[string]any{"templateKey": key}
	}
	signalPayload, _ := json.Marshal(map[string]any{"pous": []any{map[string]any{"name": "LIMIT_POU", "signals": tooManySignals}}})
	signalResponse := postJSON(t, server.URL+"/api/generate", server.URL, signalPayload)
	if signalResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("too many signals status=%d want 400: %s", signalResponse.StatusCode, readBody(signalResponse.Body))
	}
	signalResponse.Body.Close()

	tooManyModules := make([]any, maxDocumentModules+1)
	for index := range tooManyModules {
		tooManyModules[index] = map[string]any{
			"signals": []any{map[string]any{"objectName": fmt.Sprintf("_MODULE_SIGNAL_%d", index), "nameMode": "base"}},
		}
	}
	modulePayload, _ := json.Marshal(map[string]any{"pous": []any{map[string]any{
		"name": "MODULE_LIMIT_POU", "defaultTemplateKey": key,
		"io": map[string]any{"type": "AI", "modules": tooManyModules},
	}}})
	moduleResponse := postJSON(t, server.URL+"/api/generate", server.URL, modulePayload)
	moduleBody := readBody(moduleResponse.Body)
	moduleResponse.Body.Close()
	if moduleResponse.StatusCode != http.StatusGone || !strings.Contains(moduleBody, "io.modules") {
		t.Fatalf("retired module format status=%d want 410: %s", moduleResponse.StatusCode, moduleBody)
	}

	largest := catalog.Templates[0]
	for _, template := range catalog.Templates[1:] {
		if template.PrimitiveCount > largest.PrimitiveCount {
			largest = template
		}
	}
	objectSignalCount := maxDocumentObjects/largest.PrimitiveCount + 1
	if objectSignalCount > maxDocumentSignals {
		t.Fatalf("test fixture cannot reach the object limit within %d signals", maxDocumentSignals)
	}
	tooManyObjects := make([]any, objectSignalCount)
	for index := range tooManyObjects {
		tooManyObjects[index] = map[string]any{"templateKey": largest.Key}
	}
	objectPayload, _ := json.Marshal(map[string]any{"pous": []any{map[string]any{"name": "OBJECT_LIMIT_POU", "signals": tooManyObjects}}})
	objectResponse := postJSON(t, server.URL+"/api/generate", server.URL, objectPayload)
	defer objectResponse.Body.Close()
	if objectResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("too many objects status=%d want 400: %s", objectResponse.StatusCode, readBody(objectResponse.Body))
	}
}

// TestAPIPersistenceFailureRemainsServerErrorWithManualIDs forces allocator persistence failure after a manual-ID
// library request and requires HTTP 500 rather than a validation response.
func TestAPIPersistenceFailureRemainsServerErrorWithManualIDs(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	temp := isolatedHTTPTemp(t)
	statePath := filepath.Join(temp, "state.json")
	allocator, err := allocation.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(statePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	application := New(repository, config.Default(), allocator, filepath.Join(temp, "output"), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	pouID := int64(123456)
	payload, _ := json.Marshal(map[string]any{
		"templateKey": catalog.Templates[0].Key,
		"objectName":  "_PERSISTENCE_TEST",
		"pouName":     "PERSISTENCE_TEST",
		"nameMode":    "base",
		"pouId":       pouID,
	})
	response := postJSON(t, server.URL+"/api/generate", server.URL, payload)
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("persistence status=%d want 500: %s", response.StatusCode, readBody(response.Body))
	}
}

// TestMalformedOriginIsRejectedWithoutPanic sends an invalid Origin header to library refresh and requires a safe
// HTTP 403 response.
func TestMalformedOriginIsRejectedWithoutPanic(t *testing.T) {
	application := New(nil, config.Config{}, nil, isolatedHTTPTemp(t), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	request := httptest.NewRequest(http.MethodPost, "/api/refresh", bytes.NewReader([]byte(`{}`)))
	request.Header.Set("Origin", "%")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("malformed Origin status=%d want 403", response.Code)
	}
}

// TestDecodeJSONRejectsBodyOverEightMiB passes an oversized JSON string through the HTTP decoder and requires its
// body-size limit to reject the request.
func TestDecodeJSONRejectsBodyOverEightMiB(t *testing.T) {
	payload := `{"value":"` + strings.Repeat("x", (8<<20)+1) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(payload))
	response := httptest.NewRecorder()
	var target map[string]any
	if err := decodeJSON(response, request, &target); err == nil {
		t.Fatal("JSON body over 8 MiB was accepted")
	}
}

// isolatedLibraryDirectory copies the read-only AD3 library fixture into isolated HTTP-test storage so
// import/refresh tests cannot mutate user data.
func isolatedLibraryDirectory(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "libraries", "Library AD3_v2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	directory := isolatedHTTPTemp(t)
	if err := os.WriteFile(filepath.Join(directory, "Library AD3_v2.xml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return directory
}

// templateKeyByID finds a required template in the loaded catalog and fails explicitly if the HTTP fixture lacks
// it.
func templateKeyByID(t *testing.T, catalog library.Catalog, id string) string {
	t.Helper()
	for _, item := range catalog.Templates {
		if item.ID == id {
			return item.Key
		}
	}
	t.Fatalf("template %s missing", id)
	return ""
}

// newDocumentTestServer starts a local HTTP test server with the supplied repository and temporary allocator/output
// paths.
func newDocumentTestServer(t *testing.T, repository *library.Repository) *httptest.Server {
	t.Helper()
	temp := isolatedHTTPTemp(t)
	allocator, err := allocation.NewAllocator(filepath.Join(temp, "data", "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	application := New(
		repository,
		config.Default(),
		allocator,
		filepath.Join(temp, "output"),
		fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}},
		log.New(io.Discard, "", 0),
	)
	return httptest.NewServer(application.Handler())
}

// postJSON sends JSON with the requested Origin to the local test server and returns the response for status/body
// assertions.
func postJSON(t *testing.T, address, origin string, payload []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// readBody reads an HTTP response body into text for test diagnostics.
func readBody(reader io.Reader) string {
	data, _ := io.ReadAll(reader)
	return string(data)
}
