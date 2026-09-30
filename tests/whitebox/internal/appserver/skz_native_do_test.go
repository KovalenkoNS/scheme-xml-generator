// Prepared DO workbook ST generation and rejection of the retired native fixed-FBD HTTP profile.
package appserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/application/ioimport"
	"scheme-xml-generator/internal/generator/addressing"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"testing"
)

// nativeDOAPIInput reads the prepared DO fixture, parses its source plan and builds a legacy-profile payload used
// to test the retired HTTP route.
func nativeDOAPIInput(t *testing.T) ([]byte, *assignments.Plan, stassignment.ModuleMappingRequest) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "skzmap", "do_native.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := ioimport.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	request := skzRawRequest(t, data, "st", "DO", "")
	request.Kind, request.DOFBDProfile = "fbd", "native850"
	return data, source, request
}

// TestPreparedDOPreviewAndRetiredFBDProfile keeps native workbook preview available while closing its hardcoded renderer.
// The public profile advertises no native FBD choices, and generation returns 410 without reserving IDs.
func TestPreparedDOPreviewAndRetiredFBDProfile(t *testing.T) {
	data, _, request := nativeDOAPIInput(t)
	app, state, output := aoTestApplication(t)
	profileResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(profileResponse, httptest.NewRequest(http.MethodGet, "/api/skz/profile", nil))
	var profile struct {
		Available bool     `json:"fbdAvailable"`
		Profiles  []string `json:"doFBDProfiles"`
	}
	if err := json.Unmarshal(profileResponse.Body.Bytes(), &profile); err != nil || profileResponse.Code != http.StatusOK || profile.Available || len(profile.Profiles) != 0 {
		t.Fatal(profileResponse.Body.String(), err)
	}
	preview := httptest.NewRecorder()
	app.Handler().ServeHTTP(preview, plcDiagnosticMultipart(t, "/api/skz/preview", "DO_native.xlsx", data, nil))
	if preview.Code != http.StatusOK {
		t.Fatal(preview.Body.String())
	}
	assertNoAOOutputOrState(t, state, output)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DO_native.xlsx", data, map[string]string{"config": skzDIJSON(t, request)}))
	assertRetiredSKZFBD(t, response, state, output)
}

// TestPreparedDOBookGeneratesFullST generates ST from the prepared DO workbook and checks all
// 32 channels per module without FBD graph metadata.
func TestPreparedDOBookGeneratesFullST(t *testing.T) {
	data, source, _ := nativeDOAPIInput(t)
	for _, mode := range []string{"st"} {
		t.Run(mode, func(t *testing.T) {
			request := skzRawRequest(t, data, mode, "DO", "")
			application, _, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DO.xlsx", data, map[string]string{"config": skzDIJSON(t, request)}))
			if response.Code != http.StatusCreated {
				t.Fatalf("%s: %d %s", mode, response.Code, response.Body.String())
			}
			var batch aoGenerateResponse
			if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if len(batch.Files) != 2 || batch.Summary.SignalCount != 1692 || batch.Summary.IOModuleCount != 60 {
				t.Fatal("prepared totals", batch.Summary)
			}
			if mode == "st" && batch.Summary.AssignmentCount != 1920 {
				t.Fatal("not all 32 channels in ST", batch.Summary)
			}

			for _, file := range batch.Files {
				xmlData, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(xmlData, []byte(`Info="NOT"`)) || bytes.Contains(xmlData, []byte("VAL_DIAG")) {
					t.Fatal("FBD graph metadata leaked into ST")
				}

				assertSKZRawST(t, xmlData, source, request, file.ControllerName, addressing.PhysicalProfileMeasurement)

			}
		})
	}
}

// TestSKZNativeDOAPIRejectsInvalidProfilesWithoutWrites checks legacy DO-FBD options cannot bypass 410 and invalid
// ST profile combinations fail before output or allocation.
func TestSKZNativeDOAPIRejectsInvalidProfilesWithoutWrites(t *testing.T) {
	data, _, base := nativeDOAPIInput(t)
	for _, scenario := range []string{"missing ID", "duplicate ID", "CPU715", "legacy", "unknown profile", "ST profile"} {
		t.Run(scenario, func(t *testing.T) {
			request := base
			request.POUs = append([]stassignment.ModuleGroupRequest(nil), base.POUs...)
			request.POUs[0].ModuleIDs = append([]*int64(nil), base.POUs[0].ModuleIDs...)
			context := ""
			switch scenario {
			case "missing ID":
				request.POUs[0].ModuleIDs[0] = nil
			case "duplicate ID":
				request.POUs[0].ModuleIDs[1] = request.POUs[0].ModuleIDs[0]
			case "CPU715":
				context = `{"controllerTypeName":"TENIX-CPU715"}`
			case "legacy":
				context = `{"physicalProfile":"legacy-iu-qu"}`
			case "unknown profile":
				request.DOFBDProfile = "typo"
			case "ST profile":
				request.Kind = "st"
			}
			application, statePath, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DO.xlsx", data, map[string]string{"config": skzDIJSON(t, request), "context": context}))
			if request.Kind == "fbd" {
				assertRetiredSKZFBD(t, response, statePath, outputDir)
				return
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("accepted %s: %d %s", scenario, response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}
