// Raw DI/DO workbook ST HTTP checks, including complete physical channel coverage and controller isolation.
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
	"scheme-xml-generator/internal/application/ioimport"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"strings"
	"testing"
)

// TestSKZRawDOAndMixedWorkbookAPI generates DO-only and mixed DI/DO ST batches from the raw workbook, checking all
// physical channels, controller isolation and downloads.
func TestSKZRawDOAndMixedWorkbookAPI(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	source, err := ioimport.Read(workbook)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []struct {
		kind                                      string
		pous, modules, signals, blocks, cards, st int
	}{
		{"DO", 12, 72, 1948, 2020, 559, 2304},
		{"", 20, 122, 3184, 3306, 1227, 3954},
	} {
		for _, mode := range []string{"st"} {
			t.Run(selection.kind+"/"+mode, func(t *testing.T) {
				application, _, outputDir := aoTestApplication(t)
				application.repository = nil
				request := skzRawRequest(t, workbook, mode, selection.kind, "")
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "PS_IO_LIST_SCS_v4_new.xlsx", workbook,
					map[string]string{"fileName": "raw_IO", "config": skzDIJSON(t, request)}))
				if response.Code != http.StatusCreated {
					t.Fatalf("generate %d: %s", response.Code, response.Body.String())
				}
				var batch aoGenerateResponse
				if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
					t.Fatal(err)
				}
				if len(batch.Files) != 3 || batch.Kind != mode || batch.Summary.POUCount != selection.pous || batch.Summary.IOModuleCount != selection.modules || batch.Summary.SignalCount != selection.signals {
					t.Fatalf("raw workbook totals: %+v", batch.Summary)
				}

				if mode == "st" && (batch.Summary.AssignmentCount != selection.st || batch.Summary.RepeatedAssignmentCount != 1461) {
					t.Fatalf("ST repetitions must be retained: %+v", batch.Summary)
				}
				for _, file := range batch.Files {
					data, err := os.ReadFile(filepath.Join(outputDir, file.FileName))
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(file.FileName, file.ControllerName) {
						t.Fatal("PLC identity missing from filename", file.FileName)
					}
					for _, other := range batch.Files {
						if other.ControllerName != file.ControllerName && bytes.Contains(data, []byte("_"+other.ControllerName+"_A")) {
							t.Fatal("foreign PLC module in document", file.ControllerName, other.ControllerName)
						}
					}
					if selection.kind == "DO" && (file.Summary.POUNumber != "47" || file.Summary.POUGroupID != "19913" || bytes.Contains(data, []byte(`NAME="DI_`))) {
						t.Fatal("DO-only selection or native context lost", file.Summary)
					}

					assertSKZRawST(t, data, source, request, file.ControllerName, addressing.PhysicalProfileMeasurement)

					download := httptest.NewRecorder()
					application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, file.URL, nil))
					if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), data) {
						t.Fatal("raw workbook download mismatch")
					}
				}
			})
		}
	}
}

// assertSKZRawST compares downloaded ST against independently parsed DI/DO modules and selected physical IDs,
// requiring every assignment exactly once.
func assertSKZRawST(t *testing.T, data []byte, source *assignments.Plan, request stassignment.ModuleMappingRequest, scs, profile string) {
	t.Helper()
	choices := map[string]stassignment.ModuleGroupRequest{}
	for _, choice := range request.POUs {
		choices[choice.GroupKey] = choice
	}
	want := map[string]bool{}
	for _, group := range source.Groups {
		choice, selected := choices[group.Key]
		if !selected || group.ControllerName != scs {
			continue
		}
		for i, module := range group.Modules {
			tag := "_" + scs + "_" + strings.ReplaceAll(module.Name, "-", "_")
			id := *choice.ModuleIDs[i]
			if group.Kind == "DI" {
				want[fmt.Sprintf("%s.Stat := ANY_TO_DWORD (QUAL_STAT(_IO_I%d_DI32_0_VAL.Quality));", tag, id)] = true
				for channel := 0; channel < 32; channel++ {
					want[fmt.Sprintf("%s.i%02d := _IO_I%d_DI32_%d_VAL.Measurement;", tag, channel, id, channel)] = true
				}
			} else {
				for channel := 0; channel < 32; channel++ {
					line := fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := %s._%02d;", id, channel, tag, channel)
					if profile == addressing.PhysicalProfileLegacy {
						line = fmt.Sprintf("_IO_QU%d_%d.Value := %s._%02d;", id, channel, tag, channel)
					}
					want[line] = true
				}
			}
		}
	}
	var document aoSTTestDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, pou := range document.POUs {
		for _, line := range strings.Split(pou.Code, "\n") {
			if !strings.Contains(line, ":=") {
				continue
			}
			line = strings.TrimSpace(line)
			if !want[line] {
				t.Fatal("unexpected or repeated physical assignment", line)
			}
			delete(want, line)
		}
	}
	if len(want) != 0 || bytes.Contains(data, []byte("<ISACARDSINFO")) {
		t.Fatal("missing ST assignments or graphical metadata", len(want))
	}
}

// TestRawDOSparseSourceGeneratesFullChannelST checks a 28-signal DO source still emits all 32 physical ST outputs for
// both CPU address profiles, without inventing source rows.
func TestRawDOSparseSourceGeneratesFullChannelST(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	source, err := ioimport.Read(workbook)
	if err != nil {
		t.Fatal(err)
	}
	const scs = "3000_S_SC_B03"
	var targetGroup string
	targetIndex := -1
	for _, group := range source.Groups {
		if group.ControllerName != scs || group.Kind != "DO" {
			continue
		}
		for index, module := range group.Modules {
			if module.Name != "A33-08" {
				continue
			}
			if len(module.Channels) != 28 {
				t.Fatalf("A33-08 source must remain sparse: %d channels", len(module.Channels))
			}
			for channel, mapped := range module.Channels {
				if mapped.Channel != channel {
					t.Fatalf("A33-08 source position %d = %d", channel, mapped.Channel)
				}
			}
			targetGroup, targetIndex = group.Key, index
		}
	}
	if targetIndex < 0 {
		t.Fatal("A33-08 missing from raw source")
	}
	all := skzRawRequest(t, workbook, "st", "DO", "")
	var choices []stassignment.ModuleGroupRequest
	var targetID int64
	for _, choice := range all.POUs {
		if strings.HasPrefix(choice.GroupKey, scs+":") {
			choices = append(choices, choice)
		}
		if choice.GroupKey == targetGroup {
			targetID = *choice.ModuleIDs[targetIndex]
		}
	}
	for _, tc := range []struct{ cpu, profile string }{
		{cpuprofile.ControllerCPU715, addressing.PhysicalProfileLegacy},
		{cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement},
	} {
		for _, mode := range []string{"st"} {
			t.Run(tc.cpu+"/"+mode, func(t *testing.T) {
				application, _, outputDir := aoTestApplication(t)
				application.repository = nil
				request := stassignment.ModuleMappingRequest{Kind: mode, POUs: choices}
				context, err := json.Marshal(map[string]string{"controllerTypeName": tc.cpu, "physicalProfile": tc.profile})
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "IO.xlsx", workbook,
					map[string]string{"config": skzDIJSON(t, request), "context": string(context)}))
				if response.Code != http.StatusCreated {
					t.Fatalf("generate: %d %s", response.Code, response.Body.String())
				}
				var batch aoGenerateResponse
				if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
					t.Fatal(err)
				}
				if len(batch.Files) != 1 || batch.Files[0].ControllerName != scs || batch.Summary.IOModuleCount != 16 || batch.Summary.SignalCount != 444 {
					t.Fatalf("B03 source composition changed: %+v", batch)
				}
				data, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
				if err != nil {
					t.Fatal(err)
				}

				if batch.Summary.AssignmentCount != 512 || batch.Summary.RepeatedAssignmentCount != 333 {
					t.Fatalf("B03 must bind all 16 x 32 outputs while retaining logical repetition count: %+v", batch.Summary)
				}
				assertSKZRawST(t, data, source, request, scs, tc.profile)
				for channel := 28; channel < 32; channel++ {
					line := fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := _%s_A33_08._%02d;", targetID, channel, scs, channel)
					if tc.profile == addressing.PhysicalProfileLegacy {
						line = fmt.Sprintf("_IO_QU%d_%d.Value := _%s_A33_08._%02d;", targetID, channel, scs, channel)
					}
					if bytes.Count(data, []byte(line)) != 1 {
						t.Fatal("missing or repeated A33-08 tail output", line)
					}
				}

			})
		}
	}
}

// TestSKZRawDOOnlySupports715WithoutDIProfileRestriction selects one DO group for CPU715 and checks legacy physical
// ST addresses and explicit POU context without DI leakage.
func TestSKZRawDOOnlySupports715WithoutDIProfileRestriction(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	for _, mode := range []string{"st"} {
		application, _, outputDir := aoTestApplication(t)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "IO.xlsx", workbook, map[string]string{
			"config":  skzDIJSON(t, skzRawRequest(t, workbook, mode, "DO", "DO_A11")),
			"context": `{"controllerTypeName":"TENIX-CPU715","pouNumber":"150"}`,
		}))
		if response.Code != http.StatusCreated {
			t.Fatalf("DO-only %s: %d %s", mode, response.Code, response.Body.String())
		}
		var batch aoGenerateResponse
		if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
			t.Fatal(err)
		}
		if len(batch.Files) != 1 || batch.Summary.POUCount != 1 || batch.Summary.IOModuleCount != 2 || batch.Summary.SignalCount != 60 {
			t.Fatal("DO selection leaked other groups", batch.Summary)
		}
		data, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte(`POUNum="150"`)) || !bytes.Contains(data, []byte(`ControllerTypeName="TENIX-CPU715"`)) || bytes.Contains(data, []byte(`NAME="DI_`)) {
			t.Fatal("DO-only selection or manual context lost")
		}
		if mode == "st" && (!bytes.Contains(data, []byte("_IO_QU0_0.Value := _3000_S_SC_B01_A11_14._00;")) || bytes.Contains(data, []byte(".Measurement"))) {
			t.Fatal("DO715 must use legacy physical addresses")
		}
	}
}

// TestSKZRawMixedRejectsConflictsWithoutSideEffects rejects mixed DI/DO ST ID collisions or unsupported CPU
// profiles; retired FBD expansion requests remain 410 without writes.
func TestSKZRawMixedRejectsConflictsWithoutSideEffects(t *testing.T) {
	workbook := skzDIRawWorkbook(t)
	for _, name := range []string{"DI extension into DO", "DO extension into DI", "duplicate ID across kinds", "mixed DI ST715"} {
		t.Run(name, func(t *testing.T) {
			request := skzRawRequest(t, workbook, "st", "", "")
			context := ""
			wantError := ""
			switch name {
			case "DI extension into DO", "DO extension into DI":
				wantError = "уже существует в другой POU"
				kind, only, count := "DI", "DI_A11", 10
				if name == "DO extension into DI" {
					kind, only, count = "DO", "DO_A70", 13
				}
				request = skzRawRequest(t, workbook, "fbd", kind, only)
				request.POUs[0].ModuleCount = &count
			case "duplicate ID across kinds":
				wantError = "уникален в ПЛК"
				for i := range request.POUs {
					if request.POUs[i].GroupKey == "3000_S_SC_B01:DO:A11" {
						id := int64(0)
						request.POUs[i].ModuleIDs[0] = &id
					}
				}
			case "mixed DI ST715":
				wantError = "DI ST"
				context = `{"controllerTypeName":"TENIX-CPU715"}`
			}
			application, statePath, outputDir := aoTestApplication(t)
			response := httptest.NewRecorder()
			application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "IO.xlsx", workbook,
				map[string]string{"config": skzDIJSON(t, request), "context": context}))
			if request.Kind == "fbd" {
				assertRetiredSKZFBD(t, response, statePath, outputDir)
				return
			}
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), wantError) {
				t.Fatalf("accepted conflict: %d %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}
