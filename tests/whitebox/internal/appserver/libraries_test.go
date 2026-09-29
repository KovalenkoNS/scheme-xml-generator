// HTTP import checks exercise validation, local-origin protection and catalog
// visibility with isolated directories; no user libraries are modified.
package appserver

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
)

const importLibraryXML = `<root><SCADATA_VER VER="29"/><SECTION Num="2"><OTHER><OBJTYPE ID="10" Name="Library type"><ISAOBJLIST><ISAOBJ ID="11" Prefix="value"><ISATNAME>REAL</ISATNAME></ISAOBJ></ISAOBJLIST></OBJTYPE></OTHER></SECTION></root>`

// libraryImportApp creates a library-import HTTP server with empty temporary library storage and returns that path
// for write checks.
func libraryImportApp(t *testing.T) (*Server, string) {
	t.Helper()
	temp := isolatedHTTPTemp(t)
	libraries := filepath.Join(temp, "libraries")
	if err := os.Mkdir(libraries, 0o755); err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(libraries)
	if _, err := repository.Refresh(); err != nil {
		t.Fatal(err)
	}
	allocator, err := generator.NewAllocator(filepath.Join(temp, "state.json"), config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	app := New(repository, generator.Generator{Config: config.Default()}, allocator, filepath.Join(temp, "output"), fstest.MapFS{}, log.New(io.Discard, "", 0))
	return app, libraries
}

// libraryUpload builds a multipart import containing the requested count of named XML files for HTTP import
// validation.
func libraryUpload(t *testing.T, name, data string, count int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < count; i++ {
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/libraries/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://localhost")
	return request
}

// TestLibraryImportEndpointAddsTypesAndReusesIdenticalCopy imports a types-only library twice and checks catalog
// refresh, byte deduplication and one persisted file.
func TestLibraryImportEndpointAddsTypesAndReusesIdenticalCopy(t *testing.T) {
	app, directory := libraryImportApp(t)
	var first library.ImportResult
	for index, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, libraryUpload(t, "../source.XML", importLibraryXML, 1))
		if response.Code != wantStatus {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var result library.ImportResult
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Catalog.Types) != 1 || result.Catalog.Types[0].Fields[0].TypeName != "REAL" || len(result.Catalog.Templates) != 0 {
			t.Fatalf("missing template-free types: %+v", result.Catalog)
		}
		if index == 0 {
			first = result
		} else if result.File != first.File || result.Created {
			t.Fatalf("repeated copy differs: %+v", result)
		}
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 || entries[0].Name() != first.File {
		t.Fatalf("unexpected library files: %v", entries)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	var catalog library.Catalog
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil || len(catalog.Types) != 1 {
		t.Fatalf("catalog endpoint not refreshed: %v", err)
	}
}

// TestLibraryImportRejectsUnsafeAndInvalidRequestsWithoutFiles checks invalid XML, file names, upload counts,
// origins and sizes cannot change the stored libraries or catalog.
func TestLibraryImportRejectsUnsafeAndInvalidRequestsWithoutFiles(t *testing.T) {
	for _, test := range []struct {
		name, filename, source, origin string
		count, status                  int
		oversized                      bool
	}{
		{name: "malformed", filename: "bad.xml", source: `<root><SECTION>`, count: 1, status: http.StatusBadRequest},
		{name: "not-library", filename: "bad.xml", source: `<root/>`, count: 1, status: http.StatusBadRequest},
		{name: "wrong-extension", filename: "source.txt", source: importLibraryXML, count: 1, status: http.StatusBadRequest},
		{name: "multiple-files", filename: "source.xml", source: importLibraryXML, count: 2, status: http.StatusBadRequest},
		{name: "missing-file", filename: "source.xml", count: 0, status: http.StatusBadRequest},
		{name: "cross-origin", filename: "source.xml", source: importLibraryXML, origin: "https://untrusted.invalid", count: 1, status: http.StatusForbidden},
		{name: "over-limit", filename: "source.xml", source: importLibraryXML, count: 1, status: http.StatusRequestEntityTooLarge, oversized: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, directory := libraryImportApp(t)
			request := libraryUpload(t, test.filename, test.source, test.count)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.oversized {
				request.ContentLength = library.MaxLibraryBytes + (128 << 10) + 1
			}
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			entries, _ := os.ReadDir(directory)
			if len(entries) != 0 || len(app.repository.Catalog().Libraries) != 0 {
				t.Fatal("rejected request changed library files/catalog")
			}
		})
	}
}
