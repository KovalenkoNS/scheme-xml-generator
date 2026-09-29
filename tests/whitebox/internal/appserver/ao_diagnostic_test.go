// AO HMI HTTP generation, controller selection, persisted downloads and atomic allocation failures.
package appserver

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/config"
)

type diagnosticTestXML struct {
	XMLName xml.Name
	Common  struct {
		Project string `xml:"Project,attr"`
	} `xml:"Common"`
	Pages []struct {
		IDAttribute string `xml:"ID,attr"`
		ID          string `xml:"ID"`
		Name        string `xml:"NAME"`
		Primitives  []struct {
			ID     string `xml:"SourceT11ID,attr"`
			X      int    `xml:"X,attr"`
			Y      int    `xml:"Y,attr"`
			MSID   string `xml:"ObjMSID"`
			CardID string `xml:"CardID"`
		} `xml:"PageLayers>OneLayer>OnePrim"`
	} `xml:"Pages>OnePage"`
	Cards []struct {
		ID   string `xml:"ID,attr"`
		Info string `xml:"CardInfo,attr"`
	} `xml:"CARDSINFO>rec"`
}

// TestAODiagnosticAPIAllFramesForSelectedController generates AO HMI for one selected PLC and checks downloaded
// bytes, module pages, card reuse and allocation against the parsed table.
func TestAODiagnosticAPIAllFramesForSelectedController(t *testing.T) {
	second := strings.SplitN(strings.ReplaceAll(smallAOMap, "A11-00", "A11-01"), "\n", 2)[1]
	other := strings.SplitN(strings.ReplaceAll(smallAOMap, "FCS_MAIN", "FCS_OTHER"), "\n", 2)[1]
	data := []byte(smallAOMap + second + other)
	plan, err := aomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	application, statePath, outputDir := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", data, map[string]string{
		"fileName": "diag", "diagnostic": `{"fcs":["FCS_MAIN"]}`,
		"context": `{"project":"test project","resourceNumber":"2"}`,
	}))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d: %s", response.Code, response.Body.String())
	}
	var result aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Kind != "diagnostic" || len(result.Files) != 1 || result.Summary.FrameCount != 2 || result.Summary.SignalCount != 8 || result.Summary.Cards != 7 || result.Summary.Graphics != 10 || result.Summary.POUCount != 0 || result.Summary.Blocks != 0 {
		t.Fatalf("wrong diagnostic summary: %+v", result)
	}
	file := result.Files[0]
	if file.ControllerName != "FCS_MAIN" || file.FileName != "diag_diagnostic_FCS_MAIN.xml" || len(file.Summary.Frames) != 2 {
		t.Fatalf("wrong diagnostic metadata: %+v", file)
	}
	download := httptest.NewRecorder()
	application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
	if download.Code != http.StatusOK {
		t.Fatalf("download status=%d", download.Code)
	}
	onDisk, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
	if err != nil || !bytes.Equal(onDisk, download.Body.Bytes()) {
		t.Fatalf("disk/download mismatch: %v", err)
	}
	doc := assertDiagnosticTestXML(t, download.Body.Bytes(), file.ControllerName, plan, "2", map[string]bool{}, map[string]bool{}, map[string]bool{})
	if doc.Common.Project != "test project" || !strings.Contains(doc.Pages[0].Name, "A11_00") || !strings.Contains(doc.Pages[1].Name, "A11_01") {
		t.Fatalf("separate module pages or context lost: %+v", doc)
	}
	// A repeat is still a visible diagnostic widget referencing the same card.
	if doc.Pages[0].Primitives[1].CardID != doc.Pages[1].Primitives[1].CardID {
		t.Error("same object received two card identities in one PLC")
	}
	assertDiagnosticAllocatorState(t, statePath, 2, 10, 7)
}

// assertDiagnosticAllocatorState reads the persisted ID cursors after HMI generation and checks exactly the
// expected page, primitive and card increments.
func assertDiagnosticAllocatorState(t *testing.T, path string, pages, primitives, cards int64) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state config.IDDefaults
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	expected := config.Default().IDs
	expected.NextPage += pages
	expected.NextT11 += primitives
	expected.NextCard += cards
	if state != expected {
		t.Fatalf("incorrect diagnostic allocation: got%+v want%+v", state, expected)
	}
}

// assertDiagnosticTestXML compares a generated AO HMI document with selected-controller modules, channel bindings
// and unique transport IDs.
func assertDiagnosticTestXML(t *testing.T, data []byte, controllerName string, plan *aomap.Plan, resource string, pageIDs, primitiveIDs, cardIDs map[string]bool) diagnosticTestXML {
	t.Helper()
	var doc diagnosticTestXML
	if err := xml.Unmarshal(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.XMLName.Local != "BufScada" || bytes.Contains(data, []byte("<POUS")) || bytes.Contains(data, []byte("<STCODE")) {
		t.Fatal("wrong document type for operator-panel diagnostics")
	}
	expected := map[string]aomap.Module{}
	suffix := controllerName
	if index := strings.LastIndex(controllerName, "_SC_"); index >= 0 {
		suffix = controllerName[index+4:]
	}
	for _, group := range plan.Groups {
		if group.ControllerName == controllerName {
			for _, module := range group.Modules {
				expected["AO_"+suffix+"_"+module.Name+"_AOC4H"] = module
			}
		}
	}
	if len(doc.Pages) != len(expected) {
		t.Fatalf("controller %s pages=%d expected=%d", controllerName, len(doc.Pages), len(expected))
	}
	cards := map[string]string{}
	usedCards := map[string]bool{}
	for _, card := range doc.Cards {
		assertFreshAOID(t, cardIDs, card.ID, "diagnostic card")
		cards[card.ID] = card.Info
		if !strings.HasPrefix(card.Info, "2/"+controllerName+"/"+resource+"/") {
			t.Fatalf("foreign controller card: %s", card.Info)
		}
	}
	for _, page := range doc.Pages {
		module, ok := expected[page.Name]
		if !ok || page.IDAttribute != page.ID || len(page.Primitives) != 5 {
			t.Fatalf("foreign/duplicate/malformed page: %+v", page)
		}
		delete(expected, page.Name)
		assertFreshAOID(t, pageIDs, page.ID, "diagnostic page")
		for index, primitive := range page.Primitives {
			assertFreshAOID(t, primitiveIDs, primitive.ID, "diagnostic primitive")
			if index == 0 {
				if primitive.MSID != "3655" || primitive.CardID != "0" || primitive.X != 0 || primitive.Y != 0 {
					t.Fatal("native diagnostic background changed")
				}
				continue
			}
			channel := module.Channels[index-1]
			want := fmt.Sprintf("2/%s/%s/%s/(AN_v1)", controllerName, resource, channel.Tag)
			if primitive.MSID != "3679" || primitive.X != 70 || primitive.Y != 54+24*(index-1) || cards[primitive.CardID] != want {
				t.Fatalf("wrong channel widget/binding: %+v expected%s", primitive, want)
			}
			usedCards[primitive.CardID] = true
		}
	}
	if len(expected) != 0 || len(usedCards) != len(cards) {
		t.Fatalf("missing frames or unreferenced cards: %v %v", expected, cards)
	}
	return doc
}

// TestAODiagnosticAPIActualMapAllControllers generates HMI from the optional complete AO map and checks one
// isolated downloadable file per PLC.
func TestAODiagnosticAPIActualMapAllControllers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "XML dev", "AO_excell_import_scheme.txt"))
	if os.IsNotExist(err) {
		t.Skip("local AO source fixture missing")
	}
	if err != nil {
		t.Fatal(err)
	}
	plan, err := aomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	controllerName := []string{}
	seen := map[string]bool{}
	for _, group := range plan.Groups {
		if !seen[group.ControllerName] {
			controllerName = append(controllerName, group.ControllerName)
			seen[group.ControllerName] = true
		}
	}
	selection, _ := json.Marshal(map[string]any{"fcs": controllerName})
	application, statePath, _ := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", data, map[string]string{"diagnostic": string(selection)}))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d: %s", response.Code, response.Body.String())
	}
	var result aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Kind != "diagnostic" || len(result.Files) != 8 || result.Summary.FrameCount != 128 || result.Summary.SignalCount != 512 || result.Summary.Graphics != 640 || result.Summary.Cards != 309 || result.Summary.POUCount != 0 {
		t.Fatalf("wrong full-map summary: %+v", result.Summary)
	}
	pageIDs, primitiveIDs, cardIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, file := range result.Files {
		if !seen[file.ControllerName] {
			t.Fatalf("unexpected/duplicate FCS %s", file.ControllerName)
		}
		delete(seen, file.ControllerName)
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
		if download.Code != http.StatusOK {
			t.Fatalf("download status=%d", download.Code)
		}
		assertDiagnosticTestXML(t, download.Body.Bytes(), file.ControllerName, plan, "1", pageIDs, primitiveIDs, cardIDs)
	}
	assertDiagnosticAllocatorState(t, statePath, 128, 640, 309)
}

// TestAODiagnosticAPIRejectsInvalidSelectionAndContextBeforeWriting submits malformed selections and HMI contexts;
// HTTP validation must fail before output or ID allocation.
func TestAODiagnosticAPIRejectsInvalidSelectionAndContextBeforeWriting(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	for _, selection := range []string{"", "null", `{}`, `{"fcs":null}`, `{"fcs":[]}`, `{"fcs":[null]}`, `{"fcs":["UNKNOWN"]}`, `{"fcs":["FCS_MAIN","UNKNOWN"]}`, `{"fcs":["FCS_MAIN","FCS_MAIN"]}`, `{"fcs":["FCS_MAIN"],"pous":[]}`, `{"fcs":["FCS_MAIN"]} {}`} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", []byte(smallAOMap), map[string]string{"diagnostic": selection}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid selection %s status=%d: %s", selection, response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
	for _, context := range []string{`null`, `{"resourceNumber":null}`, `{"resourceNumber":1}`, `{"resourceNumber":"0"}`, `{"resourceNumber":"../other"}`, `{"resourceNumber":"2147483648"}`, `{"controllerId":"637"}`, `{"project":"bad\u0000project"}`, `{} {}`} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", []byte(smallAOMap), map[string]string{"diagnostic": `{"fcs":["FCS_MAIN"]}`, "context": context}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid context %s status=%d: %s", context, response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
}

// TestAODiagnosticAPIOriginAndPersistenceFailure checks that AO-HMI rejects foreign origins and reports failed
// allocator persistence as HTTP 500 without output.
func TestAODiagnosticAPIOriginAndPersistenceFailure(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	fields := map[string]string{"diagnostic": `{"fcs":["FCS_MAIN"]}`}
	request := aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", []byte(smallAOMap), fields)
	request.Header.Set("Origin", "https://example.com")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote origin status=%d", response.Code)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
	if err := os.MkdirAll(statePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-diagnostic", []byte(smallAOMap), fields))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("persistence status=%d: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}
