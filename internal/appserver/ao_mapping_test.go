package appserver

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"scheme-xml-generator/internal/aomap"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
)

const smallAOMap = "FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта\n" +
	"FCS_MAIN\tCAB\tA11-00\t0\t_IO_QU*A11-00*_0.ValueDINT := REAL_TO_DINT(_TEST_AO.OUT, 0.0, 100.0);\tA11-00\t\tAO\tAN_v1\n"

func aoTestApplication(t *testing.T) (*Server, string, string) {
	t.Helper()
	temp := t.TempDir()
	statePath := filepath.Join(temp, "data", "state.json")
	outputDir := filepath.Join(temp, "output")
	allocator, err := generator.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	// No repository refresh and no library files: the native profile is standalone.
	application := New(library.NewRepository(filepath.Join(temp, "absent-libraries")), generator.Generator{Config: config.Default()}, allocator, outputDir, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}, log.New(io.Discard, "", 0))
	return application, statePath, outputDir
}

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

func TestAOProfilePreviewAndGenerationWithoutLibraries(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	profile := httptest.NewRecorder()
	application.Handler().ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/temporary/ao/profile", nil))
	if profile.Code != http.StatusOK {
		t.Fatalf("profile status=%d: %s", profile.Code, profile.Body.String())
	}
	var profileBody struct {
		Context     generator.AOMappingContext `json:"context"`
		Description string                     `json:"description"`
	}
	if err := json.Unmarshal(profile.Body.Bytes(), &profileBody); err != nil {
		t.Fatal(err)
	}
	if profileBody.Context != generator.DefaultAOMappingContext() || profileBody.Description == "" {
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

	generated := httptest.NewRecorder()
	application.Handler().ServeHTTP(generated, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), map[string]string{
		"fileName": "AO_test", "context": `{"project":"test-project","controllerId":"123"}`,
	}))
	if generated.Code != http.StatusCreated {
		t.Fatalf("generate status=%d: %s", generated.Code, generated.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(generated.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 1 || batch.Summary.POUCount != 1 || batch.Summary.Blocks != 4 || batch.Summary.Cards != 4 || batch.Summary.IOModuleCount != 1 {
		t.Fatalf("bad generated batch: %+v", batch)
	}
	result := batch.Files[0]
	if result.FCS != "FCS_MAIN" || result.FileName != "AO_test_FCS_MAIN.xml" || result.Summary.POUCount != 1 || result.Summary.Blocks != 4 || result.Summary.Cards != 4 {
		t.Fatalf("bad generated result: %+v", result)
	}
	download := httptest.NewRecorder()
	application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, result.URL, nil))
	if download.Code != http.StatusOK {
		t.Fatalf("download status=%d: %s", download.Code, download.Body.String())
	}
	for _, want := range []string{`Project="test-project"`, `ControllerID="123"`, `ControllerTypeName="TENIX-CPU715"`, `NAME="AO_A11"`, `Info="_TEST_AO"`, `Info="_FCS_MAIN_A11_00_3"`, `<IV>,,,100.0</IV>`, `IsRetain="-1"`} {
		if !strings.Contains(download.Body.String(), want) {
			t.Errorf("download missing %s", want)
		}
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("successful generation did not commit state: %v", err)
	}
}

func TestAOAPIProcessesActualMapAndDownloadsAllPOUs(t *testing.T) {
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
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", data, map[string]string{"fileName": "AO_actual_map"}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate status=%d: %s", response.Code, response.Body.String())
	}
	var generated aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &generated); err != nil {
		t.Fatal(err)
	}
	if len(generated.Files) != 8 || generated.Summary.POUCount != 21 || generated.Summary.Blocks != 309 || generated.Summary.Cards != 309 || generated.Summary.IOModuleCount != 128 || generated.Summary.SignalCount != 512 || generated.Summary.SkippedDuplicateCount != 203 {
		t.Fatalf("unexpected actual map summary: %+v", generated.Summary)
	}
	// Coordinates represent channel positions, not a compact sequence of owners.
	// A duplicated ST-connected tag must leave exactly its own position empty.
	groupsByFCS := make(map[string]map[string]map[int]aomap.Channel)
	for _, group := range plan.Groups {
		if groupsByFCS[group.FCS] == nil {
			groupsByFCS[group.FCS] = make(map[string]map[int]aomap.Channel)
		}
		positions := make(map[int]aomap.Channel)
		position := 0
		for _, module := range group.Modules {
			for _, channel := range module.Channels {
				positions[50+190*position] = channel
				position++
			}
		}
		groupsByFCS[group.FCS][group.POUName] = positions
	}
	t11IDs, cardIDs, pouIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	fcsSeen := map[string]bool{}
	for _, file := range generated.Files {
		if fcsSeen[file.FCS] {
			t.Fatalf("duplicate FCS file %s", file.FCS)
		}
		fcsSeen[file.FCS] = true
		if file.FileName != "AO_actual_map_"+file.FCS+".xml" || file.Summary.Links != 0 || file.Summary.Graphics != 0 {
			t.Fatalf("unexpected per-FCS metadata: %+v", file)
		}
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
		if download.Code != http.StatusOK {
			t.Fatalf("download status=%d", download.Code)
		}
		doc := decodeAOTestXML(t, download.Body.Bytes())
		groups := groupsByFCS[file.FCS]
		if len(doc.POUs) != len(groups) {
			t.Fatalf("%s contains %d POU, expected %d", file.FCS, len(doc.POUs), len(groups))
		}
		tagsSeen := map[string]bool{}
		cardReferences := map[string]string{}
		for _, pou := range doc.POUs {
			if groups[pou.Name] == nil || !strings.HasPrefix(pou.Name, "AO_A") {
				t.Fatalf("foreign or incorrectly named POU %s in FCS %s", pou.Name, file.FCS)
			}
			assertFreshAOID(t, pouIDs, pou.ID, "POU")
			occupied := map[int]bool{}
			for _, block := range pou.Blocks {
				channel, ok := groups[pou.Name][block.Graphics.Y]
				if !ok || channel.Duplicate || occupied[block.Graphics.Y] || channel.Tag != block.Info || block.Graphics.X != 500 {
					t.Fatalf("wrong or duplicated channel at (%d,%d) in %s/%s: block=%s channel=%+v", block.Graphics.X, block.Graphics.Y, file.FCS, pou.Name, block.Info, channel)
				}
				occupied[block.Graphics.Y] = true
				if tagsSeen[strings.ToLower(block.Info)] {
					t.Fatalf("repeated owner %s in FCS %s", block.Info, file.FCS)
				}
				tagsSeen[strings.ToLower(block.Info)] = true
				if block.Params.CardID == "" || cardReferences[block.Params.CardID] != "" {
					t.Fatalf("missing or repeated card reference %s", block.Params.CardID)
				}
				cardReferences[block.Params.CardID] = block.Info
				assertFreshAOID(t, t11IDs, block.ID, "T11ID")
			}
			for y, channel := range groups[pou.Name] {
				if occupied[y] == channel.Duplicate {
					t.Fatalf("channel position %s/%s y=%d has occupied=%t duplicate=%t", file.FCS, pou.Name, y, occupied[y], channel.Duplicate)
				}
			}
		}
		for _, card := range doc.Cards {
			assertFreshAOID(t, cardIDs, card.ID, "card ID")
			if cardReferences[card.ID] != card.Info {
				t.Fatalf("unreferenced or mismatched card %s (%s)", card.ID, card.Info)
			}
			delete(cardReferences, card.ID)
		}
		if len(cardReferences) != 0 {
			t.Fatalf("dangling card references: %v", cardReferences)
		}
		if file.FCS == "3000_D_SC_B01" {
			for _, want := range []string{`NAME="AO_A11"`, `NAME="AO_A12"`, `Info="_3107_TV_64101A"`, `Info="_3000_D_SC_B01_A11_04_3"`} {
				if !strings.Contains(download.Body.String(), want) {
					t.Errorf("actual map missing %s", want)
				}
			}
		}
	}
	if len(t11IDs) != 309 || len(cardIDs) != 309 || len(pouIDs) != 21 {
		t.Fatalf("unexpected unique transport IDs: T11=%d cards=%d POU=%d", len(t11IDs), len(cardIDs), len(pouIDs))
	}
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		NextT11  int64 `json:"nextT11"`
		NextCard int64 `json:"nextCard"`
		NextPOU  int64 `json:"nextPou"`
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal(err)
	}
	defaults := config.Default().IDs
	if state.NextT11 != defaults.NextT11+309 || state.NextCard != defaults.NextCard+309 || state.NextPOU != defaults.NextPOU+21 {
		t.Fatalf("allocator consumed IDs for empty duplicate positions: %+v", state)
	}
}

type aoTestDocument struct {
	POUs []struct {
		ID     string `xml:"ID,attr"`
		Name   string `xml:"NAME,attr"`
		Number string `xml:"POUNum,attr"`
		Blocks []struct {
			ID       string `xml:"T11ID,attr"`
			Info     string `xml:"Info,attr"`
			Graphics struct {
				X int `xml:"X,attr"`
				Y int `xml:"Y,attr"`
			} `xml:"Graphics"`
			Params struct {
				CardID string `xml:"cardId,attr"`
			} `xml:"Params"`
		} `xml:"ISAGraf>Blocks>Block"`
	} `xml:"POUS>OnePOU"`
	Cards []struct {
		ID   string `xml:"ID,attr"`
		Info string `xml:"Info,attr"`
	} `xml:"ISACARDSINFO>rec"`
}

func decodeAOTestXML(t *testing.T, data []byte) aoTestDocument {
	t.Helper()
	var document aoTestDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func assertFreshAOID(t *testing.T, ids map[string]bool, id, kind string) {
	t.Helper()
	if id == "" || ids[id] {
		t.Fatalf("empty or repeated %s %q across generated files", kind, id)
	}
	ids[id] = true
}

func TestAOSeparateFCSCanReusePOUAndInstanceNames(t *testing.T) {
	application, _, outputDir := aoTestApplication(t)
	other := strings.SplitN(strings.ReplaceAll(smallAOMap, "FCS_MAIN", "FCS_OTHER"), "\n", 2)[1]
	data := []byte(smallAOMap + other)
	for run := 1; run <= 2; run++ {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", data, map[string]string{"fileName": "shared.xml"}))
		if response.Code != http.StatusCreated {
			t.Fatalf("generation status=%d: %s", response.Code, response.Body.String())
		}
		var batch aoGenerateResponse
		if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
			t.Fatal(err)
		}
		if len(batch.Files) != 2 || batch.Summary.Cards != 8 || batch.Summary.Blocks != 8 || batch.Summary.POUCount != 2 || batch.Summary.SkippedDuplicateCount != 0 {
			t.Fatalf("same-name instances must have separate controller cards: %+v", batch)
		}
		for _, file := range batch.Files {
			wantedName := "shared_" + file.FCS + ".xml"
			if run > 1 {
				wantedName = "shared_" + file.FCS + "-2.xml"
			}
			if file.FileName != wantedName {
				t.Errorf("name=%s want=%s", file.FileName, wantedName)
			}
			data, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
			if err != nil {
				t.Fatal(err)
			}
			doc := decodeAOTestXML(t, data)
			if len(doc.POUs) != 1 || doc.POUs[0].Name != "AO_A11" || doc.POUs[0].Number != "36" || len(doc.Cards) != 4 {
				t.Fatalf("unexpected controller document %+v", doc)
			}
		}
	}
}

func TestAODuplicateLeavesEmptyPositionAndPrefersMainModule(t *testing.T) {
	application, _, outputDir := aoTestApplication(t)
	header := strings.SplitN(smallAOMap, "\n", 2)[0] + "\n"
	row := func(module string, channel int, tag string) string {
		return fmt.Sprintf("FCS_MAIN\tCAB\t%s\t%d\t_IO_QU*%s*_%d.ValueDINT := REAL_TO_DINT(%s.OUT, 0.0, 100.0);\tA11-01\tA11-00\tAOR\tAN_v1\n", module, channel, module, channel, tag)
	}
	// The redundant occurrence appears first in both input and module ordering.
	data := []byte(header + row("A11-00", 0, "_TEST_AO") + row("A11-00", 1, "_NEXT_AO") + row("A11-01", 0, "_TEST_AO"))
	preview := httptest.NewRecorder()
	application.Handler().ServeHTTP(preview, aoMultipartRequest(t, "/api/temporary/ao/preview", data, nil))
	var plan aomap.Plan
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &plan) != nil {
		t.Fatalf("preview status=%d: %s", preview.Code, preview.Body.String())
	}
	if plan.DuplicateCount != 1 || !plan.Groups[0].Modules[0].Channels[0].Duplicate || plan.Groups[0].Modules[0].Channels[0].Tag != "_TEST_AO" || plan.Groups[0].Modules[1].Channels[0].Duplicate {
		t.Fatalf("preview did not preserve main owner and explain the hole: %+v", plan)
	}
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", data, nil))
	if response.Code != http.StatusCreated {
		t.Fatalf("generation status=%d: %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 1 || batch.Summary.SignalCount != 8 || batch.Summary.Blocks != 7 || batch.Summary.Cards != 7 || batch.Summary.SkippedDuplicateCount != 1 {
		t.Fatalf("bad skipped-duplicate summary: %+v", batch)
	}
	xmlData, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
	if err != nil {
		t.Fatal(err)
	}
	document := decodeAOTestXML(t, xmlData)
	if len(document.POUs) != 1 || len(document.POUs[0].Blocks) != 7 {
		t.Fatalf("unexpected document: %+v", document)
	}
	owners, next := 0, 0
	for _, block := range document.POUs[0].Blocks {
		if block.Graphics.Y == 50 || block.Info == "_FCS_MAIN_A11_00_0" {
			t.Fatal("duplicate ST-connected channel was replaced with a block or a reserve")
		}
		switch block.Info {
		case "_TEST_AO":
			owners++
			if block.Graphics.Y != 810 {
				t.Fatalf("main module owner moved to y=%d, want810", block.Graphics.Y)
			}
		case "_NEXT_AO":
			next++
			if block.Graphics.Y != 240 {
				t.Fatalf("next channel moved into the gap: y=%d, want240", block.Graphics.Y)
			}
		}
	}
	if owners != 1 || next != 1 {
		t.Fatalf("main owner count=%d, following channel count=%d", owners, next)
	}
	for _, card := range document.Cards {
		if card.Info == "_FCS_MAIN_A11_00_0" {
			t.Fatal("duplicate position created an unused reserve card")
		}
	}
}

func TestAOInvalidLaterFCSDoesNotWriteEarlierFilesOrConsumeIDs(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	other := strings.SplitN(strings.ReplaceAll(smallAOMap, "FCS_MAIN", "FCS_OTHER"), "\n", 2)[1]
	other = strings.ReplaceAll(other, "0.0, 100.0", "0.0, 200.0")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap+other), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want400: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

func TestAOOutputNameKeepsFCSAndRemovesPaths(t *testing.T) {
	for _, requested := range []string{"", "../../unsafe.xml", strings.Repeat("long", 70) + ".xml"} {
		name := aoOutputName(requested, "3000_D_SC_B01")
		if !strings.HasSuffix(name, "_3000_D_SC_B01.xml") || strings.ContainsAny(name, `/\\`) {
			t.Fatalf("unsafe or unidentifiable result name %q", name)
		}
	}
}

func TestAOInvalidRequestsDoNotConsumeIDs(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	tests := []struct {
		name   string
		data   []byte
		fields map[string]string
		origin string
		want   int
	}{
		{"empty file", nil, nil, "", http.StatusBadRequest},
		{"invalid TXT", []byte("not a map"), nil, "", http.StatusBadRequest},
		{"unknown context", []byte(smallAOMap), map[string]string{"context": `{"surprise":"value"}`}, "", http.StatusBadRequest},
		{"null context", []byte(smallAOMap), map[string]string{"context": `null`}, "", http.StatusBadRequest},
		{"null context field", []byte(smallAOMap), map[string]string{"context": `{"controllerId":null}`}, "", http.StatusBadRequest},
		{"trailing JSON", []byte(smallAOMap), map[string]string{"context": `{} {}`}, "", http.StatusBadRequest},
		{"numeric context", []byte(smallAOMap), map[string]string{"context": `{"controllerId":123}`}, "", http.StatusBadRequest},
		{"out of range context", []byte(smallAOMap), map[string]string{"context": `{"controllerId":"2147483648"}`}, "", http.StatusBadRequest},
		{"unknown form field", []byte(smallAOMap), map[string]string{"surprise": "value"}, "", http.StatusBadRequest},
		{"oversize file", bytes.Repeat([]byte("x"), int(maxAOMappingFileBytes)+1), nil, "", http.StatusBadRequest},
		{"oversize body", bytes.Repeat([]byte("x"), int(maxAOMappingFileBytes+maxAOMultipartExtra)+1), nil, "", http.StatusBadRequest},
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

func TestAOAllocatorPersistenceFailureIsServerError(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	if err := os.MkdirAll(statePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want500: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

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
