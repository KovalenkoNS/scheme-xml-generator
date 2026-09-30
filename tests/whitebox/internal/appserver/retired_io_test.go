// Проверка отказа старого io.modules через общий FBD API без расхода ID и изменения результатов.
package appserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/library"
	"testing"
	"testing/fstest"
)

// TestAPIDocumentRejectsRetiredIOWithoutChangingExistingResults сначала выпускает обычный библиотечный FBD.
// Затем проверяет старые аппаратные секции всех направлений и неизменность allocator/сохранённого XML после каждого отказа.
func TestAPIDocumentRejectsRetiredIOWithoutChangingExistingResults(t *testing.T) {
	repository := library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := templateKeyByID(t, catalog, "17510")
	root := isolatedHTTPTemp(t)
	statePath, outputDir := filepath.Join(root, "state.json"), filepath.Join(root, "output")
	allocator, err := allocation.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	application := New(repository, config.Default(), allocator, outputDir, fstest.MapFS{}, log.New(io.Discard, "", 0))
	post := func(pous []any) *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]any{"fileName": "existing-library-result", "pous": pous})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "http://localhost/api/generate", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		application.Handler().ServeHTTP(response, request)
		return response
	}
	flat := map[string]any{
		"name": "LIBRARY_ONLY", "defaultTemplateKey": key,
		"signals": []any{map[string]any{"objectName": "_BASELINE", "nameMode": "base"}},
	}
	ok := post([]any{flat})
	if ok.Code != http.StatusCreated {
		t.Fatalf("ordinary library FBD failed: %d %s", ok.Code, ok.Body.String())
	}
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("baseline outputs=%v err=%v", entries, err)
	}
	resultPath := filepath.Join(outputDir, entries[0].Name())
	resultBefore, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"AI", "AO", "DI", "DO"} {
		for _, explicitID := range []bool{false, true} {
			idMode := "missing-id"
			if explicitID {
				idMode = "explicit-id"
			}
			t.Run(direction+"/"+idMode, func(t *testing.T) {
				module := map[string]any{"signals": []any{map[string]any{"objectName": "_PHYSICAL", "nameMode": "base"}}}
				if explicitID {
					module["id"] = 17
				}
				physical := map[string]any{
					"name": "RETIRED_IO", "defaultTemplateKey": key,
					"io": map[string]any{"type": direction, "modules": []any{module}},
				}
				// The second POU must prevent partial allocation/output for the valid first POU too.
				response := post([]any{flat, physical})
				assertLegacyFBDUnavailable(t, response)
				stateAfter, err := os.ReadFile(statePath)
				if err != nil || !bytes.Equal(stateBefore, stateAfter) {
					t.Fatalf("retired IO changed allocator: %v", err)
				}
				entriesAfter, err := os.ReadDir(outputDir)
				if err != nil || len(entriesAfter) != 1 || entriesAfter[0].Name() != entries[0].Name() {
					t.Fatalf("retired IO changed output inventory: %v err=%v", entriesAfter, err)
				}
				resultAfter, err := os.ReadFile(resultPath)
				if err != nil || !bytes.Equal(resultBefore, resultAfter) {
					t.Fatalf("retired IO changed existing XML: %v", err)
				}
			})
		}
	}
}
