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
	"strconv"
	"strings"
	"testing"

	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/skzmap"
)

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

func skzAPIConfig(t *testing.T, source []byte, kind string) string {
	t.Helper()
	plan, err := skzmap.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	request := generator.SKZRequest{Kind: kind}
	for _, group := range plan.Groups {
		id := int64(24) // Same physical ID is allowed in different PLCs.
		choice := generator.SKZPOURequest{GroupKey: group.Key}
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

func TestSKZAPIModesAndControllerIsolation(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	for _, kind := range []string{"st", "fbd"} {
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
				if kind == "fbd" && (batch.Summary.Blocks != 14 || batch.Summary.Cards != 6 || batch.Summary.Graphics != 4) {
					t.Fatalf("AI FBD must include the complete native fragment for both PLCs: %+v", batch.Summary)
				}
				wantNumber := "4"
				if kind == "fbd" {
					wantNumber = "8"
				}
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
					if !strings.Contains(file.FileName, file.FCS) || (attempt == 1 && !strings.HasSuffix(file.FileName, "-2.xml")) {
						t.Fatal("controller name/collision lost", file.FileName)
					}
					if kind == "st" && (!bytes.Contains(data, []byte("_IO_I24_AI16H_0_VAL.Measurement")) || bytes.Contains(data, []byte("<ISACARDSINFO"))) {
						t.Fatal("invalid ST")
					}
					if kind == "fbd" && (!bytes.Contains(data, []byte(`Info="AD3_v2"`)) || bytes.Contains(data, []byte("<STCODE"))) {
						t.Fatal("invalid FBD")
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

func TestSKZAPIDONativeContextAndExplicitOverride(t *testing.T) {
	workbook, err := os.ReadFile(filepath.Join("..", "skzmap", "testdata", "do.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ context, group, number string }{
		{"", "19913", "47"},
		{`{"groupId":"100","pouNumber":"90"}`, "100", "90"},
	} {
		application, _, outputDir := aoTestApplication(t)
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "DO.xlsx", workbook, map[string]string{
			"config": skzAPIConfig(t, workbook, "fbd"), "context": tc.context,
		}))
		if response.Code != http.StatusCreated {
			t.Fatalf("%d: %s", response.Code, response.Body.String())
		}
		var batch aoGenerateResponse
		if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
		if err != nil {
			t.Fatal(err)
		}
		var document aoSTTestDocument
		if err := xml.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if len(document.POUs) != 2 || !bytes.Contains(data, []byte(`GroupID="`+tc.group+`"`)) || !bytes.Contains(data, []byte(`POUNum="`+tc.number+`"`)) {
			t.Fatalf("wrong DO context, want %s/%s", tc.group, tc.number)
		}
	}
}

func TestSKZAPIManualModuleCount(t *testing.T) {
	workbook := skzAPIWorkbook(t)
	for _, kind := range []string{"st", "fbd"} {
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
			if kind == "fbd" {
				if batch.Summary.POUCount != 2 || len(document.POUs) != 2 || document.POUs[0].Name != "AI_A1_00" || document.POUs[1].Name != "AI_A1_01" {
					t.Fatalf("each AI module must have its own FBD POU: %+v", document)
				}
			} else if batch.Summary.POUCount != 1 || len(document.POUs) != 1 || document.POUs[0].Name != "AI_A1_channels" {
				t.Fatalf("ST grouping changed: %+v", document)
			}
			if kind == "st" {
				if !bytes.Contains(data, []byte("_PLC_ONE_A1_01_15.Xin := _IO_I77_AI16H_15_VAL.Measurement;")) || bytes.Contains(data, []byte("_IO_I24_AI16H_1_VAL")) {
					t.Fatal("manual ID or sparse source changed")
				}
			} else if batch.Summary.Blocks != 119 || batch.Summary.Cards != 51 || batch.Summary.Graphics != 34 || bytes.Contains(data, []byte("_IO_I77")) {
				t.Fatalf("FBD reserve fragments: %+v", batch.Summary)
			}
		})
	}
}

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
			if response.Code != http.StatusBadRequest {
				t.Fatalf("accepted: %d %s", response.Code, response.Body.String())
			}
			assertNoAOOutputOrState(t, statePath, outputDir)
		})
	}
}

func TestSKZAPINativeAIFBDSeparatesModulesAndAddedReserve(t *testing.T) {
	workbook, err := os.ReadFile(filepath.Join("..", "skzmap", "testdata", "ai.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := skzmap.Parse(workbook)
	if err != nil {
		t.Fatal(err)
	}
	request := generator.SKZRequest{Kind: "fbd"}
	var names []string
	expected := map[string][]string{}
	for _, group := range source.Groups {
		choice := generator.SKZPOURequest{GroupKey: group.Key}
		for _, module := range group.Modules {
			name := "AI_" + strings.ReplaceAll(module.Name, "-", "_")
			names = append(names, name)
			for _, channel := range module.Channels {
				expected[name] = append(expected[name], channel.Tag)
			}
		}
		if group.Prefix == "A1" {
			count := len(group.Modules) + 1
			choice.ModuleCount = &count
			names = append(names, "AI_A1_15")
			for channel := 0; channel < 16; channel++ {
				expected["AI_A1_15"] = append(expected["AI_A1_15"], fmt.Sprintf("_%s_A1_15_%d", group.SCS, channel))
			}
		}
		request.POUs = append(request.POUs, choice)
	}
	config, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	application, _, outputDir := aoTestApplication(t)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, plcDiagnosticMultipart(t, "/api/skz/generate", "AI.xlsx", workbook, map[string]string{
		"config": string(config), "context": `{"pouNumber":"100"}`,
	}))
	if response.Code != http.StatusCreated {
		t.Fatalf("generate: %d %s", response.Code, response.Body.String())
	}
	var batch aoGenerateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Files) != 1 || batch.Summary.POUCount != 33 || batch.Summary.IOModuleCount != 33 || batch.Summary.SignalCount != 504 {
		t.Fatalf("expected one XML with 33 module POUs: %+v", batch.Summary)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, batch.Files[0].FileName))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		POUs []struct {
			ID     int64  `xml:"ID,attr"`
			Name   string `xml:"NAME,attr"`
			Number string `xml:"POUNum,attr"`
			Blocks []struct {
				Kind   string `xml:"GROBJTYPE,attr"`
				Info   string `xml:"Info,attr"`
				Params struct {
					Card string `xml:"cardId,attr"`
					Text string `xml:"T11Text,attr"`
				} `xml:"Params"`
			} `xml:"ISAGraf>Blocks>Block"`
		} `xml:"POUS>OnePOU"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.POUs) != len(names) {
		t.Fatalf("XML POU count=%d", len(doc.POUs))
	}
	for i, pou := range doc.POUs {
		if pou.Name != names[i] || pou.Number != strconv.Itoa(100+i) || pou.ID != doc.POUs[0].ID+int64(i) {
			t.Fatalf("module order/POU numbering changed: %s/%s/%d", pou.Name, pou.Number, pou.ID)
		}
		owners, tags := map[string]string{}, map[string]bool{}
		for _, block := range pou.Blocks {
			if block.Kind == "37" {
				owners[block.Params.Card] = block.Info
				tags[block.Info] = true
			}
			if block.Params.Text == ".Out" || block.Params.Text == ".Stat" {
				if owner := owners[block.Params.Card]; owner == "" || block.Info != owner+block.Params.Text {
					t.Fatalf("member binding crossed module POU in %s: %s", pou.Name, block.Info)
				}
			}
		}
		if len(tags) != len(expected[pou.Name]) || len(tags) > 16 {
			t.Fatalf("module POU %s has %d calls, want %d", pou.Name, len(tags), len(expected[pou.Name]))
		}
		for _, tag := range expected[pou.Name] {
			if !tags[tag] {
				t.Fatalf("missing tag %s in %s", tag, pou.Name)
			}
		}
	}
}
