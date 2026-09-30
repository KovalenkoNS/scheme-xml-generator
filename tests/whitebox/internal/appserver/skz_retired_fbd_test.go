// Retired SKZ FBD routes must not bypass the library-only generation boundary.
package appserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"strings"
	"testing"
)

// assertRetiredSKZFBD checks the user-facing refusal and allocator/output invariants.
// A retired request must be an explicit 410 with library guidance and no partial file.
func assertRetiredSKZFBD(t *testing.T, response *httptest.ResponseRecorder, statePath, outputDir string) {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusGone || !strings.Contains(body.Error, "библиотеки") {
		t.Fatalf("retired FBD response: %d %s, decode=%v", response.Code, response.Body.String(), err)
	}
	assertNoAOOutputOrState(t, statePath, outputDir)
}

// TestSKZRetiredFBDCannotBeRestoredByCPUProfileOrWorkbook exercises former entry variants.
// Valid AI/DI/DO/native workbooks, both CPUs and legacy mode names all stop before generation.
func TestSKZRetiredFBDCannotBeRestoredByCPUProfileOrWorkbook(t *testing.T) {
	raw := skzDIRawWorkbook(t)
	native, _, nativeRequest := nativeDOAPIInput(t)
	for _, scenario := range []struct {
		name    string
		data    []byte
		request stassignment.ModuleMappingRequest
	}{
		{"AI", skzAPIWorkbook(t), stassignment.ModuleMappingRequest{POUs: []stassignment.ModuleGroupRequest{{GroupKey: "PLC_ONE:AI:A1"}}}},
		{"DI", raw, skzDIRequest(t, raw, "fbd", "")},
		{"DO", raw, skzRawRequest(t, raw, "fbd", "DO", "")},
		{"native DO", native, nativeRequest},
		{"unparsed workbook", []byte("not a workbook"), stassignment.ModuleMappingRequest{}},
	} {
		for _, cpu := range []string{cpuprofile.ControllerCPU715, cpuprofile.ControllerCPU850} {
			for _, mode := range []string{"fbd", "fbd-native-do"} {
				t.Run(scenario.name+"/"+cpu+"/"+mode, func(t *testing.T) {
					request := scenario.request
					request.Kind = mode
					application, state, output := aoTestApplication(t)
					response := httptest.NewRecorder()
					application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "source.xlsx", scenario.data, map[string]string{
						"config": skzDIJSON(t, request), "context": `{"controllerTypeName":"` + cpu + `","physicalProfile":"legacy-iu-qu"}`,
					}))
					assertRetiredSKZFBD(t, response, state, output)
				})
			}
		}
	}
}
