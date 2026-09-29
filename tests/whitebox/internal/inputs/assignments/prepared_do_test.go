// Проверки правой части DO-присваиваний и принадлежности библиотечного шаблона конкретному каналу.
package assignments

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/iomap"
)

// Проверяет, что подготовленный DO берёт сигнал из правой части выражения и сохраняет выбранный шаблон отдельно для
// каждого канала.
func TestPreparedDOUsesExpressionRHSAndPreservesPerChannelTemplate(t *testing.T) {
	sheet := tinySheet("DO",
		`_IO_Q*A1-00*_DO32P_2_VAL.Measurement := _3101_XZV_60007_XYC;`,
		`_IO_Q*A1-00*_DO32P_3_VAL.Measurement := _OTHER_DDVH;`,
		`_IO_Q*A1-00*_DO32P_4_VAL.Measurement := _THIRD;`,
	)
	sheet.Rows[1].Cells["A"], sheet.Rows[1].Cells["M"], sheet.Rows[1].Cells["K"] = "3101-XZV-60007", "_3101_XZY_60007", "простой"
	sheet.Rows[2].Cells["E"], sheet.Rows[2].Cells["M"] = "3", "_OTHER"
	sheet.Rows[2].Cells["G"], sheet.Rows[2].Cells["H"], sheet.Rows[2].Cells["I"], sheet.Rows[2].Cells["C"] = "A3-00", "A4-00", "DOR-VFC", "OTHER_CAB"
	sheet.Rows[3].Cells["E"], sheet.Rows[3].Cells["K"], sheet.Rows[3].Cells["M"] = "4", "D32V", ""
	plan, err := ParseSheets([]iomap.Sheet{sheet})
	if err != nil {
		t.Fatal(err)
	}
	module := plan.Groups[0].Modules[0]
	if module.MainModule != "" || module.RedundantModule != "" || module.IOType != "" || module.MarshallingCabinet != "" || module.Template != "" {
		t.Fatal("row metadata was incorrectly presented as common module data", module)
	}
	want := []Channel{
		{Channel: 2, Tag: "_3101_XZV_60007_XYC", Template: "простой", SourceRow: 2},
		{Channel: 3, Tag: "_OTHER_DDVH", Template: "DO-1", SourceRow: 3},
		{Channel: 4, Tag: "_THIRD", Template: "D32V", SourceRow: 4},
	}
	if !reflect.DeepEqual(module.Channels, want) {
		t.Fatalf("channel source/template changed: %+v", module.Channels)
	}
	data, err := json.Marshal(module.Channels[0])
	if err != nil || !strings.Contains(string(data), `"template":"простой"`) {
		t.Fatal("per-channel template missing from API JSON", string(data), err)
	}
	var roundTrip Channel
	if err := json.Unmarshal(data, &roundTrip); err != nil || roundTrip != want[0] {
		t.Fatal("template JSON did not round-trip", roundTrip, err)
	}
}

// Проверяет идентичность подготовленных DO-строк: выбранный шаблон участвует в сравнении, разные шаблоны не
// схлопываются как дубль.
func TestPreparedDOExactDuplicateIncludesTemplate(t *testing.T) {
	sheet := tinySheet("DO", dout, dout)
	plan, err := ParseSheets([]iomap.Sheet{sheet})
	if err != nil || len(plan.Warnings) != 1 || len(plan.Groups[0].Modules[0].Channels) != 1 || plan.Groups[0].Modules[0].Channels[0].Template != "DO-1" {
		t.Fatalf("same RHS/template duplicate: %+v, %v", plan, err)
	}
	sheet.Rows[2].Cells["K"] = "простой"
	if _, err := ParseSheets([]iomap.Sheet{sheet}); err == nil {
		t.Fatal("same physical channel with conflicting templates was accepted")
	}
}

// Проверяет отказ подготовленного DO-адаптера при неверном адресе, идентификаторе сигнала или ПЛК до построения плана.
func TestPreparedDOStillRejectsInvalidAddressIdentifierAndController(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*iomap.Sheet)
	}{
		{"function RHS", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "_TAG_DDVH", "NOT(_TAG_DDVH)", 1) }},
		{"operator RHS", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "_TAG_DDVH", "NOT _TAG_DDVH", 1) }},
		{"extra statement", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = dout + " _OTHER := FALSE;" }},
		{"wrong module", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "A1-00", "A1-01", 1) }},
		{"wrong channel", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "DO32P_2", "DO32P_3", 1) }},
		{"channel 32", func(s *iomap.Sheet) {
			s.Rows[1].Cells["E"], s.Rows[1].Cells["F"] = "32", strings.Replace(dout, "DO32P_2", "DO32P_32", 1)
		}},
		{"invalid identifier", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "_TAG_DDVH", "_TAG-BAD", 1) }},
		{"control character", func(s *iomap.Sheet) { s.Rows[1].Cells["F"] = strings.Replace(dout, "_TAG_DDVH", "_TAG\x01", 1) }},
		{"unsupported template", func(s *iomap.Sheet) { s.Rows[1].Cells["K"] = "UNKNOWN" }},
		{"wrong IO kind", func(s *iomap.Sheet) { s.Rows[1].Cells["I"] = "DIR-VFC" }},
		{"conflicting controller", func(s *iomap.Sheet) { s.Rows[2].Cells["L"] = "42" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sheet := tinySheet("DO", dout, strings.Replace(dout, "DO32P_2", "DO32P_3", 1))
			sheet.Rows[2].Cells["E"] = "3"
			tc.mutate(&sheet)
			if _, err := ParseSheets([]iomap.Sheet{sheet}); err == nil {
				t.Fatal("invalid prepared DO mapping accepted")
			}
		})
	}
}

// Разбирает поставляемую подготовленную DO-книгу; сверяет число назначений и шаблоны каналов с фактической фикстурой.
func TestPreparedDONativeWorkbookCountsAndTemplates(t *testing.T) {
	data, err := os.ReadFile("../../../tests/fixtures/skzmap/do_native.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	const wantSHA = "b734a80fb656986f1079aeac48304205e5809d2e321b929fea86740cc796a4c3"
	if len(data) != 159128 || fmt.Sprintf("%x", sha256.Sum256(data)) != wantSHA {
		t.Fatal("native DO workbook fixture bytes changed")
	}
	plan, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	type counts struct{ groups, modules, channels, simple, do1 int }
	want := map[string]counts{
		"3000_S_SC_B01": {8, 44, 1284, 400, 884},
		"3000_S_SC_B03": {2, 16, 408, 344, 64},
	}
	actual := map[string]counts{}
	foundLoopTag, mixedTemplates := false, false
	for _, group := range plan.Groups {
		if group.Kind != "DO" || group.Key != group.ControllerName+":DO:"+group.Prefix {
			t.Fatal("unexpected group identity", group)
		}
		count := actual[group.ControllerName]
		count.groups++
		for _, module := range group.Modules {
			count.modules++
			if module.Type != "DO32P" || module.ObjectType != "D32V" || module.Capacity != 32 {
				t.Fatal("wrong physical module type", module)
			}
			if module.Template == "" {
				mixedTemplates = true
			}
			for i, channel := range module.Channels {
				count.channels++
				if channel.Channel < 0 || channel.Channel > 31 || channel.SourceRow < 2 || channel.Member != "" || channel.Reserve || i > 0 && module.Channels[i-1].Channel >= channel.Channel {
					t.Fatal("invalid, invented or reordered channel", channel)
				}
				switch channel.Template {
				case "простой":
					count.simple++
				case "DO-1":
					count.do1++
				default:
					t.Fatal("row template was lost", channel)
				}
				if channel.Tag == "_3101_XZV_60007_XYC" {
					foundLoopTag = true
					if channel.Template != "простой" {
						t.Fatal("Loop-derived RHS lost its template", channel)
					}
				}
			}
		}
		actual[group.ControllerName] = count
	}
	if !reflect.DeepEqual(actual, want) || len(plan.Groups) != 10 || len(plan.Warnings) != 0 || !foundLoopTag || !mixedTemplates {
		t.Fatalf("native DO counts: %+v; want %+v; warnings %v; Loop RHS %v, mixed templates %v", actual, want, plan.Warnings, foundLoopTag, mixedTemplates)
	}
}
