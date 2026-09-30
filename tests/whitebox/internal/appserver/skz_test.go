// Compatibility /api/skz adapter checks for prepared assignment ST; retired FBD requests must remain unavailable.
package appserver

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"scheme-xml-generator/internal/application/ioimport"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"strings"
	"testing"
)

// skzAPIWorkbook builds a prepared assignment XLSX for two PLCs, with explicit source headers, to exercise the
// /api/skz compatibility adapter.
func skzAPIWorkbook(t *testing.T) []byte {
	t.Helper()
	rows := [][]string{{"Loop", "SCS", "Mashalling_cabinet", "Module", "Channel", "SCS AI", "Main_module", "Redundant_module", "I/O Type", "Шаблон", "Тип объекта", "Марка", "ControllerID"}}
	for _, scs := range []string{"PLC_ONE", "PLC_TWO"} {
		for _, assignment := range []string{
			"_SENSOR_main.Xin := _IO_I*A1-00*_AI16H_0_VAL.Measurement;",
			"_SENSOR_main.Xs := QUAL_STAT(_IO_I*A1-00*_AI16H_0_VAL.Quality);",
		} {
			rows = append(rows, []string{"SENSOR", scs, "CAB", "A1-00", "0", assignment, "A1-00", "A2-00", "AI", "AD3_v2", "AD3_v2", "_SENSOR_main", ""})
		}
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, value := range row {
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t>`, rune('A'+j), i+1)
			if err := xml.EscapeText(&sheet, []byte(value)); err != nil {
				t.Fatal(err)
			}
			sheet.WriteString(`</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts := map[string]string{
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="AI" sheetId="1" r:id="r1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{"_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml"} {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

// skzAPIConfig parses the fixture workbook and serializes explicit ST group choices and physical module IDs for the
// compatibility API.
func skzAPIConfig(t *testing.T, source []byte, kind string) string {
	t.Helper()
	plan, err := ioimport.Read(source)
	if err != nil {
		t.Fatal(err)
	}
	request := stassignment.ModuleMappingRequest{Kind: kind}
	for _, group := range plan.Groups {
		id := int64(24) // Same physical ID is allowed in different PLCs.
		choice := stassignment.ModuleGroupRequest{GroupKey: group.Key}
		if kind == "st" {
			choice.ModuleIDs = []*int64{&id}
		}
		request.POUs = append(request.POUs, choice)
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSKZAPIModesAndControllerIsolation checks table preview and ST generation through /api/skz, including per-PLC
// files, context, collision-safe names and download bytes.
func TestSKZAPIModesAndControllerIsolation(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	for _, kind := range []string{"st"} {
		t.Run(kind, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			application.repository = nil
			preview := httptest.NewRecorder()
			application.Handler().ServeHTTP(preview, plcDiagnosticMultipart(t, "/api/skz/preview", "AI.xlsx", workbook, nil))
			if preview.Code != http.StatusOK {
				t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
			profile := httptest.NewRecorder()
			application.Handler().ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/skz/profile", nil))
			if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), "TENIX-CPU850") {
				t.Fatal(profile.Body.String())
			}
			fields := map[string]string{"fileName": "SKZ_test", "config": skzAPIConfig(t, workbook, kind), "context": `{"project":"SKZ test","controllerId":"88"}`}
			for attempt := 0; attempt < 2; attempt++ {
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, fields))
				if response.Code != http.StatusCreated {
					t.Fatalf("generate: %d %s", response.Code, response.Body.String())
				}
				var batch aoGenerateResponse
				if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
					t.Fatal(err)
				}
				if batch.Kind != kind || len(batch.Files) != 2 || batch.Summary.POUCount != 2 {
					t.Fatalf("batch: %+v", batch)
				}

				wantNumber := "4"

				for _, file := range batch.Files {
					if file.Summary.POUNumber != wantNumber {
						t.Fatalf("AI %s POUNum=%s, want %s", kind, file.Summary.POUNumber, wantNumber)
					}
					data, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(data, []byte(`ControllerTypeName="TENIX-CPU850"`)) || !bytes.Contains(data, []byte(`ControllerID="88"`)) {
						t.Fatal("context lost")
					}
					if !strings.Contains(file.FileName, file.ControllerName) || (attempt == 1 && !strings.HasSuffix(file.FileName, "-2.xml")) {
						t.Fatal("controller name/collision lost", file.FileName)
					}
					if kind == "st" && (!bytes.Contains(data, []byte("_IO_I24_AI16H_0_VAL.Measurement")) || bytes.Contains(data, []byte("<ISACARDSINFO"))) {
						t.Fatal("invalid ST")
					}

					download := httptest.NewRecorder()
					application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
					if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), data) {
						t.Fatal("download mismatch")
					}
				}
			}
		})
	}
}

// TestSKZAPIControllerTypesAndPhysicalProfiles generates ST for CPU715/850 and supported physical address profiles,
// checking context and emitted driver assignments.
func TestSKZAPIControllerTypesAndPhysicalProfiles(t *testing.T) {
	application, _, _ := aoTestApplication(t)
	profile := httptest.NewRecorder()
	application.Handler().ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/skz/profile", nil))
	var options struct {
		ControllerTypes         []string          `json:"controllerTypes"`
		PhysicalProfiles        []string          `json:"physicalProfiles"`
		DefaultPhysicalProfiles map[string]string `json:"defaultPhysicalProfiles"`
	}
	if err := json.Unmarshal(profile.Body.Bytes(), &options); err != nil {
		t.Fatal(err)
	}
	if profile.Code != http.StatusOK || strings.Join(options.ControllerTypes, ",") != "TENIX-CPU715,TENIX-CPU850" || strings.Join(options.PhysicalProfiles, ",") != "legacy-iu-qu,measurement-quality" || options.DefaultPhysicalProfiles[cpuprofile.ControllerCPU715] != addressing.PhysicalProfileLegacy || options.DefaultPhysicalProfiles[cpuprofile.ControllerCPU850] != addressing.PhysicalProfileMeasurement {
		t.Fatal("incomplete controller/profile options", profile.Body.String())
	}
	workbook := skzAPIWorkbook(t)
	for _, tc := range []struct {
		name, cpu, physical string
		measurement         bool
	}{
		{"715 default", cpuprofile.ControllerCPU715, "", false},
		{"850 default", cpuprofile.ControllerCPU850, "", true},
		{"715 legacy", cpuprofile.ControllerCPU715, addressing.PhysicalProfileLegacy, false},
		{"850 legacy", cpuprofile.ControllerCPU850, addressing.PhysicalProfileLegacy, false},
		{"850 measurement", cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement, true},
	} {
		for _, mode := range []string{"st"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				application, _, outputDir := aoTestApplication(t)
				response := httptest.NewRecorder()
				context, err := json.Marshal(map[string]string{"controllerTypeName": tc.cpu, "physicalProfile": tc.physical, "controllerId": "99"})
				if err != nil {
					t.Fatal(err)
				}
				application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": skzAPIConfig(t, workbook, mode), "context": string(context)}))
				if response.Code != http.StatusCreated {
					t.Fatalf("%d: %s", response.Code, response.Body.String())
				}
				var batch aoGenerateResponse
				if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
					t.Fatal(err)
				}
				if len(batch.Files) != 2 || batch.Kind != mode || batch.Summary.SignalCount != 2 {
					t.Fatalf("controller separation changed: %+v", batch)
				}
				for _, file := range batch.Files {
					data, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(data, []byte(`ControllerTypeName="`+tc.cpu+`"`)) || !bytes.Contains(data, []byte(`ControllerID="99"`)) {
						t.Fatal("selected controller context lost")
					}

					wantValue, wantStatus := "_SENSOR_main.Xin := _IO_IU24_0.ValueDINT;", "_SENSOR_main.Xs := _IO_IU24_0.Status;"
					if tc.measurement {
						wantValue, wantStatus = "_SENSOR_main.Xin := _IO_I24_AI16H_0_VAL.Measurement;", "_SENSOR_main.Xs := QUAL_STAT(_IO_I24_AI16H_0_VAL.Quality);"
					}
					if !bytes.Contains(data, []byte(wantValue)) || !bytes.Contains(data, []byte(wantStatus)) || bytes.Contains(data, []byte("QUAL_STAT")) != tc.measurement || strings.Contains(strings.Join(file.Warnings, "\n"), "QUAL_STAT") != tc.measurement {
						t.Fatalf("incorrect physical ST/profile warnings: %s, %+v", data, file.Warnings)
					}
				}
			})
		}
	}
}

// TestSKZAPIRejectsUnsupportedControllerProfileWithoutSideEffects submits unknown CPUs and incompatible ST profiles
// to the assignment endpoint; validation must precede files and allocation.
func TestSKZAPIRejectsUnsupportedControllerProfileWithoutSideEffects(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	for _, mode := range []string{"st"} {
		contexts := []string{
			`{"controllerTypeName":"TENIX-CPU999"}`,
			`{"controllerTypeName":""}`,
			`{"controllerTypeName":"TENIX-CPU715","physicalProfile":"unknown"}`,
			`{"controllerTypeName":"TENIX-CPU850","physicalProfile":"unknown"}`,
			`{"physicalProfile":null}`,
		}

		contexts = append(contexts, `{"controllerTypeName":"TENIX-CPU715","physicalProfile":"measurement-quality"}`)

		for _, context := range contexts {
			t.Run(mode+"/"+context, func(t *testing.T) {
				application, statePath, outputDir := aoTestApplication(t)
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": skzAPIConfig(t, workbook, mode), "context": context}))
				if response.Code != http.StatusBadRequest {
					t.Fatalf("accepted invalid context: %d %s", response.Code, response.Body.String())
				}
				assertNoAOOutputOrState(t, statePath, outputDir)
			})
		}
	}
}

// TestSKZAPIRejectsBeforeReservingIDs checks malformed ST configuration JSON and module IDs are rejected before the
// allocator or output directory changes.
func TestSKZAPIRejectsBeforeReservingIDs(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	valid := skzAPIConfig(t, workbook, "st")
	for _, raw := range []string{"", "null", "{}", `{"kind":"wat","pous":[]}`, strings.ReplaceAll(valid, "24", "null"), strings.ReplaceAll(valid, "24", "-1"), valid + " {}", strings.TrimSuffix(valid, "}") + `,"unknown":1}`} {
		t.Run(raw, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": raw}))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("accepted: %d %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}

// TestRetiredDOFBDRejectsContextOverrides verifies that import metadata cannot restore retired FBD.
// Both the default and an explicit group/POU destination return 410 before ID or file writes.
func TestRetiredDOFBDRejectsContextOverrides(t *testing.T) {
	workbook, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "skzmap", "do.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []string{"", `{"groupId":"100","pouNumber":"90"}`} {
		app, state, output := aoTestApplication(t)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DO.xlsx", workbook, map[string]string{"config": skzAPIConfig(t, workbook, "fbd"), "context": ctx}))
		assertRetiredSKZFBD(t, response, state, output)
	}
}

// TestSKZAPIManualModuleCount adds a reserve module through the ST API and verifies chosen IDs, all channel
// assignments and selected-controller isolation.
func TestSKZAPIManualModuleCount(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	for _, kind := range []string{"st"} {
		t.Run(kind, func(t *testing.T) {
			application, _, outputDir := aoTestApplication(t)
			config := fmt.Sprintf(`{"kind":%q,"pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":2,"moduleIds":[24,77]}]}`, kind)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": config}))
			if response.Code != http.StatusCreated {
				t.Fatalf("generate: %d %s", response.Code, response.Body.String())
			}
			var batch aoGenerateResponse
			if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
				t.Fatal(err)
			}
			if len(batch.Files) != 1 || batch.Summary.IOModuleCount != 2 || batch.Summary.SignalCount != 17 {
				t.Fatalf("sparse source and full added module: %+v", batch)
			}
			data, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("_PLC_ONE_A1_01_15")) || bytes.Contains(data, []byte("_PLC_TWO")) {
				t.Fatal("added reserve or PLC selection lost")
			}
			var document aoSTTestDocument
			if err := xml.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}

			if batch.Summary.POUCount != 1 || len(document.POUs) != 1 || document.POUs[0].Name != "AI_A1_channels" {
				t.Fatalf("ST grouping changed: %+v", document)
			}

			if !bytes.Contains(data, []byte("_PLC_ONE_A1_01_15.Xin := _IO_I77_AI16H_15_VAL.Measurement;")) || bytes.Contains(data, []byte("_IO_I24_AI16H_1_VAL")) {
				t.Fatal("manual ID or sparse source changed")
			}

		})
	}
}

// TestSKZAPIRejectsInvalidExpandedModulesWithoutSideEffects checks invalid ST expansion choices return 400 and
// retired FBD choices return 410, leaving output and state unchanged.
func TestSKZAPIRejectsInvalidExpandedModulesWithoutSideEffects(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	configs := map[string]string{}
	for _, count := range []string{"0", "-1", "1.5", `"2"`, "[]", "4097", "258"} {
		configs["count="+count] = `{"kind":"fbd","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":` + count + `}]}`
	}
	configs["missing added ID"] = `{"kind":"st","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":2,"moduleIds":[24]}]}`
	configs["blank added ID"] = `{"kind":"st","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":2,"moduleIds":[24,null]}]}`
	configs["batch channel limit"] = `{"kind":"fbd","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":130},{"groupKey":"PLC_TWO:AI:A1","moduleCount":130}]}`
	configs["expanded POU limit"] = `{"kind":"fbd","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":129}]}`
	configs["batch expanded POU limit"] = `{"kind":"fbd","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":65},{"groupKey":"PLC_TWO:AI:A1","moduleCount":64}]}`
	for name, config := range configs {
		t.Run(name, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": config}))
			var decoded stassignment.ModuleMappingRequest
			wantStatus := http.StatusBadRequest
			if json.Unmarshal([]byte(config), &decoded) == nil && decoded.Kind == "fbd" {
				wantStatus = http.StatusGone
			}
			if response.Code != wantStatus {
				t.Fatalf("accepted: %d %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}

// TestRetiredAIFBDRejectsReserveExpansion prevents reserve expansion from re-enabling a fixed FBD profile.
// The HTTP boundary rejects the decoded FBD request; historical module geometry remains covered by core tests.
func TestRetiredAIFBDRejectsReserveExpansion(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	app, state, output := aoTestApplication(t)
	response := httptest.NewRecorder()
	config := `{"kind":"fbd","pous":[{"groupKey":"PLC_ONE:AI:A1","moduleCount":2,"moduleIds":[24,77]}]}`
	app.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{"config": config}))
	assertRetiredSKZFBD(t, response, state, output)
}
