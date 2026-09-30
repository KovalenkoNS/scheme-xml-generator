// Raw DI workbook HTTP preview/ST export and explicit rejection of unsupported profiles.
package appserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	programcontext "scheme-xml-generator/internal/generator/program"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"strings"
	"testing"
)

// skzDIRawWorkbook reads the fixed raw DI/DO XLSX fixture used by compatibility-API tests; absence is an error.
func skzDIRawWorkbook(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "skzmap", "di.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// skzDIRequest selects DI groups from the parsed workbook and delegates construction of explicit per-controller
// module IDs.
func skzDIRequest(t *testing.T, workbook []byte, mode, only string) stassignment.ModuleMappingRequest {
	t.Helper()
	return skzRawRequest(t, workbook, mode, "DI", only)
}

// skzRawRequest parses an assignment workbook and builds selected kind/POU choices, numbering ST module IDs
// independently in each PLC.
func skzRawRequest(t *testing.T, workbook []byte, mode, kind, only string) stassignment.ModuleMappingRequest {
	t.Helper()
	plan, err := assignments.Parse(workbook)
	if err != nil {
		t.Fatal(err)
	}
	request := stassignment.ModuleMappingRequest{Kind: mode}
	nextID := map[string]int64{}
	for _, group := range plan.Groups {
		if kind != "" && group.Kind != kind {
			continue
		}
		if only != "" && group.POUName != only {
			continue
		}
		choice := stassignment.ModuleGroupRequest{GroupKey: group.Key}
		if mode == "st" {
			for range group.Modules {
				id := nextID[group.ControllerName]
				nextID[group.ControllerName]++
				choice.ModuleIDs = append(choice.ModuleIDs, &id)
			}
		}
		request.POUs = append(request.POUs, choice)
	}
	return request
}

// skzDIJSON serializes a module-mapping request for the multipart compatibility API and fails the test on encoding
// errors.
func skzDIJSON(t *testing.T, request stassignment.ModuleMappingRequest) string {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSKZDIRawWorkbookAPI checks raw DI preview and full-channel ST export, including per-PLC output, native
// context and persisted download bytes.
func TestSKZDIRawWorkbookAPI(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	for _, mode := range []string{"st"} {
		t.Run(mode, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			application.repository = nil // This workflow does not require library files.
			preview := httptest.NewRecorder()
			application.Handler().ServeHTTP(preview, plcDiagnosticMultipart(t, "/api/skz/preview", "PS_IO_LIST_SCS_v4_new.xlsx", workbook, nil))
			if preview.Code != http.StatusOK {
				t.Fatalf("preview %d: %s", preview.Code, preview.Body.String())
			}
			var plan assignments.Plan
			if err := json.Unmarshal(preview.Body.Bytes(), &plan); err != nil || len(plan.Groups) != 20 {
				t.Fatalf("unexpected mixed DI/DO preview: %v, %d groups", err, len(plan.Groups))
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "PS_IO_LIST_SCS_v4_new.xlsx", workbook,
				map[string]string{"fileName": "DI_test", "config": skzDIJSON(t, skzDIRequest(t, workbook, mode, ""))}))
			if response.Code != http.StatusCreated {
				t.Fatalf("generate %d: %s", response.Code, response.Body.String())
			}
			var batch aoGenerateResponse
			if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if len(batch.Files) != 3 || batch.Kind != mode || batch.Summary.POUCount != 8 || batch.Summary.IOModuleCount != 50 || batch.Summary.SignalCount != 1236 {
				t.Fatalf("DI totals: %+v", batch.Summary)
			}

			if mode == "st" && (batch.Summary.AssignmentCount != 1650 || batch.Summary.RepeatedAssignmentCount != 0) {
				t.Fatalf("DI ST totals: %+v", batch.Summary)
			}
			for _, file := range batch.Files {
				data, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
				if err != nil {
					t.Fatal(err)
				}
				wantNumber := "23"

				if file.Summary.POUNumber != wantNumber || file.Summary.POUGroupID != "19814" || !strings.Contains(file.FileName, file.ControllerName) {
					t.Fatal("native DI context or file PLC lost", file.FileName, file.Summary)
				}
				for _, value := range []string{`ControllerTypeName="TENIX-CPU850"`, `ControllerID="189312"`, `ResuorceID="644"`} {
					if !bytes.Contains(data, []byte(value)) {
						t.Fatal("DI default context lost", value)
					}
				}
				for _, other := range batch.Files {
					if other.ControllerName != file.ControllerName && bytes.Contains(data, []byte("_"+other.ControllerName+"_A")) {
						t.Fatal("different PLC module leaked into document", file.ControllerName, other.ControllerName)
					}
				}

				if bytes.Count(data, []byte(":=")) != file.Summary.IOModuleCount*33 || !bytes.Contains(data, []byte("_IO_I0_DI32_31_VAL.Measurement")) || bytes.Contains(data, []byte("<ISACARDSINFO")) {
					t.Fatal("incorrect full-channel ST or physical zero ID")
				}

				download := httptest.NewRecorder()
				application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
				if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), data) {
					t.Fatal("DI download mismatch")
				}
			}
		})
	}
}

// TestSKZDISelectionAndRejectedProfiles checks unsupported DI-ST CPU/address profiles are rejected before writes
// and DI fixed-FBD requests remain unavailable.
func TestSKZDISelectionAndRejectedProfiles(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	for _, context := range []string{`{"controllerTypeName":"TENIX-CPU715"}`, `{"physicalProfile":"legacy-iu-qu"}`} {
		application, statePath, outputDir := aoTestApplication(t)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DI.xlsx", workbook,
			map[string]string{"config": skzDIJSON(t, skzDIRequest(t, workbook, "st", "DI_A12")), "context": context}))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "DI ST") {
			t.Fatalf("DI unsupported profile: %d %s", response.Code, response.Body.String())
		}
		assertNoAOOutputOrState(t, statePath, outputDir)
	}
	app, state, output := aoTestApplication(t)
	response := httptest.NewRecorder()
	request := skzDIRequest(t, workbook, "fbd", "DI_A12")
	app.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DI.xlsx", workbook, map[string]string{"config": skzDIJSON(t, request), "context": `{"controllerTypeName":"TENIX-CPU715","pouNumber":"101","controllerId":"42"}`}))
	assertRetiredSKZFBD(t, response, state, output)
}

// TestSKZDINativeProfileDefaults checks the HTTP profile exposes DI-ST defaults while advertising no retired
// fixed-FBD contexts.
func TestSKZDINativeProfileDefaults(t *testing.T) {
	application, _, _ := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/skz/profile", nil))
	var profile struct {
		ST  map[string]programcontext.ProgramContext `json:"contexts"`
		FBD map[string]programcontext.ProgramContext `json:"fbdContexts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	for _, contexts := range []map[string]programcontext.ProgramContext{profile.ST} {
		ctx := contexts["DI"]
		if ctx.ControllerID != "189312" || ctx.ResourceID != "644" || ctx.GroupID != "19814" || ctx.ControllerTypeName != cpuprofile.ControllerCPU850 {
			t.Fatal("DI profile default missing", ctx)
		}
	}
	if profile.ST["DI"].POUNumber != "23" || len(profile.FBD) != 0 {
		t.Fatal("ST defaults changed or retired FBD profile is still advertised")
	}
}
