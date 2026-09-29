// Проверки правил исходной IO-таблицы: ёмкости, имена, резервы, стойки и приоритет явно заданных данных.
package iomap

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// Возвращает заголовки исходного IO-листа, используемые семантическими тестами адаптера, без изменения production-
// схемы.
func semanticsHeaders() map[string]string {
	return map[string]string{
		"A": "FCS", "B": "Control cabinet", "C": "ModuleType",
		"D": "Main module", "E": "Redundand module", "F": "Channel",
		"G": "Loop", "H": "Tag No", "I": "Loop No", "J": "TYP No",
		"K": "LL", "L": "L", "M": "H", "N": "HH",
		"O": "MainChassis", "P": "Slot", "Q": "SCADA Tag", "R": "SCADA Reserve Tag",
	}
}

// Создаёт исходную IO-строку и применяет точечные изменения полей для проверки правил имён, размещений и резервов.
func semanticsRow(changes map[string]string) map[string]string {
	values := map[string]string{
		"A": "FCS_UNIT", "B": "9000-D-SC-T99", "C": "AI16H", "D": "A91-03", "E": "-", "F": "2",
		"G": "9000-PIA-100", "H": "9000-PT-100", "I": "9000-P-100", "J": "AI",
	}
	for key, value := range changes {
		values[key] = value
	}
	return values
}

// Оборачивает исходные строки и заголовки в тестовый лист IO, сохраняя номера строк для проверки диагностик.
func semanticsSheets(rows ...map[string]string) []Sheet {
	sheet := Sheet{Name: "IO test", Rows: []Row{{Number: 1, Cells: semanticsHeaders()}}}
	for i, cells := range rows {
		sheet.Rows = append(sheet.Rows, Row{Number: i + 2, Cells: cells})
	}
	return []Sheet{sheet}
}

// Находит физический модуль в результате IO-адаптера; отсутствие ожидаемого имени завершает конкретный тест ошибкой.
func findSemanticModule(t *testing.T, plan *Plan, name string) Module {
	t.Helper()
	for _, controller := range plan.Controllers {
		for _, module := range controller.Modules {
			if module.Name == name {
				return module
			}
		}
	}
	t.Fatalf("missing module %s", name)
	return Module{}
}

// Проверяет аппаратную ёмкость модулей IO и заполнение свободных каналов резервами, не теряя исходные занятые позиции.
func TestIOSemanticsModuleCapacityAndFreeChannels(t *testing.T) {
	for _, tc := range []struct {
		kind            string
		capacity        int
		tag, objectType string
	}{
		{"AI16H", 16, "_9000_PI_100", "AD3_v2"},
		{"AOC4H", 4, "_9000_TV_100", "AN_v1"},
		{"DI32", 32, "_9000_XS_100", ""},
		{"DO32P", 32, "_9000_XS_100", ""},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			tag := "9000-XS-100"
			if tc.kind == "AOC4H" {
				tag = "9000-TY-100"
			}
			plan, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"C": tc.kind, "H": tag})))
			if err != nil {
				t.Fatal(err)
			}
			if plan.RowCount != 1 || plan.ModuleCount != 1 || plan.SignalCount != tc.capacity {
				t.Fatalf("bad counts: %+v", plan)
			}
			module := findSemanticModule(t, plan, "A91_03")
			if module.Capacity != tc.capacity || len(module.Channels) != tc.capacity || module.Rack != "A91" || module.Slot != 3 {
				t.Fatalf("bad module: %+v", module)
			}
			for channel, signal := range module.Channels {
				if signal.Channel != channel || signal.ObjectType != tc.objectType {
					t.Fatalf("bad channel metadata: %+v", signal)
				}
				if channel == 2 {
					if signal.Tag != tc.tag || signal.Reserve || signal.Redundant || signal.SourceRow != 2 {
						t.Fatalf("bad occupied channel: %+v", signal)
					}
				} else if signal.Tag != fmt.Sprintf("_9000_D_SC_T99_A91_03_%d", channel) || !signal.Reserve || signal.SourceRow != 0 {
					t.Fatalf("bad free channel: %+v", signal)
				}
			}
		})
	}
}

// Проверяет правила имён резервных AI и общих AO-получателей в IO-адаптере на основном и дублирующем размещении.
func TestIOSemanticsAIRedundantNamesAndAOSharedNames(t *testing.T) {
	for _, tc := range []struct {
		kind, tag, main, redundant string
		capacity                   int
	}{
		{"AI16H", "9000-PT-100", "_9000_PI_100_main", "_9000_PI_100_reserve", 16},
		{"AOC4H", "9000-TY-100", "_9000_TV_100", "_9000_TV_100", 4},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			plan, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"C": tc.kind, "E": "A91_04", "H": tc.tag})))
			if err != nil {
				t.Fatal(err)
			}
			if plan.RowCount != 1 || plan.ModuleCount != 2 || plan.SignalCount != 2*tc.capacity {
				t.Fatalf("bad counts: %+v", plan)
			}
			main := findSemanticModule(t, plan, "A91_03")
			backup := findSemanticModule(t, plan, "A91_04")
			if main.Channels[2].Tag != tc.main || backup.Channels[2].Tag != tc.redundant || main.Channels[2].Redundant || !backup.Channels[2].Redundant {
				t.Fatalf("bad placement naming: main %+v backup %+v", main.Channels[2], backup.Channels[2])
			}
			if backup.Channels[2].Reserve {
				t.Fatal("redundant technological placement confused with free channel")
			}
			if main.Channels[0].Tag != "_9000_D_SC_T99_A91_03_0" || backup.Channels[0].Tag != "_9000_D_SC_T99_A91_04_0" {
				t.Fatalf("free channel does not use its own physical module")
			}
		})
	}
}

// Проверяет, что явный резерв исходной IO-строки относится к своему физическому модулю, а не заимствует имя соседнего.
func TestIOSemanticsSourceReserveUsesOwnModule(t *testing.T) {
	plan, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"H": "—", "G": "invalid loop is irrelevant for free channel", "E": "A91-04"})))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A91_03", "A91_04"} {
		channel := findSemanticModule(t, plan, name).Channels[2]
		if channel.Tag != "_9000_D_SC_T99_"+name+"_2" || !channel.Reserve || channel.SourceRow != 2 {
			t.Fatalf("bad source reserve: %+v", channel)
		}
	}
}

// Проверяет приоритет явно заданных SCADA-имён над выводом имён IO-адаптером по исходному тегу.
func TestIOSemanticsExplicitNamesOverrideInference(t *testing.T) {
	plan, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"G": "not inferable", "H": "not inferable", "I": "", "E": "A91-04", "Q": "_Exact_AI_PRIMARY", "R": "_Exact_AI_BACKUP"})))
	if err != nil {
		t.Fatal(err)
	}
	if findSemanticModule(t, plan, "A91_03").Channels[2].Tag != "_Exact_AI_PRIMARY" || findSemanticModule(t, plan, "A91_04").Channels[2].Tag != "_Exact_AI_BACKUP" {
		t.Fatalf("explicit names lost: %+v", plan)
	}
	plan, err = ParseSheets(semanticsSheets(semanticsRow(map[string]string{"C": "AOC4H", "H": "9000-UNKNOWN-100", "E": "A91-04", "Q": "_Exact_AO"})))
	if err != nil {
		t.Fatal(err)
	}
	if findSemanticModule(t, plan, "A91_03").Channels[2].Tag != "_Exact_AO" || findSemanticModule(t, plan, "A91_04").Channels[2].Tag != "_Exact_AO" {
		t.Fatal("AO did not reuse explicit name for both physical placements")
	}
}

// Проверяет заполнение прежде свободного канала явным тегом без ложного конфликта с автоматически созданным резервом.
func TestIOSemanticsExplicitTagCanOccupyPreviouslyFreeChannel(t *testing.T) {
	plan, err := ParseSheets(semanticsSheets(semanticsRow(map[string]string{"H": "", "G": "", "I": "", "E": "A91-04", "Q": "_ExplicitPrimary", "R": "_ExplicitRedundant"})))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"A91_03": "_ExplicitPrimary", "A91_04": "_ExplicitRedundant"} {
		channel := findSemanticModule(t, plan, name).Channels[2]
		if channel.Reserve || channel.Tag != want {
			t.Fatalf("explicit name treated as empty reserve: %+v", channel)
		}
	}
}

// Переставляет заголовки IO-листа; проверяет сохранение смысла полей и исходных координат ошибок после распознавания
// колонок.
func TestIOSemanticsReorderedHeadersAndSourceCoordinates(t *testing.T) {
	original := semanticsRow(map[string]string{"O": "A91", "P": "3", "M": "90"})
	headers, cells := map[string]string{}, map[string]string{}
	for column, header := range semanticsHeaders() {
		// Deliberately invert columns and move required fields beyond Z.
		newColumn := testColumnName(65 - int(column[0]) + 60)
		headers[newColumn] = header
		cells[newColumn] = original[column]
		if header == "Main module" {
			headers[newColumn] = " Main module\nОсновной модуль "
		}
		if header == "Redundand module" {
			headers[newColumn] = "Redundant_module"
		}
	}
	sheets := []Sheet{{Name: "Instructions", Rows: []Row{{Number: 1, Cells: map[string]string{"A": "Read me"}}}}, {Name: "Reordered IO", Rows: []Row{{Number: 1, Cells: map[string]string{"A": "Title"}}, {Number: 7, Cells: headers}, {Number: 12, Cells: cells}}}}
	plan, err := ParseSheets(sheets)
	if err != nil {
		t.Fatal(err)
	}
	ch := findSemanticModule(t, plan, "A91_03").Channels[2]
	if plan.SheetName != "Reordered IO" || ch.SourceRow != 12 || ch.Tag != "_9000_PIA_100" {
		t.Fatalf("reordered schema/source coordinates lost: %+v, %+v", plan, ch)
	}
}

// Проверяет именование AI с аварийными и PID-признаками в IO-адаптере, включая приоритет явно переданных сведений.
func TestIOSemanticsAIAlarmAndPIDNaming(t *testing.T) {
	for _, tc := range []struct {
		name    string
		changes map[string]string
		tag     string
	}{
		{"no_alarm", map[string]string{}, "_9000_PI_100"},
		{"alarm", map[string]string{"N": "100"}, "_9000_PIA_100"},
		{"empty_alarm_markers", map[string]string{"K": "-", "L": "—", "M": " ", "N": ""}, "_9000_PI_100"},
		{"pid", map[string]string{"J": "PID_1", "G": "9000-PIC-100", "I": "9000-P-200"}, "_9000_PI_200"},
		{"cyrillic_lookalike", map[string]string{"G": "9000-РIА-100"}, "_9000_PI_100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := ParseSheets(semanticsSheets(semanticsRow(tc.changes)))
			if err != nil {
				t.Fatal(err)
			}
			if got := findSemanticModule(t, plan, "A91_03").Channels[2].Tag; got != tc.tag {
				t.Fatalf("got %q want %q", got, tc.tag)
			}
		})
	}
}

// Подаёт IO-адаптеру неверные поля строки; ожидает отказ вместо частичного плана с догаданными значениями.
func TestIOSemanticsRejectsInvalidRows(t *testing.T) {
	for _, tc := range []struct {
		name     string
		changes  map[string]string
		contains string
	}{
		{"unsupported", map[string]string{"C": "UNKNOWN"}, "ModuleType"},
		{"empty_type", map[string]string{"C": ""}, "ModuleType"},
		{"negative_channel", map[string]string{"F": "-1"}, "канал"},
		{"fractional_channel", map[string]string{"F": "1.5"}, "канал"},
		{"ai_upper_bound", map[string]string{"F": "16"}, "канал"},
		{"ao_upper_bound", map[string]string{"C": "AOC4H", "F": "4"}, "канал"},
		{"di_upper_bound", map[string]string{"C": "DI32", "F": "32"}, "канал"},
		{"do_upper_bound", map[string]string{"C": "DO32P", "F": "32"}, "канал"},
		{"bad_module", map[string]string{"D": "rack-slot"}, "модуля"},
		{"bad_slot", map[string]string{"D": "A91-16"}, "слот"},
		{"main_chassis_mismatch", map[string]string{"O": "A92"}, "MainChassis"},
		{"slot_mismatch", map[string]string{"P": "4"}, "Slot"},
		{"same_redundant", map[string]string{"E": "A91_03"}, "резервный модуль"},
		{"bad_redundant", map[string]string{"E": "A91-99"}, "резервный модуль"},
		{"empty_fcs", map[string]string{"A": ""}, "FCS"},
		{"invalid_fcs", map[string]string{"A": "FCS/../../evil"}, "FCS"},
		{"invalid_cabinet", map[string]string{"B": "cabinet<script>"}, "Control cabinet"},
		{"bad_explicit", map[string]string{"Q": "<injection>"}, "SCADA Tag"},
		{"missing_explicit_backup", map[string]string{"Q": "_Valid", "E": "A91-04"}, "SCADA Reserve Tag"},
		{"bad_explicit_backup", map[string]string{"Q": "_Valid", "R": "_Invalid.Name", "E": "A91-04"}, "SCADA Tag"},
		{"no_ai_inference", map[string]string{"G": "?", "I": "?"}, "Loop"},
		{"no_ao_rule", map[string]string{"C": "AOC4H", "H": "9000-ZZ-100"}, "правила имени AO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSheets(semanticsSheets(semanticsRow(tc.changes)))
			if err == nil || !strings.Contains(err.Error(), tc.contains) || !strings.Contains(err.Error(), "строка 2") {
				t.Fatalf("expected contextual %q error, got %v", tc.contains, err)
			}
		})
	}
}

// Проверяет конфликты физических координат IO-модулей и каналов; неоднозначное размещение не допускается в план.
func TestIOSemanticsRejectsCoordinateCollisions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     []map[string]string
		contains string
	}{
		{"same_channel", []map[string]string{semanticsRow(nil), semanticsRow(map[string]string{"H": "9000-PT-101"})}, "строки 2 и 3"},
		{"type_conflict", []map[string]string{semanticsRow(nil), semanticsRow(map[string]string{"C": "AOC4H", "F": "0", "H": "9000-TY-100"})}, "разными типами"},
		{"primary_overlaps_redundant", []map[string]string{semanticsRow(map[string]string{"E": "A91-04"}), semanticsRow(map[string]string{"D": "A91-04"})}, "строки 2 и 3"},
		{"redundant_overlaps_primary", []map[string]string{semanticsRow(nil), semanticsRow(map[string]string{"D": "A91-04", "E": "A91-03"})}, "строки 2 и 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSheets(semanticsSheets(tc.rows...))
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected collision %q got %v", tc.contains, err)
			}
		})
	}
	// Module addresses belong to their PLC/cabinet; identical addresses in
	// different PLCs must not be diagnosed as collisions.
	plan, err := ParseSheets(semanticsSheets(semanticsRow(nil), semanticsRow(map[string]string{"A": "FCS_OTHER", "B": "9000-D-SC-T98"})))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Controllers) != 2 || plan.ModuleCount != 2 {
		t.Fatalf("independent PLCs merged: %+v", plan)
	}
}

// Проверяет отказ IO-адаптера, когда книга не содержит распознаваемой таблицы или содержит неоднозначный набор данных.
func TestIOSemanticsRejectsMissingOrAmbiguousTables(t *testing.T) {
	if _, err := ParseSheets([]Sheet{{Name: "AO", Rows: []Row{{Number: 1, Cells: map[string]string{"A": "FCS", "B": "Module", "C": "DCS AO"}}}}}); err == nil {
		t.Fatal("derived map accepted as IO")
	}
	sheets := semanticsSheets(semanticsRow(nil))
	second := sheets[0]
	second.Name = "Copy"
	if _, err := ParseSheets(append(sheets, second)); err == nil || !strings.Contains(err.Error(), "несколько таблиц") {
		t.Fatalf("ambiguous tables accepted: %v", err)
	}
	if _, err := ParseSheets(semanticsSheets()); err == nil || !strings.Contains(err.Error(), "не найдено физических модулей") {
		t.Fatalf("empty map accepted: %v", err)
	}
}

// Проверяет конфликтующие IO-заголовки; адаптер не должен произвольно выбирать одну из колонок одного назначения.
func TestIOSemanticsRejectsAmbiguousHeaders(t *testing.T) {
	for _, duplicate := range []string{"Channel", "SCADA Main", "Redundant module"} {
		t.Run(duplicate, func(t *testing.T) {
			sheets := semanticsSheets(semanticsRow(nil))
			sheets[0].Rows[0].Cells["ZZ"] = duplicate
			_, err := ParseSheets(sheets)
			if err == nil || !strings.Contains(err.Error(), "заголовки") {
				t.Fatalf("ambiguous alias %q accepted: %v", duplicate, err)
			}
		})
	}
}

// Проверяет нормализацию имени ПЛК только внутри его физической стойки, сохраняя независимость остальных стоек книги.
func TestIOSemanticsNormalizeControllerWithinPhysicalRackOnly(t *testing.T) {
	plan, err := ParseSheets(semanticsSheets(
		semanticsRow(map[string]string{"A": "FCS_RIGHT", "F": "0"}),
		semanticsRow(map[string]string{"A": "FCS_RIGHT", "F": "1"}),
		semanticsRow(map[string]string{"A": "FCS_COPIED", "F": "2"}),
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Controllers) != 1 || plan.Controllers[0].SourceController != "FCS_RIGHT" || plan.ModuleCount != 1 {
		t.Fatalf("misplaced FCS label not normalized to physical rack: %+v", plan)
	}
	if !strings.Contains(strings.Join(plan.Warnings, "\n"), "FCS_COPIED") {
		t.Fatal("FCS correction not reported to user")
	}
	// Different racks in one cabinet may legitimately belong to separate PLCs.
	plan, err = ParseSheets(semanticsSheets(
		semanticsRow(map[string]string{"A": "FCS7", "B": "3000-D-SC-B07", "D": "A70-03"}),
		semanticsRow(map[string]string{"A": "FCS8", "B": "3000-D-SC-B07", "D": "A80-03"}),
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Controllers) != 2 || plan.Controllers[0].Name != "3000_D_SC_B07_1" || plan.Controllers[1].Name != "3000_D_SC_B07_2" {
		t.Fatalf("distinct PLC racks merged: %+v", plan)
	}
}

// Проверяет отказ IO-адаптера при неоднозначном владельце стойки вместо автоматического объединения разных ПЛК.
func TestIOSemanticsRejectsAmbiguousRackOwnership(t *testing.T) {
	_, err := ParseSheets(semanticsSheets(
		semanticsRow(map[string]string{"A": "FCS_A", "F": "0"}),
		semanticsRow(map[string]string{"A": "FCS_B", "F": "1"}),
	))
	if err == nil || !strings.Contains(err.Error(), "неоднозначно") {
		t.Fatalf("ownership tie guessed silently: %v", err)
	}
	_, err = ParseSheets(semanticsSheets(
		semanticsRow(map[string]string{"A": "FCS_A", "F": "0", "E": "A92-04"}),
		semanticsRow(map[string]string{"A": "FCS_A", "F": "1"}),
		semanticsRow(map[string]string{"A": "FCS_B", "F": "0", "D": "A92-03"}),
		semanticsRow(map[string]string{"A": "FCS_B", "F": "1", "D": "A92-03"}),
	))
	if err == nil || !strings.Contains(err.Error(), "разным FCS") {
		t.Fatalf("primary/redundant across distinct PLC owners accepted: %v", err)
	}
}

// Проверяет применение калибровки имён IO в точной области источника и приоритет явных значений над справочными.
func TestIOSemanticsCalibrationExactScopeAndExplicitPriority(t *testing.T) {
	if len(referenceTags) == 0 {
		t.Fatal("missing commissioning calibration")
	}
	keys := make([]string, 0, len(referenceTags))
	for key := range referenceTags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var row sourceRow
	var isRedundant bool
	var calibrated string
	for _, key := range keys {
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(key), &fields); err != nil || len(fields) != 12 {
			t.Fatalf("invalid calibration key %q", key)
		}
		destinations := []any{&row.FCS, &row.Cabinet, &row.Type, &row.Main, &row.Redundant, &row.Channel, &row.Loop, &row.LoopNo, &row.TagNo, &row.Typno, &row.Alarms, &isRedundant}
		for i, dest := range destinations {
			if err := json.Unmarshal(fields[i], dest); err != nil {
				t.Fatal(err)
			}
		}
		if row.Type == "AI16H" && !isRedundant {
			calibrated = referenceTags[key]
			break
		}
	}
	if calibrated == "" {
		t.Fatal("no primary AI calibration found")
	}
	if tag := referenceTag(row, isRedundant); tag != calibrated {
		t.Fatalf("exact row did not match %q: %q", calibrated, tag)
	}
	corrected := row
	corrected.OriginalFCS = row.FCS
	corrected.FCS = "FCS_CORRECTED_OWNER"
	if referenceTag(corrected, isRedundant) != calibrated {
		t.Fatal("physical owner correction discarded original-row calibration")
	}
	// Source row number is a diagnostic coordinate, not a naming input: moving
	// a row must not discard a valid calibration.
	moved := row
	moved.Number = 99999
	if referenceTag(moved, isRedundant) != calibrated {
		t.Fatal("calibration tied to arbitrary Excel row number")
	}
	mutations := []struct {
		name   string
		change func(*sourceRow)
	}{
		{"fcs", func(r *sourceRow) { r.FCS += "_CHANGED" }},
		{"cabinet", func(r *sourceRow) { r.Cabinet += "_CHANGED" }},
		{"type", func(r *sourceRow) { r.Type = "AOC4H" }},
		{"main", func(r *sourceRow) { r.Main = "A99999_15" }},
		{"redundant", func(r *sourceRow) { r.Redundant = "A99999_14" }},
		{"channel", func(r *sourceRow) { r.Channel = 1000 }},
		{"loop", func(r *sourceRow) { r.Loop += "_CHANGED" }},
		{"loop_number", func(r *sourceRow) { r.LoopNo += "_CHANGED" }},
		{"tag_number", func(r *sourceRow) { r.TagNo += "_CHANGED" }},
		{"typno", func(r *sourceRow) { r.Typno += "_CHANGED" }},
	}
	for alarm := 0; alarm < 4; alarm++ {
		i := alarm
		mutations = append(mutations, struct {
			name   string
			change func(*sourceRow)
		}{fmt.Sprintf("alarm_%d", i), func(r *sourceRow) { r.Alarms[i] += "_CHANGED" }})
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := row
			mutation.change(&changed)
			if tag := referenceTag(changed, isRedundant); tag != "" {
				t.Fatalf("stale calibration reused after %s changed: %q", mutation.name, tag)
			}
		})
	}
	if referenceKey(row, false) == referenceKey(row, true) {
		t.Fatal("calibration ignores primary/redundant role")
	}
	row.Explicit = "_OVERRIDE_CALIBRATED_NAME"
	tag, fromProfile, err := signalTag(row, false)
	if err != nil || fromProfile || tag != row.Explicit {
		t.Fatalf("explicit tag did not override commissioning name: %q %t %v", tag, fromProfile, err)
	}
}

// testColumnName labels reordered fixture columns independently of the workbook decoder.
func testColumnName(column int) string {
	name := ""
	for column > 0 {
		column--
		name = string(rune('A'+column%26)) + name
		column /= 26
	}
	return name
}
