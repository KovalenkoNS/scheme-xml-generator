// Проверки исходных DO-таблиц: размещения, теги, резервные каналы и объединение с другими листами.
package assignments

import (
	"os"
	"strings"
	"testing"

	"scheme-xml-generator/internal/iomap"
)

// Создаёт лист исходных DO-каналов с заменяемыми полями строки; адаптер сам строит нейтральные назначения.
func rawDOSheet(overrides ...map[string]string) iomap.Sheet {
	rows := make([]map[string]string, len(overrides))
	for index, values := range overrides {
		rows[index] = map[string]string{"A": "3101-XZY-60507", "C": "DOR-P", "D": "A3-03", "E": "A4-03", "F": "5", "G": "3101-LOOP-NOT-TAG", "H": "A3-04", "I": "A4-04", "L": "A3", "M": "A4"}
		for key, value := range values {
			rows[index][key] = value
		}
	}
	sheet := rawDISheet(rows...)
	sheet.Name = "raw DO"
	return sheet
}

// Проверяет разбор исходной смешанной книги: DI и DO сохраняются в собственных группах и не исчезают при совместном
// импорте.
func TestRawMixedWorkbookIncludesDOAndDI(t *testing.T) {
	data, err := os.ReadFile("../../../tests/fixtures/skzmap/di.xlsx") // The original fixture is the whole mixed IO workbook.
	if err != nil {
		t.Fatal(err)
	}
	plan, err := parseAssignmentPlan(data)
	if err != nil {
		t.Fatal(err)
	}
	modules, signals, doGroups, doModules, doSignals := 0, 0, 0, 0, 0
	controllers, sourceRows := map[string]bool{}, map[int]int{}
	for _, group := range plan.Groups {
		modules += len(group.Modules)
		for _, module := range group.Modules {
			signals += len(module.Channels)
		}
		if group.Kind != "DO" {
			continue
		}
		doGroups++
		doModules += len(group.Modules)
		controllers[group.ControllerName] = true
		if group.Key != group.ControllerName+":DO:"+group.Prefix || group.POUName != "DO_"+group.Prefix {
			t.Fatalf("bad DO group %+v", group)
		}
		for _, module := range group.Modules {
			if module.Type != "DO32P" || module.ObjectType != "D32V" || module.Capacity != 32 {
				t.Fatalf("bad DO module %+v", module)
			}
			for index, channel := range module.Channels {
				doSignals++
				sourceRows[channel.SourceRow]++
				if !strings.HasPrefix(channel.Tag, "_") || !strings.HasSuffix(channel.Tag, "_DDVH") || channel.Member != "" || channel.SourceRow < 2 || channel.Channel < 0 || channel.Channel > 31 || index > 0 && module.Channels[index-1].Channel >= channel.Channel {
					t.Fatalf("bad DO channel %+v", channel)
				}
			}
		}
	}
	if len(plan.Groups) != 32 || modules != 166 || signals != 3724 {
		t.Fatalf("mixed totals: groups=%d modules=%d channels=%d", len(plan.Groups), modules, signals)
	}
	if doGroups != 12 || doModules != 72 || doSignals != 1948 || len(controllers) != 3 || len(sourceRows) != 487 {
		t.Fatalf("DO totals: groups=%d modules=%d channels=%d PLCs=%d source rows=%d", doGroups, doModules, doSignals, len(controllers), len(sourceRows))
	}
	for row, count := range sourceRows {
		if count != 4 {
			t.Fatalf("DO source row %d has %d physical placements, expected four", row, count)
		}
	}
}

// Проверяет четыре физических размещения DO в исходном формате: Tag No становится именем, разреженные номера каналов
// сохраняются.
func TestRawDOFourPlacementsUseTagNoAndKeepSparseChannels(t *testing.T) {
	plan, err := parseAssignmentSheets([]iomap.Sheet{rawDOSheet(
		nil,
		map[string]string{"A": "_ALREADY_NAMED", "F": "31", "C": "DOR-VFC", "J": "OTHER CABINET"},
		map[string]string{"A": "3101-OTHER-2", "F": "0", "C": "DOR (SCS3)", "B": "plc_850", "D": "a3_03", "E": "a4_03"},
	)})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Groups) != 2 || len(plan.Warnings) != 0 {
		t.Fatalf("unexpected groups/warnings %+v", plan)
	}
	for _, group := range plan.Groups {
		if group.ControllerName != "PLC_850" || len(group.Modules) != 2 {
			t.Fatalf("bad group %+v", group)
		}
		for _, module := range group.Modules {
			if len(module.Channels) != 3 || module.IOType != "" || module.MarshallingCabinet != "" {
				t.Fatalf("sparse map or row metadata changed: %+v", module)
			}
			for index, channel := range module.Channels {
				if channel.Channel != []int{0, 5, 31}[index] || channel.Member != "" {
					t.Fatalf("wrong physical assignment %+v", channel)
				}
			}
			if module.Channels[1].Tag != "_3101_XZY_60507_DDVH" || module.Channels[2].Tag != "_ALREADY_NAMED_DDVH" {
				t.Fatalf("Tag No was replaced by Loop or extra underscore: %+v", module.Channels)
			}
		}
	}
}

// Проверяет исходный DO-лист без необязательных размещений и Loop; адаптер не должен требовать отсутствующие данные
// для основного канала.
func TestRawDOOptionalPlacementsAndLoopAreNotRequired(t *testing.T) {
	for _, optional := range []string{"", "-"} {
		sheet := rawDOSheet(map[string]string{"E": optional, "H": optional, "I": optional, "G": ""})
		plan, err := parseAssignmentSheets([]iomap.Sheet{sheet})
		if err != nil || len(plan.Groups) != 1 || len(plan.Groups[0].Modules) != 1 || len(plan.Groups[0].Modules[0].Channels) != 1 {
			t.Fatalf("one explicit placement: %+v %v", plan, err)
		}
		// A DO-only raw sheet can omit all optional placement and Loop columns.
		for _, column := range []string{"E", "H", "I", "G"} {
			delete(sheet.Rows[0].Cells, column)
		}
		plan, err = parseAssignmentSheets([]iomap.Sheet{sheet})
		if err != nil || len(plan.Groups) != 1 || len(plan.Groups[0].Modules) != 1 {
			t.Fatalf("minimal DO columns: %+v %v", plan, err)
		}
	}
}

// Проверяет обработку повторов исходных DO-строк и изоляцию ПЛК с одинаковыми адресами модулей.
func TestRawDODuplicatesAndIndependentPLCs(t *testing.T) {
	// Identical physical assignments collapse even if descriptive Loop differs.
	plan, err := parseAssignmentSheets([]iomap.Sheet{rawDOSheet(nil, map[string]string{"A": "3101-xzy-60507", "B": "plc_850", "G": "OTHER LOOP"})})
	if err != nil || len(plan.Groups) != 2 || len(plan.Warnings) != 4 {
		t.Fatalf("exact DO duplicate: %+v %v", plan, err)
	}
	for _, group := range plan.Groups {
		for _, module := range group.Modules {
			if len(module.Channels) != 1 || module.Channels[0].SourceRow != 2 {
				t.Fatalf("duplicate changed original assignment %+v", module)
			}
		}
	}
	// One source can drive other physical channels and another PLC independently.
	plan, err = parseAssignmentSheets([]iomap.Sheet{rawDOSheet(nil, map[string]string{"F": "6"}, map[string]string{"B": "OTHER_PLC"})})
	if err != nil || len(plan.Groups) != 4 || len(plan.Warnings) != 0 {
		t.Fatalf("independent sources/PLCs: %+v %v", plan, err)
	}
	channels := 0
	for _, group := range plan.Groups {
		for _, module := range group.Modules {
			channels += len(module.Channels)
		}
	}
	if channels != 12 {
		t.Fatalf("repeated source lost physical outputs: %d", channels)
	}
	if _, err := parseAssignmentSheets([]iomap.Sheet{rawDOSheet(nil, map[string]string{"A": "OTHER-TAG", "B": "plc_850"})}); err == nil {
		t.Fatal("different DO sources occupied one physical channel")
	}
}

// Проверяет отказ DO-адаптера при неверных исходных полях и неподтверждённых именах резерва, без угадывания
// назначения.
func TestRawDORejectsInvalidDataAndUnknownReserveNames(t *testing.T) {
	for _, test := range []struct{ name, column, value string }{
		{"empty tag", "A", ""}, {"dash tag", "A", "-"}, {"SPARE tag", "A", "SPARE"}, {"reserve tag", "A", "Reserve"}, {"tag member", "A", "TAG.C1"}, {"long tag", "A", strings.Repeat("X", 155)}, {"tag control", "A", "TAG\t"},
		{"bad SCS", "B", "PLC.850"}, {"SCS control", "B", "PLC_850\n"}, {"unknown DO kind", "C", "DOR-UNKNOWN"},
		{"empty main", "D", ""}, {"dash main", "D", "-"}, {"bad crate", "D", "A0-00"}, {"large slot", "H", "A1-4096"}, {"bad redundant", "E", "MODULE"}, {"bad secondary redundant", "I", "A4-X"},
		{"duplicate placement", "H", "a3_03"}, {"main redundant collision", "E", "a3_03"}, {"redundant pair collision", "I", "a4_03"},
		{"negative channel", "F", "-1"}, {"large channel", "F", "32"}, {"fraction channel", "F", "1.5"}, {"missing channel", "F", ""},
		{"bad controller ID", "K", "x"}, {"negative controller ID", "K", "-1"}, {"metadata control", "J", "CAB\x00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAssignmentSheets([]iomap.Sheet{rawDOSheet(map[string]string{test.column: test.value})}); err == nil {
				t.Fatal("invalid DO input accepted")
			}
		})
	}
	_, err := parseAssignmentSheets([]iomap.Sheet{rawDOSheet(map[string]string{"A": "SPARE"})})
	if err == nil || !strings.Contains(err.Error(), "DO:") || !strings.Contains(err.Error(), "резерв") || !strings.Contains(err.Error(), "имени") {
		t.Fatalf("unclear reserve error: %v", err)
	}
}

// Проверяет объединение исходных и подготовленных DO-листов; конфликт типов на одной физической позиции должен
// завершаться ошибкой.
func TestRawDOPreparedMapsAndPhysicalKindCollisions(t *testing.T) {
	rawDO := rawDOSheet(nil)
	rawDI := rawDISheet(nil)
	ai := tinySheet("AI", xin, xs)
	preparedDO := numberedModuleSheet("DO", "6")
	for _, sheets := range [][]iomap.Sheet{{rawDO, rawDI, ai, preparedDO}, {preparedDO, ai, rawDI, rawDO}} {
		plan, err := parseAssignmentSheets(sheets)
		if err != nil || len(plan.Groups) != 6 {
			t.Fatalf("prepared/raw merge: %+v %v", plan, err)
		}
	}
	// DI and DO must never describe the same physical module, including case aliases.
	rawDI.Rows[1].Cells["D"], rawDI.Rows[1].Cells["B"] = "a3_03", "plc_850"
	for _, sheets := range [][]iomap.Sheet{{rawDO, rawDI}, {rawDI, rawDO}} {
		if _, err := parseAssignmentSheets(sheets); err == nil {
			t.Fatal("DI/DO physical type collision accepted")
		}
	}
	rawDO.Rows[1].Cells["D"], rawDO.Rows[1].Cells["B"] = "a1_00", "plc_850"
	for _, sheets := range [][]iomap.Sheet{{rawDO, ai}, {ai, rawDO}} {
		if _, err := parseAssignmentSheets(sheets); err == nil {
			t.Fatal("prepared AI/raw DO physical type collision accepted")
		}
	}
}
