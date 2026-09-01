package appserver

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
)

func TestAPIListsPreviewsGeneratesAndDownloads(t *testing.T) {
	libraryDir, err := filepath.Abs(filepath.Join("..", "..", "libraries"))
	if err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(libraryDir)
	if _, err := repository.Refresh(); err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	allocator, err := generator.NewAllocator(filepath.Join(temp, "data", "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	var static fs.FS = fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	application := New(repository, generator.Generator{Config: config.Default()}, allocator, filepath.Join(temp, "output"), static, log.New(io.Discard, "", 0))
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
	var preview generator.NamePreview
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
		FileName string            `json:"fileName"`
		URL      string            `json:"url"`
		Summary  generator.Summary `json:"summary"`
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

func readBody(reader io.Reader) string {
	data, _ := io.ReadAll(reader)
	return string(data)
}
