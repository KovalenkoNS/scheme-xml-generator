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
	"scheme-xml-generator/internal/generator"
)

const smallAOSTConfig = `{"pous":[{"groupKey":"FCS_MAIN:A11","moduleCount":1,"moduleIds":[41]}]}`

type aoSTTestDocument struct {
	POUs []struct {
		ID    string `xml:"ID,attr"`
		Name  string `xml:"NAME,attr"`
		IsFBD string `xml:"isFBD,attr"`
		Code  string `xml:"STCODE"`
	} `xml:"POUS>OnePOU"`
}

func aoSTRequestForPlan(t *testing.T, plan *aomap.Plan) string {
	t.Helper()
	request := generator.AOSTRequest{}
	next := map[string]int64{}
	for _, group := range plan.Groups {
		pou := generator.AOSTPOURequest{GroupKey: group.Key, ModuleCount: len(group.Modules)}
		for range group.Modules {
			id := next[group.FCS]
			pou.ModuleIDs = append(pou.ModuleIDs, &id)
			next[group.FCS]++
		}
		request.POUs = append(request.POUs, pou)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAOSTAPIWithoutLibrariesAndWithAdditionalCapacity(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("modules%d", count), func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			ids := "0"
			if count == 2 {
				ids += ",41"
			}
			st := fmt.Sprintf(`{"pous":[{"groupKey":"FCS_MAIN:A11","moduleCount":%d,"moduleIds":[%s]}]}`, count, ids)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), map[string]string{
				"st": st, "fileName": "AO_test", "context": `{"project":"ST test","controllerId":"123"}`,
			}))
			if response.Code != http.StatusCreated {
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			var batch aoGenerateResponse
			if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if batch.Kind != "st" || len(batch.Files) != 1 || batch.Summary.AssignmentCount != 4*count || batch.Summary.IOModuleCount != count || batch.Summary.POUCount != 1 || batch.Summary.Blocks != 0 || batch.Summary.Cards != 0 {
				t.Fatalf("unexpected ST summary: %+v", batch)
			}
			file := batch.Files[0]
			if file.FileName != "AO_test_ST_FCS_MAIN.xml" || file.FCS != "FCS_MAIN" {
				t.Fatalf("unexpected ST file: %+v", file)
			}
			modules := file.Summary.POUs[0].IOModules
			if len(modules) != count || modules[0].BindingPrefix != "_IO_QU0" || (count == 2 && modules[1].BindingPrefix != "_IO_QU41") {
				t.Fatalf("incorrect physical module metadata: %+v", modules)
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
			text := download.Body.String()
			for _, expected := range []string{`Project="ST test"`, `ControllerID="123"`, `NAME="AO_A11_channels"`, `isFBD="0"`, "PROGRAM AO_A11_channels", "_IO_QU0_0.ValueDINT := REAL_TO_DINT(_TEST_AO.OUT, 0.0, 100.0);", "_IO_QU0_3.ValueDINT := REAL_TO_DINT(_FCS_MAIN_A11_00_3.OUT, 0.0, 100.0);", "END_PROGRAM"} {
				if !strings.Contains(text, expected) {
					t.Errorf("missing %q", expected)
				}
			}
			for _, forbidden := range []string{"<PARAMS", "<ISAGraf", "<ISAOBJSINFO", "<ISACARDSINFO", "<GrObj"} {
				if strings.Contains(text, forbidden) {
					t.Errorf("graphical section in ST: %s", forbidden)
				}
			}
			if count == 2 && !strings.Contains(text, "_IO_QU41_3.ValueDINT := REAL_TO_DINT(_FCS_MAIN_A11_01_3.OUT, 0.0, 100.0);") {
				t.Error("additional four-channel reserve module missing")
			}
			assertAOSTAllocatorState(t, statePath, 1)
		})
	}
}

func assertAOSTAllocatorState(t *testing.T, statePath string, pouCount int64) {
	t.Helper()
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state config.IDDefaults
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	expected := config.Default().IDs
	expected.NextPOU += pouCount
	if state != expected {
		t.Fatalf("ST should consume only POU IDs: got%+v want%+v", state, expected)
	}
}

func TestAOSTAPIActualMapPreservesAllRepeatedAssignments(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "XML dev", "AO_excell_import_scheme.txt"))
	if os.IsNotExist(err) {
		t.Skip("local AO map fixture is absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	plan, err := aomap.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	application, statePath, _ := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", data, map[string]string{"st": aoSTRequestForPlan(t, plan), "fileName": "AO_actual"}))
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d: %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Kind != "st" || len(batch.Files) != 8 || batch.Summary.POUCount != 21 || batch.Summary.IOModuleCount != 128 || batch.Summary.AssignmentCount != 512 || batch.Summary.RepeatedAssignmentCount != 203 || batch.Summary.SkippedDuplicateCount != 0 || batch.Summary.Blocks != 0 || batch.Summary.Cards != 0 {
		t.Fatalf("unexpected complete ST map: %+v", batch.Summary)
	}
	groups := map[string]aomap.Group{}
	for _, group := range plan.Groups {
		groups[group.FCS+":"+group.POUName+"_channels"] = group
	}
	pouIDs := map[string]bool{}
	assignments := 0
	for _, file := range batch.Files {
		download := httptest.NewRecorder()
		application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
		if download.Code != http.StatusOK {
			t.Fatalf("download status=%d", download.Code)
		}
		var doc aoSTTestDocument
		if err := xml.Unmarshal(bytes.TrimPrefix(download.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}), &doc); err != nil {
			t.Fatal(err)
		}
		addresses := map[string]bool{}
		for _, pou := range doc.POUs {
			key := file.FCS + ":" + pou.Name
			group, ok := groups[key]
			if !ok || pou.IsFBD != "0" || pouIDs[pou.ID] || !strings.HasPrefix(pou.Code, "PROGRAM "+pou.Name+"\n") {
				t.Fatalf("foreign or repeated POU: %+v", pou)
			}
			delete(groups, key)
			pouIDs[pou.ID] = true
			want := map[string]int{}
			for _, module := range group.Modules {
				for _, channel := range module.Channels {
					want[fmt.Sprintf("REAL_TO_DINT(%s.OUT, %s, %s);", channel.Tag, channel.Min, channel.Max)]++
				}
			}
			for _, line := range strings.Split(pou.Code, "\n") {
				left, right, ok := strings.Cut(line, " := ")
				if !ok {
					continue
				}
				if addresses[left] || want[right] < 1 {
					t.Fatalf("invalid or repeated physical address / lost source binding: %s", line)
				}
				addresses[left] = true
				want[right]--
				assignments++
			}
			for tag, count := range want {
				if count != 0 {
					t.Errorf("%s: lost %d assignments %s", key, count, tag)
				}
			}
		}
	}
	if assignments != 512 || len(groups) != 0 {
		t.Fatalf("lost source assignments/groups: %d, %v", assignments, groups)
	}
	assertAOSTAllocatorState(t, statePath, 21)
}

func TestAOSTAPIInvalidRequestsDoNotWriteOrAllocate(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	tests := map[string]string{
		"missing": "", "null": "null", "empty": `{}`, "no POU": `{"pous":[]}`,
		"null POU list":       `{"pous":null}`,
		"null POU entry":      `{"pous":[null]}`,
		"trailing JSON":       smallAOSTConfig + " {}",
		"unknown field":       strings.Replace(smallAOSTConfig, `"moduleCount"`, `"count"`, 1),
		"unknown group":       strings.Replace(smallAOSTConfig, "FCS_MAIN:A11", "FCS_MAIN:A12", 1),
		"null ID":             strings.Replace(smallAOSTConfig, "[41]", "[null]", 1),
		"string ID":           strings.Replace(smallAOSTConfig, "[41]", `["41"]`, 1),
		"negative ID":         strings.Replace(smallAOSTConfig, "[41]", "[-1]", 1),
		"fraction ID":         strings.Replace(smallAOSTConfig, "[41]", "[1.5]", 1),
		"overflow ID":         strings.Replace(smallAOSTConfig, "[41]", "[2147483648]", 1),
		"missing IDs":         strings.Replace(smallAOSTConfig, "[41]", "[]", 1),
		"shrink below source": strings.Replace(smallAOSTConfig, `"moduleCount":1`, `"moduleCount":0`, 1),
		"oversize field":      strings.Repeat(" ", 64<<10) + smallAOSTConfig,
		"invalid later POU":   `{"pous":[{"groupKey":"FCS_MAIN:A11","moduleCount":1,"moduleIds":[41]},null]}`,
		"duplicate selection": `{"pous":[{"groupKey":"FCS_MAIN:A11","moduleCount":1,"moduleIds":[41]},{"groupKey":"FCS_MAIN:A11","moduleCount":1,"moduleIds":[42]}]}`,
	}
	for name, st := range tests {
		t.Run(name, func(t *testing.T) {
			fields := map[string]string{}
			if st != "" {
				fields["st"] = st
			}
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), fields))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
	for _, context := range []string{`{"controllerId":null}`, `{"pouNumber":"2147483648"}`} {
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), map[string]string{"st": smallAOSTConfig, "context": context}))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid context status=%d: %s", response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
	// Opt-in ST options must never change the old graphical route's behavior.
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), map[string]string{"st": smallAOSTConfig}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("FBD accepted ST options: %d", response.Code)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

func TestAOSTAPIConflictsAndSelectedPOUs(t *testing.T) {
	otherRow := strings.SplitN(strings.ReplaceAll(smallAOMap, "A11", "A12"), "\n", 2)[1]
	data := []byte(smallAOMap + otherRow)
	for _, selectedOnly := range []bool{false, true} {
		application, statePath, outputDir := aoTestApplication(t)
		st := smallAOSTConfig
		want := http.StatusCreated
		if !selectedOnly {
			st = `{"pous":[{"groupKey":"FCS_MAIN:A11","moduleCount":1,"moduleIds":[41]},{"groupKey":"FCS_MAIN:A12","moduleCount":1,"moduleIds":[41]}]}`
			want = http.StatusBadRequest
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", data, map[string]string{"st": st}))
		if response.Code != want {
			t.Fatalf("selectedOnly=%v status=%d: %s", selectedOnly, response.Code, response.Body.String())
		}
		if !selectedOnly {
			assertNoAOOutputOrState(t, statePath, outputDir)
		} else {
			assertAOSTAllocatorState(t, statePath, 1)
		}
	}
}

func TestAOSTAPISelectionRemainsIsolatedByController(t *testing.T) {
	header, _, _ := strings.Cut(smallAOMap, "\n")
	var source strings.Builder
	source.WriteString(header + "\n")
	for _, fcs := range []string{"FCS_MAIN", "FCS_OTHER", "FCS_UNUSED"} {
		for _, prefix := range []string{"A11", "A12"} {
			fmt.Fprintf(&source, "%s\tCAB\t%s-00\t0\t_IO_QU*%s-00*_0.ValueDINT := REAL_TO_DINT(_%s_%s_TAG.OUT, 0.0, 100.0);\t%s-00\t\tAO\tAN_v1\n", fcs, prefix, prefix, fcs, prefix, prefix)
		}
	}
	type selection struct {
		fcs, prefix string
		id          int64
	}
	for _, test := range []struct {
		name     string
		selected []selection
	}{
		{"one POU of one controller", []selection{{"FCS_OTHER", "A12", 41}}},
		{"all POUs of one controller", []selection{{"FCS_OTHER", "A11", 41}, {"FCS_OTHER", "A12", 42}}},
		{"same POU name and physical ID in different controllers", []selection{{"FCS_MAIN", "A11", 41}, {"FCS_OTHER", "A11", 41}}},
		{"mixed subset excludes other POUs and controller", []selection{{"FCS_OTHER", "A12", 42}, {"FCS_MAIN", "A12", 41}, {"FCS_OTHER", "A11", 41}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			request := generator.AOSTRequest{}
			wanted := map[string]map[string]selection{}
			for _, selected := range test.selected {
				id := selected.id
				request.POUs = append(request.POUs, generator.AOSTPOURequest{GroupKey: selected.fcs + ":" + selected.prefix, ModuleCount: 1, ModuleIDs: []*int64{&id}})
				if wanted[selected.fcs] == nil {
					wanted[selected.fcs] = map[string]selection{}
				}
				wanted[selected.fcs]["AO_"+selected.prefix+"_channels"] = selected
			}
			st, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(source.String()), map[string]string{"st": string(st), "fileName": "AO_selected"}))
			if response.Code != http.StatusCreated {
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			var batch aoGenerateResponse
			if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if batch.Kind != "st" || len(batch.Files) != len(wanted) || batch.Summary.POUCount != len(test.selected) || batch.Summary.AssignmentCount != 4*len(test.selected) || batch.Summary.IOModuleCount != len(test.selected) || batch.Summary.Blocks != 0 || batch.Summary.Cards != 0 {
				t.Fatalf("unexpected selected output: %+v", batch)
			}
			files, err := os.ReadDir(outputDir)
			if err != nil || len(files) != len(wanted) {
				t.Fatalf("unselected controller output written: %v, %v", files, err)
			}
			seenControllers := map[string]bool{}
			seenPOUIDs := map[string]bool{}
			for _, file := range batch.Files {
				wantedPOUs := wanted[file.FCS]
				if len(wantedPOUs) == 0 || seenControllers[file.FCS] || file.FileName != "AO_selected_ST_"+file.FCS+".xml" {
					t.Fatalf("unexpected or repeated controller file: %+v", file)
				}
				seenControllers[file.FCS] = true
				download := httptest.NewRecorder()
				application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
				if download.Code != http.StatusOK {
					t.Fatalf("download status=%d", download.Code)
				}
				var doc aoSTTestDocument
				if err := xml.Unmarshal(bytes.TrimPrefix(download.Body.Bytes(), []byte{0xEF, 0xBB, 0xBF}), &doc); err != nil {
					t.Fatal(err)
				}
				if len(doc.POUs) != len(wantedPOUs) || file.Summary.POUCount != len(wantedPOUs) {
					t.Fatalf("%s includes missing or extra POUs: %+v", file.FCS, doc.POUs)
				}
				seenNames := map[string]bool{}
				for _, pou := range doc.POUs {
					selected, ok := wantedPOUs[pou.Name]
					if !ok || seenNames[pou.Name] || seenPOUIDs[pou.ID] || pou.IsFBD != "0" {
						t.Fatalf("%s: foreign or repeated POU %+v", file.FCS, pou)
					}
					seenNames[pou.Name], seenPOUIDs[pou.ID] = true, true
					var code strings.Builder
					fmt.Fprintf(&code, "PROGRAM %s\n\n", pou.Name)
					for channel := 0; channel < 4; channel++ {
						tag := fmt.Sprintf("_%s_%s_00_%d", selected.fcs, selected.prefix, channel)
						if channel == 0 {
							tag = "_" + selected.fcs + "_" + selected.prefix + "_TAG"
						}
						fmt.Fprintf(&code, "_IO_QU%d_%d.ValueDINT := REAL_TO_DINT(%s.OUT, 0.0, 100.0);\n", selected.id, channel, tag)
					}
					code.WriteString("\nEND_PROGRAM")
					if pou.Code != code.String() {
						t.Fatalf("%s/%s: ST contains another controller's tags or unexpected assignments:\n%s", file.FCS, pou.Name, pou.Code)
					}
				}
			}
			assertAOSTAllocatorState(t, statePath, int64(len(test.selected)))
		})
	}
}

func TestAOSTAPIPersistenceFailureAndOrigin(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	request := aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), map[string]string{"st": smallAOSTConfig})
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
	application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), map[string]string{"st": smallAOSTConfig}))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("persistence failure status=%d: %s", response.Code, response.Body.String())
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}
