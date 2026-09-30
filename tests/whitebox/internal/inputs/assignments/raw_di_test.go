// Проверки исходных DI-таблиц: размещения, имена, конфликты и сохранение разреженных физических каналов.
package assignments

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"scheme-xml-generator/internal/iomap"
)

// Создаёт исходный DI-лист с изменяемыми полями для проверок каналов, размещений и ошибок адаптера.
func rawDISheet(overrides ...map[string]string) iomap.Sheet {
	rows := []iomap.Row{{Number: 1, Cells: map[string]string{
		"A": "Tag No", "B": "SCS", "C": "I/O Type", "D": "Main_module", "E": "Redundant_module", "F": "Channel", "G": "Loop",
		"H": "Main_module2", "I": "Redundant_module2", "J": "Mashalling_cabinet", "K": "ControllerID", "L": "Main_Chassis", "M": "Redundand_Chassis",
	}}}
	for index, values := range overrides {
		cells := map[string]string{"A": "3010-LZS-10001", "B": "PLC_850", "C": "DIR(I)-NAMUR", "D": "A11-05", "E": "A12-05", "F": "2", "G": "3010-LZS-10001", "H": "-", "I": "-", "J": "CAB", "L": "A11", "M": "A12"}
		for key, value := range values {
			cells[key] = value
		}
		rows = append(rows, iomap.Row{Number: index + 2, Cells: cells})
	}
	return iomap.Sheet{Name: "raw DI", Rows: rows}
}

// Разбирает поставляемую DI-книгу и сверяет группы ПЛК, физические модули и назначения с ожидаемым исходным набором.
func TestRawDIWorkbook(t *testing.T) {
	data, err := os.ReadFile("../../../tests/fixtures/skzmap/di.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 411336 || fmt.Sprintf("%X", sha256.Sum256(data)) != "C9C10D1E86936AC5B5BE17B76E2BDEFEDF71F97321938D1373CA179884879339" {
		t.Fatal("DI fixture differs from the unchanged user workbook")
	}
	plan, err := parseAssignmentPlan(data)
	if err != nil {
		t.Fatal(err)
	}
	controllers, receivers := map[string]bool{}, map[string]bool{}
	groups, modules, signals, c1, c2 := 0, 0, 0, 0, 0
	a70B01, namedReserves, spareModules := false, map[string]int{}, 0
	for _, group := range plan.Groups {
		if group.Kind != "DI" {
			continue
		}
		groups++
		controllers[group.ControllerName] = true
		if group.Kind != "DI" || group.POUName != "DI_"+group.Prefix || group.Key != group.ControllerName+":DI:"+group.Prefix {
			t.Fatalf("bad group %+v", group)
		}
		for _, module := range group.Modules {
			modules++
			if module.Type != "DI32" || module.ObjectType != "D32V" || module.Capacity != 32 {
				t.Fatalf("bad module %+v", module)
			}
			if module.Name == "A70-14" && strings.HasSuffix(group.ControllerName, "B01") {
				a70B01 = true
			}
			if module.Name == "A11-13" || module.Name == "A12-13" {
				spareModules++
				for _, channel := range module.Channels {
					if channel.Channel == 19 || channel.SourceRow == 1578 {
						t.Fatalf("SPARE became a receiver: %+v", channel)
					}
				}
			}
			for index, channel := range module.Channels {
				signals++
				if channel.SourceRow < 2 || channel.Tag == "" || channel.Channel < 0 || channel.Channel > 31 || index > 0 && module.Channels[index-1].Channel >= channel.Channel {
					t.Fatalf("bad channel %+v", channel)
				}
				key := strings.ToUpper(group.ControllerName + ":" + channel.Tag + "." + channel.Member)
				if receivers[key] {
					t.Fatalf("repeated receiver %s", key)
				}
				receivers[key] = true
				switch channel.Member {
				case "C1":
					c1++
				case "C2":
					c2++
				default:
					t.Fatalf("invalid member %+v", channel)
				}
				if channel.Tag == "_3010_GZY_10011" || channel.Tag == "_3010_GZY_10012" {
					namedReserves[channel.Tag]++
				}
			}
		}
	}
	if groups != 8 || len(controllers) != 3 || modules != 50 || signals != 1236 || c1 != 618 || c2 != 618 {
		t.Fatalf("groups=%d PLCs=%d modules=%d channels=%d C1=%d C2=%d", groups, len(controllers), modules, signals, c1, c2)
	}
	if !a70B01 || spareModules != 2 || namedReserves["_3010_GZY_10011"] != 2 || namedReserves["_3010_GZY_10012"] != 2 {
		t.Fatalf("special cases: A70 B01=%v spare modules=%d named reserves=%v", a70B01, spareModules, namedReserves)
	}
	warnings := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(warnings, "1578") || !strings.Contains(warnings, "SPARE") {
		t.Fatalf("missing source warnings: %v", plan.Warnings)
	}
	if plan.Source == nil || len(plan.Source.Records) != 1376 || len(plan.Source.Excluded) != 401 {
		t.Fatalf("physical IO inventory or excluded-row accounting lost: %+v", plan.Source)
	}
	// Check the source's secondary placements explicitly: support for a future
	// populated Main_module2/Redundant_module2 must never happen by omission.
	sheets, err := iomap.ReadWorkbook(data)
	if err != nil || len(sheets) != 1 || sheets[0].Name != "PS_IO_LIST_SCS_v4" {
		t.Fatalf("source sheets: %v %v", sheets, err)
	}
	var columns map[string]string
	for _, row := range sheets[0].Rows {
		candidate, found, headerErr := readRawIOHeader(row)
		if headerErr != nil {
			t.Fatal(headerErr)
		}
		if found {
			columns = candidate
			break
		}
	}
	if columns["mainmodule2"] == "" || columns["redundantmodule2"] == "" {
		t.Fatal("fixture secondary placement headers are missing")
	}
	rawRows := 0
	for _, row := range sheets[0].Rows {
		if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(row.Cells[columns["iotype"]])), "DI") {
			continue
		}
		rawRows++
		if row.Cells[columns["mainmodule2"]] != "-" || row.Cells[columns["redundantmodule2"]] != "-" {
			t.Fatalf("row %d has unhandled secondary physical placements", row.Number)
		}
	}
	if rawRows != 619 {
		t.Fatalf("raw DI rows=%d", rawRows)
	}
}

// Проверяет DI-адаптер на размещениях, именах и пропусках каналов; исходные физические номера не уплотняются.
func TestRawDIPlacementsNamesAndSparseChannels(t *testing.T) {
	plan, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(
		nil,
		map[string]string{"A": "_ALREADY_NAMED", "F": "31", "C": "DIR-VFC", "J": "OTHER CABINET", "G": "Резерв"},
		map[string]string{"A": "3010-OTHER-2", "F": "0", "C": "DIR (SCS1)", "J": "THIRD CABINET", "B": "plc_850", "D": "a11_05", "E": "a12_05"},
	)})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Groups) != 2 || len(plan.Warnings) != 0 {
		t.Fatalf("bad groups or warnings: %+v", plan)
	}
	for index, group := range plan.Groups {
		if group.ControllerName != "PLC_850" || len(group.Modules) != 1 {
			t.Fatalf("case-insensitive controller identity changed: %+v", group)
		}
		module := group.Modules[0]
		if len(module.Channels) != 3 || module.IOType != "" || module.MarshallingCabinet != "" {
			t.Fatalf("sparse channels or variable row metadata: %+v", module)
		}
		for i, channel := range module.Channels {
			if channel.Channel != []int{0, 2, 31}[i] || channel.Member != fmt.Sprintf("C%d", index+1) {
				t.Fatalf("wrong placement: %+v", channel)
			}
		}
		if module.Channels[1].Tag != "_3010_LZS_10001" || module.Channels[2].Tag != "_ALREADY_NAMED" || !module.Channels[2].Reserve {
			t.Fatalf("literal names/reserved receiver changed: %+v", module.Channels)
		}
	}
}

// Проверяет DI-модуль только с резервами и без обязательного дублирующего размещения; адаптер сохраняет существующий
// инвентарь.
func TestRawDISpareOnlyModuleAndOptionalRedundant(t *testing.T) {
	for _, redundant := range []string{"", "-"} {
		plan, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(map[string]string{"A": "SPARE", "E": redundant})})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Groups) != 1 || len(plan.Groups[0].Modules) != 1 || plan.Groups[0].Modules[0].Channels == nil || len(plan.Groups[0].Modules[0].Channels) != 0 || len(plan.Warnings) != 1 {
			t.Fatalf("SPARE-only physical module disappeared: %+v", plan)
		}
	}
	plan, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(map[string]string{"E": "-"})})
	if err != nil || len(plan.Groups) != 1 || len(plan.Groups[0].Modules[0].Channels) != 1 || plan.Groups[0].Modules[0].Channels[0].Member != "C1" {
		t.Fatalf("single placement: %+v %v", plan, err)
	}
}

// Подаёт исходному DI-адаптеру повреждённые поля; неверные строки должны отвергаться с диагностикой.
func TestRawDIRejectsInvalidData(t *testing.T) {
	for _, test := range []struct{ name, column, value string }{
		{"invalid SCS", "B", "PLC.850"}, {"SCS control", "B", "PLC_850\n"},
		{"empty tag", "A", ""}, {"tag control", "A", "TAG\x00"}, {"tag whitespace control", "A", "TAG\t"}, {"tag member", "A", "TAG.C1"}, {"long tag", "A", strings.Repeat("X", 160)},
		{"missing main", "D", "-"}, {"bad crate", "D", "A0-00"}, {"large slot", "D", "A1-4096"}, {"bad redundant", "E", "MODULE"}, {"same pair", "E", "a11_05"},
		{"low channel", "F", "-1"}, {"high channel", "F", "32"}, {"fraction channel", "F", "1.5"}, {"empty channel", "F", ""},
		{"unknown DI kind", "C", "DI-UNKNOWN"}, {"secondary main", "H", "A3-00"}, {"secondary redundant", "I", "A4-00"},
		{"invalid controller ID", "K", "3.4"}, {"negative controller ID", "K", "-1"}, {"overflow controller ID", "K", "999999999999999999999999"},
		{"metadata control", "J", "CAB\x00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(map[string]string{test.column: test.value})}); err == nil {
				t.Fatal("invalid raw DI row accepted")
			}
		})
	}
}

// Проверяет отказ DI-адаптера при конфликте физических позиций или получателей, чтобы один канал не получил
// неоднозначную связь.
func TestRawDIRejectsConflictingPositionsAndReceivers(t *testing.T) {
	for _, test := range []struct {
		name string
		rows []map[string]string
	}{
		{"physical position", []map[string]string{nil, {"A": "OTHER"}}},
		{"physical position case", []map[string]string{nil, {"A": "OTHER", "B": "plc_850", "D": "a11_05", "E": "-"}}},
		{"duplicate C1 receiver", []map[string]string{nil, {"F": "3", "E": "-"}}},
		{"duplicate C2 receiver", []map[string]string{nil, {"D": "A13-00", "F": "3"}}},
		{"duplicate receiver case", []map[string]string{nil, {"A": "3010-lzs-10001", "F": "3", "B": "plc_850"}}},
		{"SPARE occupied", []map[string]string{{"A": "SPARE"}, nil}},
		{"SPARE repeats", []map[string]string{{"A": "SPARE"}, {"A": "SPARE"}}},
		{"changed controller ID", []map[string]string{{"K": "5"}, {"A": "OTHER", "F": "3", "K": "6"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(test.rows...)}); err == nil {
				t.Fatal("conflicting assignments accepted")
			}
		})
	}
	plan, err := parseAssignmentSheets([]iomap.Sheet{rawDISheet(nil, map[string]string{"B": "OTHER_PLC"})})
	if err != nil || len(plan.Groups) != 4 {
		t.Fatalf("different PLC namespaces collided: %+v %v", plan, err)
	}
}

// Проверяет заголовки исходного DI-формата и объединение с подготовленным листом; оба источника сохраняются в одном
// плане.
func TestRawDIHeadersAndPreparedSheetMerge(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*iomap.Sheet)
	}{
		{"missing required column", func(s *iomap.Sheet) { delete(s.Rows[0].Cells, "E") }},
		{"missing Tag No", func(s *iomap.Sheet) { delete(s.Rows[0].Cells, "A") }},
		{"duplicate raw column", func(s *iomap.Sheet) { s.Rows[0].Cells["N"] = "Tag No" }},
		{"mixed signatures", func(s *iomap.Sheet) { s.Rows[0].Cells["N"] = "SCS DO" }},
		{"two tables one sheet", func(s *iomap.Sheet) { header := s.Rows[0]; header.Number = 3; s.Rows = append(s.Rows, header) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			sheet := rawDISheet(nil)
			test.edit(&sheet)
			if _, err := parseAssignmentSheets([]iomap.Sheet{tinySheet("AI", xin, xs), sheet}); err == nil {
				t.Fatal("ambiguous or corrupted raw header accepted next to a valid prepared map")
			}
		})
	}
	ai := tinySheet("AI", xin, xs)
	do := tinySheet("DO", strings.Replace(dout, "A1-00", "A2-00", 1))
	do.Rows[1].Cells["D"] = "A2-00"
	raw := rawDISheet(nil, map[string]string{"C": "AIR-EP", "A": "ignored AI", "D": "", "E": "", "F": ""})
	if _, err := parseAssignmentSheets([]iomap.Sheet{raw}); err == nil {
		t.Fatal("incomplete physical AI was silently ignored")
	}
	// Only unaddressed nonphysical rows may be excluded; AI must now be read or rejected explicitly.
	raw.Rows[2].Cells["C"] = "S"
	for _, sheets := range [][]iomap.Sheet{{ai, raw, do}, {raw, do, ai}} {
		plan, err := parseAssignmentSheets(sheets)
		if err != nil || len(plan.Groups) != 4 || len(plan.Warnings) != 0 || plan.Source == nil || len(plan.Source.Excluded) != 1 || plan.Source.Excluded[0].IOType != "S" {
			t.Fatalf("mixed sheets: %+v %v", plan, err)
		}
	}
	// A raw DI and a prepared map cannot claim one physical module, even when
	// controller capitalization differs and regardless of sheet order.
	raw.Rows[1].Cells["D"], raw.Rows[1].Cells["B"] = "A1-00", "plc_850"
	for _, sheets := range [][]iomap.Sheet{{ai, raw}, {raw, ai}} {
		if _, err := parseAssignmentSheets(sheets); err == nil {
			t.Fatal("AI/DI physical module collision accepted")
		}
	}
}
