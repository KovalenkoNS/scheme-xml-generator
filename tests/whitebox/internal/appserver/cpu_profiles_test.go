// Контракты CPU-контекста библиотечного FBD, отказа прежнего io.modules и поддержанных AO ST-сценариев.
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
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/library"
)

// cpuProfileApplication creates an isolated library-backed HTTP server with a chosen configured CPU and returns an
// actual template key plus storage paths.
func cpuProfileApplication(t *testing.T, configuredCPU string) (*Server, string, string, string) {
	t.Helper()
	application, statePath, outputDir := aoTestApplication(t)
	application.generator.Config.Common.ControllerType = configuredCPU
	application.generator.Config.Common.Project = "CPU profile test"
	application.generator.Config.Common.ControllerID = "123"
	application.generator.Config.Common.ResourceID = "456"
	application.repository = library.NewRepository(isolatedLibraryDirectory(t))
	catalog, err := application.repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	return application, statePath, outputDir, templateKeyByID(t, catalog, "17510")
}

// cpuProfileLibraryRequest builds single- or multi-POU library requests so CPU override checks cover both supported
// wire shapes.
func cpuProfileLibraryRequest(key, mode string) map[string]any {
	if mode == "legacy" {
		return map[string]any{"templateKey": key, "objectName": "_CPU_SIGNAL", "nameMode": "base", "pouName": "CPU_POU", "fileName": "cpu_profile"}
	}
	return map[string]any{"fileName": "cpu_profile", "pous": []any{
		map[string]any{"name": "CPU_POU_A", "defaultTemplateKey": key, "signals": []any{map[string]any{"objectName": "_CPU_A", "nameMode": "base"}}},
		map[string]any{"name": "CPU_POU_B", "defaultTemplateKey": key, "signals": []any{map[string]any{"objectName": "_CPU_B", "nameMode": "base"}}},
	}}
}

// cpuProfilePost serializes a library-generation payload and executes the real /api/generate handler, returning its
// recorded HTTP response.
func cpuProfilePost(t *testing.T, application *Server, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	return response
}

// cpuProfileDownload downloads generated XML and checks Common.ControllerTypeName against the request's expected
// CPU.
func cpuProfileDownload(t *testing.T, application *Server, url, cpu string) []byte {
	t.Helper()
	download := httptest.NewRecorder()
	application.Handler().ServeHTTP(download, httptest.NewRequest(http.MethodGet, url, nil))
	if download.Code != http.StatusOK {
		t.Fatalf("download status=%d: %s", download.Code, download.Body.String())
	}
	var document struct {
		Common struct {
			CPU string `xml:"ControllerTypeName,attr"`
		} `xml:"Common"`
	}
	data := bytes.TrimPrefix(download.Body.Bytes(), []byte{0xef, 0xbb, 0xbf})
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Common.CPU != cpu {
		t.Fatalf("ControllerTypeName=%q want %q", document.Common.CPU, cpu)
	}
	return data
}

// cpuProfileGenerated requires a successful generation response, decodes its metadata and downloads the CPU-checked
// XML.
func cpuProfileGenerated(t *testing.T, application *Server, response *httptest.ResponseRecorder, cpu string) (generateResponse, []byte) {
	t.Helper()
	if response.Code != http.StatusCreated {
		t.Fatalf("generate status=%d: %s", response.Code, response.Body.String())
	}
	var result generateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result, cpuProfileDownload(t, application, result.URL, cpu)
}

// TestCPUProfilesLibraryContextIsRequestScoped alternates CPU715/850 library requests and verifies XML context
// changes without mutating the server's configured defaults.
func TestCPUProfilesLibraryContextIsRequestScoped(t *testing.T) {
	for _, mode := range []string{"legacy", "multi"} {
		for _, configuredCPU := range []string{"", "TENIX-CPU715", "TENIX-CPU850", "existing-legacy-config"} {
			t.Run(mode+"/configured="+configuredCPU, func(t *testing.T) {
				application, _, _, key := cpuProfileApplication(t, configuredCPU)
				originalConfig := application.generator.Config
				for _, requestedCPU := range []string{"TENIX-CPU715", "TENIX-CPU850", " \tTENIX-CPU850\r\n"} {
					payload := cpuProfileLibraryRequest(key, mode)
					payload["context"] = map[string]any{"controllerTypeName": requestedCPU}
					result, data := cpuProfileGenerated(t, application, cpuProfilePost(t, application, payload), strings.TrimSpace(requestedCPU))
					wantPOUs := 1
					if mode == "multi" {
						wantPOUs = 2
					}
					if result.Summary.POUCount != wantPOUs || bytes.Count(data, []byte("<OnePOU ")) != wantPOUs {
						t.Fatalf("lost POU in %s request: %+v", mode, result.Summary)
					}
					for _, expected := range []string{`Project="CPU profile test"`, `ControllerID="123"`, `ResuorceID="456"`} {
						if !bytes.Contains(data, []byte(expected)) {
							t.Errorf("CPU override lost configured context %s", expected)
						}
					}
					if application.generator.Config != originalConfig {
						t.Fatal("request changed the server's generator configuration")
					}
					// A later request without context must return to the configured CPU.
					cpuProfileGenerated(t, application, cpuProfilePost(t, application, cpuProfileLibraryRequest(key, mode)), configuredCPU)
				}
				// JSON null retains the same compatibility behavior as omission.
				payload := cpuProfileLibraryRequest(key, mode)
				payload["context"] = nil
				cpuProfileGenerated(t, application, cpuProfilePost(t, application, payload), configuredCPU)
			})
		}
	}
}

// TestCPUProfilesLibraryRejectsInvalidContextWithoutSideEffects rejects invalid CPU contexts in both request shapes
// and confirms no files or allocator IDs are consumed.
func TestCPUProfilesLibraryRejectsInvalidContextWithoutSideEffects(t *testing.T) {
	contexts := []string{
		`{}`, `{"controllerTypeName":""}`, `{"controllerTypeName":"  "}`,
		`{"controllerTypeName":null}`, `{"controllerTypeName":850}`,
		`{"controllerTypeName":"850"}`, `{"controllerTypeName":"TENIX-CPU999"}`,
		`{"controllerTypeName":"tenix-cpu850"}`, `{"controllerTypeName":"TENIX-CPU850","unknown":true}`,
		`[]`, `"TENIX-CPU850"`,
	}
	for _, mode := range []string{"legacy", "multi"} {
		t.Run(mode, func(t *testing.T) {
			application, statePath, outputDir, key := cpuProfileApplication(t, "TENIX-CPU715")
			originalConfig := application.generator.Config
			for _, context := range contexts {
				payload := cpuProfileLibraryRequest(key, mode)
				payload["context"] = json.RawMessage(context)
				response := cpuProfilePost(t, application, payload)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("context=%s status=%d: %s", context, response.Code, response.Body.String())
				}
				assertNoAOOutputOrState(t, statePath, outputDir)
				if application.generator.Config != originalConfig {
					t.Fatalf("invalid context %s changed configuration", context)
				}
			}
			result, _ := cpuProfileGenerated(t, application, cpuProfilePost(t, application, cpuProfileLibraryRequest(key, mode)), "TENIX-CPU715")
			defaults := config.Default().IDs
			if result.Summary.POUID != defaults.NextPOU || result.Summary.T11First != defaults.NextT11 || result.Summary.CardFirst != defaults.NextCard {
				t.Fatalf("rejected contexts advanced in-memory allocator: %+v", result.Summary)
			}
		})
	}
}

// TestCPUProfilesRetiredPhysicalAIFailsBeforeAllocationForBothCPUs проверяет запрет io.modules для CPU715/850.
// После 410 нет файлов/состояния, а обычный библиотечный FBD получает начальные ID и сохраняет выбранную CPU.
func TestCPUProfilesRetiredPhysicalAIFailsBeforeAllocationForBothCPUs(t *testing.T) {
	for _, cpu := range []string{"TENIX-CPU715", "TENIX-CPU850"} {
		t.Run(cpu, func(t *testing.T) {
			application, statePath, outputDir, key := cpuProfileApplication(t, "")
			payload := cpuProfileRetiredIORequest(key, "AI")
			payload["context"] = map[string]any{"controllerTypeName": cpu}
			assertLegacyFBDUnavailable(t, cpuProfilePost(t, application, payload))
			assertNoAOOutputOrState(t, statePath, outputDir)
			libraryPayload := cpuProfileLibraryRequest(key, "multi")
			libraryPayload["context"] = map[string]any{"controllerTypeName": cpu}
			result, _ := cpuProfileGenerated(t, application, cpuProfilePost(t, application, libraryPayload), cpu)
			defaults := config.Default().IDs
			if result.Summary.POUID != defaults.NextPOU || result.Summary.T11First != defaults.NextT11 || result.Summary.CardFirst != defaults.NextCard || result.Summary.IOModuleCount != 0 {
				t.Fatalf("retired IO consumed IDs or changed library generation: %+v", result.Summary)
			}
		})
	}
}

// cpuProfileRetiredIORequest воспроизводит прежний JSON с явным ModuleID и библиотечным якорем.
// Тесты отправляют его в общий FBD API для проверки раннего отказа, независимо от типа IO и CPU.
func cpuProfileRetiredIORequest(key, ioType string) map[string]any {
	return map[string]any{"pous": []any{map[string]any{
		"name": "PHYSICAL_" + ioType, "defaultTemplateKey": key,
		"io": map[string]any{"type": ioType, "modules": []any{map[string]any{
			"id": 8, "signals": []any{map[string]any{"objectName": "_PHYSICAL_SIGNAL", "nameMode": "base"}},
		}}},
	}}}
}

// cpuProfilePhysicalLibrary создаёт минимальные допустимые AO/DI/DO-якоря прежнего аппаратного формата.
// HTTP-тест доказывает отказ даже при доступном шаблоне; временная библиотека не затрагивает исходники пользователя.
func cpuProfilePhysicalLibrary(t *testing.T, ioType string) (*library.Repository, string) {
	t.Helper()
	owner := library.ObjectType{ID: "500", Name: "CPU_TEST_" + ioType}
	primitive := library.Primitive{ID: "1", X: "100", Y: "50", Width: "170", Height: "20"}
	if ioType == "AO" {
		primitive.ObjectType, primitive.CardID, primitive.ISAObjectID = "36", "0", "791"
		primitive.Params, primitive.TypeName, primitive.LibraryName = "[TEXT]=\n[CI]=3\n[CO]=1\n[COMMENT]=false", "REAL_TO_DINT", "TACSfbl"
	} else {
		initial := "FALSE"
		owner.ISAObjects.Items = []library.ISAObject{{ID: "501", InitialValue: &initial, TypeName: "BOOL", LibraryName: "Системная"}}
		primitive.ObjectType, primitive.CardID, primitive.ISAObjectID = "31", "501", "-9"
		primitive.Params = "[TEXT]=\n[CI]=1\n[CO]=1\n[COMMENT]=false"
	}
	owner.Templates.Items = []library.Template{{ID: "502", Name: "CPU_TEST_" + ioType, Width: "400", Height: "200", Background: "16777215", Contents: library.Contents{Primitives: []library.Primitive{primitive}}}}
	document := library.Document{Version: library.Version{Value: "29"}, Sections: []library.Section{{Number: "1", Other: library.Other{ObjectTypes: []library.ObjectType{owner}}}}}
	data, err := xml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	directory := isolatedHTTPTemp(t)
	if err := os.WriteFile(filepath.Join(directory, "CPU-test.xml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(directory)
	catalog, err := repository.Refresh()
	if err != nil || len(catalog.Errors) != 0 {
		t.Fatalf("load CPU test library: %v, %+v", err, catalog.Errors)
	}
	return repository, templateKeyByID(t, catalog, "502")
}

// TestCPUProfilesRetiredPhysicalBindingsIgnoreCPUOverride проверяет отказ AO/DI/DO io.modules для обеих CPU.
// Настройка сервера и явное переопределение не возвращают отключённый FBD; allocator и output остаются пустыми.
func TestCPUProfilesRetiredPhysicalBindingsIgnoreCPUOverride(t *testing.T) {
	for _, ioType := range []string{"AO", "DI", "DO"} {
		t.Run(ioType, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			repository, key := cpuProfilePhysicalLibrary(t, ioType)
			application.repository = repository
			for _, configuredCPU := range []string{"TENIX-CPU715", "TENIX-CPU850"} {
				application.generator.Config.Common.ControllerType = configuredCPU
				for _, requestedCPU := range []string{"", "TENIX-CPU715", "TENIX-CPU850"} {
					payload := cpuProfileRetiredIORequest(key, ioType)
					if requestedCPU != "" {
						payload["context"] = map[string]any{"controllerTypeName": requestedCPU}
					}
					assertLegacyFBDUnavailable(t, cpuProfilePost(t, application, payload))
					assertNoAOOutputOrState(t, statePath, outputDir)
				}
			}
		})
	}
}

// cpuProfileAOBatch decodes a successful AO-ST batch response and requires exactly one generated controller file.
func cpuProfileAOBatch(t *testing.T, response *httptest.ResponseRecorder) aoGenerateResponse {
	t.Helper()
	if response.Code != http.StatusCreated {
		t.Fatalf("AO generation status=%d: %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 1 {
		t.Fatalf("expected one AO controller file: %+v", batch)
	}
	return batch
}

// TestRetiredAOFBDRejectsBothCPUProfiles проверяет единый запрет встроенного AO FBD для обеих моделей.
// Явный CPU715/CPU850 и прежний default возвращают 410, не меняя текущие файлы и ID.
func TestRetiredAOFBDRejectsBothCPUProfiles(t *testing.T) {
	application, statePath, outputDir := aoTestApplication(t)
	for _, cpu := range []string{"TENIX-CPU715", "TENIX-CPU850", " TENIX-CPU850 ", ""} {
		fields := map[string]string{}
		if cpu != "" {
			fields["context"] = fmt.Sprintf(`{"controllerTypeName":%q}`, cpu)
		}
		before := cpuProfileOutputSnapshot(t, statePath, outputDir)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate", []byte(smallAOMap), fields))
		assertLegacyFBDUnavailable(t, response)
		if after := cpuProfileOutputSnapshot(t, statePath, outputDir); !reflect.DeepEqual(before, after) {
			t.Fatal("blocked AO FBD changed state or files")
		}
	}
}

// cpuProfileOutputSnapshot reads current generated files and allocator state into a byte-content snapshot for
// rejection-side-effect checks.
func cpuProfileOutputSnapshot(t *testing.T, statePath, outputDir string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if data, err := os.ReadFile(statePath); err == nil {
		result["state.json"] = string(data)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result["output/"+entry.Name()] = string(data)
	}
	return result
}

// TestCPUProfilesAOST850RejectionPreservesStateAndOutputs checks the unconfirmed CPU850 AO-ST export profile is
// rejected without changing files or IDs; the reason must not identify CPU850 as PAZ.
func TestCPUProfilesAOST850RejectionPreservesStateAndOutputs(t *testing.T) {
	for _, withExistingOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%v", withExistingOutput), func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			generate := func(cpu string) *httptest.ResponseRecorder {
				fields := map[string]string{"st": smallAOSTConfig, "fileName": "AO_CPU"}
				if cpu != "" {
					fields["context"] = fmt.Sprintf(`{"controllerTypeName":%q}`, cpu)
				}
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/generate-st", []byte(smallAOMap), fields))
				return response
			}
			createdPOUs := int64(0)
			if withExistingOutput {
				cpuProfileAOBatch(t, generate("TENIX-CPU715"))
				createdPOUs++
			}
			before := cpuProfileOutputSnapshot(t, statePath, outputDir)
			response := generate("TENIX-CPU850")
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "850") || !strings.Contains(response.Body.String(), "не подтверждён") || strings.Contains(response.Body.String(), "ПАЗ") || strings.Contains(response.Body.String(), "нет AO") {
				t.Fatalf("AO ST 850 status=%d: %s", response.Code, response.Body.String())
			}
			if after := cpuProfileOutputSnapshot(t, statePath, outputDir); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected AO ST 850 request changed state or output files")
			}
			batch := cpuProfileAOBatch(t, generate(""))
			data := cpuProfileDownload(t, application, batch.Files[0].URL, "TENIX-CPU715")
			if batch.Files[0].Summary.POUs[0].POUID != config.Default().IDs.NextPOU+createdPOUs || !bytes.Contains(data, []byte("_IO_QU41_0.ValueDINT := REAL_TO_DINT(_TEST_AO.OUT, 0.0, 100.0);")) {
				t.Fatalf("AO ST 850 rejection advanced IDs or changed default 715 behavior: %+v", batch)
			}
			assertAOSTAllocatorState(t, statePath, createdPOUs+1)
		})
	}
}

// TestCPUProfilesAORejectsInvalidControllerTypes checks invalid CPU values produce 400 for AO ST and 410 for
// retired AO FBD, without writes.
func TestCPUProfilesAORejectsInvalidControllerTypes(t *testing.T) {
	for _, route := range []string{"generate", "generate-st"} {
		t.Run(route, func(t *testing.T) {
			application, statePath, outputDir := aoTestApplication(t)
			for _, context := range []string{`{"controllerTypeName":""}`, `{"controllerTypeName":" "}`, `{"controllerTypeName":"TENIX-CPU999"}`, `{"controllerTypeName":"850"}`, `{"controllerTypeName":850}`, `{"controllerTypeName":null}`} {
				fields := map[string]string{"context": context}
				if route == "generate-st" {
					fields["st"] = smallAOSTConfig
				}
				response := httptest.NewRecorder()
				application.Handler().ServeHTTP(response, aoMultipartRequest(t, "/api/temporary/ao/"+route, []byte(smallAOMap), fields))
				want := http.StatusBadRequest
				if route == "generate" {
					want = http.StatusGone
				}
				if response.Code != want {
					t.Fatalf("context=%s status=%d: %s", context, response.Code, response.Body.String())
				}
				assertNoAOOutputOrState(t, statePath, outputDir)
			}
		})
	}
}
