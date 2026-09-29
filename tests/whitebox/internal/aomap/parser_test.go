// Проверки текстовой карты AO: ПЛК, размещения, повторы, кодировки и ошибки исходных назначений.
package aomap

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

const testHeader = "FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта\n"

// Создаёт строку текстовой карты AO с выражением, физическим каналом и размещениями для реального парсера.
func testRow(fcs, module string, channel int, tag, main, redundant, ioType string) string {
	return fmt.Sprintf("%s\tCAB-1\t%s\t%d\t_IO_QU*%s*_%d.ValueDINT := REAL_TO_DINT(%s.OUT, 0.0, 100.0);\t%s\t%s\t%s\tAN_v1\n", fcs, module, channel, module, channel, tag, main, redundant, ioType)
}

// Читает локальную карту AO и сверяет состав ПЛК, групп, модулей и назначений; отсутствие частного материала
// отмечается отдельно.
func TestParseProvidedMap(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "XML dev", "AO_excell_import_scheme.txt"))
	if os.IsNotExist(err) {
		t.Skip("development source map not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RowCount != 410 || plan.ModuleCount != 128 || plan.GroupCount != 21 || plan.ControllerCount != 8 || plan.ChannelCount != 512 || plan.ReserveCount != 102 || plan.UniqueTagCount != 207 || plan.DuplicateCount != 203 {
		t.Fatalf("unexpected source totals: rows=%d modules=%d groups=%d channels=%d reserves=%d tags=%d duplicates=%d", plan.RowCount, plan.ModuleCount, plan.GroupCount, plan.ChannelCount, plan.ReserveCount, plan.UniqueTagCount, plan.DuplicateCount)
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", plan.Warnings)
	}
	first := plan.Groups[0]
	if first.Key != "3000_D_SC_B01:A11" || first.POUName != "AO_A11" || len(first.Modules) != 14 {
		t.Fatalf("wrong first POU: %+v", first)
	}
	if first.Modules[0].Channels[0].Tag != "_3107_TV_64101A" || first.Modules[1].Channels[0].Tag != "_3107_TV_64101A" {
		t.Fatal("redundant physical module assignments must preserve repeated technology tags")
	}
	if first.Modules[0].Channels[0].Duplicate || !first.Modules[1].Channels[0].Duplicate || first.Modules[1].Channels[0].DuplicateOf != "AO_A11/A11_00/0" {
		t.Fatal("only redundant repeated calls must become empty graphical slots")
	}
	if first.Modules[4].Channels[3].Tag != "_3000_D_SC_B01_A11_04_3" || !first.Modules[4].Channels[3].Reserve {
		t.Fatalf("missing trailing channel was not filled: %+v", first.Modules[4])
	}
	duplicates := 0
	for _, part := range SplitByController(plan) {
		duplicates += part.DuplicateCount
		for _, group := range part.Groups {
			for _, module := range group.Modules {
				for _, channel := range module.Channels {
					if channel.Reserve && channel.Duplicate {
						t.Fatalf("unique unused reserve slot was lost: %+v", channel)
					}
				}
			}
		}
	}
	if duplicates != 203 {
		t.Fatalf("controller split lost duplicate counts: %d", duplicates)
	}
}

// Проверяет группировку текстовой карты AO при пропусках слотов и явно заданном резерве, сохраняя физические адреса.
func TestGroupingGapsAndExplicitReserve(t *testing.T) {
	data := testHeader +
		testRow("FCS2", "A11-00", 1, "_SECOND", "A11-00", "", "AO") +
		testRow("FCS1", "A12-00", 0, "_A12", "A12-00", "", "AO") +
		testRow("FCS1", "A11-12", 2, "_FCS1_A11_12_2", "A11-12", "", "AO") +
		testRow("FCS1", "A11-02", 3, "_A11", "A11-02", "", "AO")
	plan, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if plan.GroupCount != 3 || plan.ModuleCount != 4 || plan.ChannelCount != 16 || plan.ReserveCount != 13 || plan.UniqueTagCount != 3 {
		t.Fatalf("unexpected totals: %+v", plan)
	}
	if plan.Groups[0].Key != "FCS1:A11" || plan.Groups[1].Key != "FCS1:A12" || plan.Groups[2].Key != "FCS2:A11" {
		t.Fatalf("FCS and module prefixes must form distinct ordered groups: %+v", plan.Groups)
	}
	if plan.Groups[0].POUName != "AO_A11" || plan.Groups[1].POUName != "AO_A12" || plan.Groups[2].POUName != "AO_A11" {
		t.Fatal("POU names must use AO_<group>; controller identity belongs to the file, not the POU name")
	}
	modules := plan.Groups[0].Modules
	if len(modules) != 2 || modules[0].Name != "A11_02" || modules[1].Name != "A11_12" {
		t.Fatalf("module numbers must sort numerically without filling missing modules: %+v", modules)
	}
	if modules[0].Channels[0].Tag != "_FCS1_A11_02_0" || modules[0].Channels[0].SourceRow != 0 || modules[0].Channels[3].SourceRow != 5 {
		t.Fatalf("wrong reserve/source locations: %+v", modules[0].Channels)
	}
	if !modules[1].Channels[2].Reserve || modules[1].Channels[2].SourceRow != 4 {
		t.Fatalf("explicit reserve lost its source row: %+v", modules[1].Channels[2])
	}
}

// Проверяет разделение карты AO по ПЛК: одинаковые локальные имена и теги разных контроллеров не смешиваются.
func TestSplitByControllerKeepsControllerLocalNamesAndTags(t *testing.T) {
	source := testHeader +
		testRow("FCS2", "A11-00", 0, "_SHARED", "A11-00", "", "AO") +
		testRow("FCS1", "A12-00", 2, "_OTHER", "A12-00", "", "AO") +
		testRow("FCS1", "A11-00", 0, "_SHARED", "A11-00", "", "AO")
	plan, err := Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	parts := SplitByController(plan)
	if len(parts) != 2 || parts[0].Groups[0].ControllerName != "FCS1" || parts[1].Groups[0].ControllerName != "FCS2" {
		t.Fatalf("bad partitions: %+v", parts)
	}
	first, second := parts[0], parts[1]
	if first.ControllerCount != 1 || first.RowCount != 2 || first.GroupCount != 2 || first.ModuleCount != 2 || first.ChannelCount != 8 || first.ReserveCount != 6 || first.UniqueTagCount != 2 {
		t.Fatalf("wrong first controller totals: %+v", first)
	}
	if second.ControllerCount != 1 || second.RowCount != 1 || second.GroupCount != 1 || second.ModuleCount != 1 || second.ChannelCount != 4 || second.ReserveCount != 3 || second.UniqueTagCount != 1 {
		t.Fatalf("wrong second controller totals: %+v", second)
	}
	for _, part := range parts {
		group := part.Groups[0]
		if group.POUName != "AO_A11" || group.Modules[0].Channels[0].Tag != "_SHARED" {
			t.Fatal("partition rewrote names/tags")
		}
	}
	if len(plan.Groups) != 3 || plan.ControllerCount != 2 || plan.UniqueTagCount != 3 {
		t.Fatal("partition changed original preview")
	}
	if len(SplitByController(nil)) != 0 {
		t.Fatal("nil map should have no partitions")
	}
}

// Проверяет аннотации повторных и резервных AO-присваиваний после разбора текстовой карты.
func TestDuplicateAndRedundantAssignments(t *testing.T) {
	row := testRow("FCS1", "A11-00", 0, "_SHARED", "A11-00", "A11-01", "AOR(I)")
	plan, err := Parse([]byte(testHeader + row + row + testRow("FCS1", "A11-01", 0, "_SHARED", "A11-00", "A11-01", "AOR(I)")))
	if err != nil {
		t.Fatal(err)
	}
	if plan.RowCount != 3 || plan.ModuleCount != 2 || plan.UniqueTagCount != 1 || plan.DuplicateCount != 1 || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "Строка 3") {
		t.Fatalf("unexpected duplicate handling: %+v", plan)
	}
}

// Проверяет сравнение SCADA-тегов AO без учёта регистра, чтобы различие регистра не скрывало повторное назначение.
func TestSCADATagIdentityIsCaseInsensitive(t *testing.T) {
	data := testHeader +
		testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "", "AO") +
		testRow("FCS1", "A11-00", 1, "_tag", "A11-00", "", "AO") +
		testRow("FCS1", "A11-00", 2, "_fcs1_a11_00_2", "A11-00", "", "AO")
	plan, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if plan.UniqueTagCount != 1 || plan.ReserveCount != 2 || plan.DuplicateCount != 1 {
		t.Fatalf("SCADA tag identity must ignore case: %+v", plan)
	}
	if plan.Groups[0].Modules[0].Channels[1].Tag != "_tag" {
		t.Fatal("preserve source spelling while counting case-insensitive identity")
	}
	if !plan.Groups[0].Modules[0].Channels[1].Duplicate || plan.Groups[0].Modules[0].Channels[1].DuplicateOf != "AO_A11/A11_00/0" {
		t.Fatal("case-insensitive repeat must be an empty slot pointing to the first channel")
	}
}

// Меняет порядок строк карты AO; владельцем повторного тега должен оставаться основной модуль, а не первая прочитанная
// строка.
func TestDuplicateOwnerPrefersMainModuleIndependentOfRows(t *testing.T) {
	mainRow := testRow("FCS1", "A11-01", 2, "_TAG", "A11-01", "A11-00", "AOR")
	redundantRow := testRow("FCS1", "A11-00", 2, "_tag", "A11-01", "A11-00", "AOR")
	for _, rows := range []string{redundantRow + mainRow, mainRow + redundantRow} {
		plan, err := Parse([]byte(testHeader + rows))
		if err != nil {
			t.Fatal(err)
		}
		redundant := plan.Groups[0].Modules[0].Channels[2]
		main := plan.Groups[0].Modules[1].Channels[2]
		if plan.DuplicateCount != 1 || main.Duplicate || !redundant.Duplicate || redundant.DuplicateOf != "AO_A11/A11_01/2" {
			t.Fatalf("owner must use Main_module even when it sorts after the redundant module: %+v", plan)
		}
		if redundant.Tag != "_tag" || redundant.SourceRow == 0 || redundant.Reserve || redundant.Min != "0.0" || redundant.Max != "100.0" {
			t.Fatalf("blank graphical position lost the source assignment: %+v", redundant)
		}
	}
}

// Проверяет устойчивый выбор владельца AO-тега по физической позиции отдельно внутри каждого ПЛК.
func TestDuplicateOwnersUseStablePositionsAndControllerScope(t *testing.T) {
	rows := []string{
		testRow("FCS1", "A11-00", 0, "_TAG", "A11-01", "A11-00", "AOR"),
		testRow("FCS1", "A2-03", 0, "_TAG", "A2-04", "A2-03", "AOR"),
		testRow("FCS1", "A2-00", 2, "_TAG", "A2-01", "A2-00", "AOR"),
		testRow("FCS1", "A2-00", 1, "_TAG", "A2-01", "A2-00", "AOR"),
		testRow("FCS2", "A2-00", 1, "_TAG", "A2-01", "A2-00", "AOR"),
	}
	for _, order := range [][]int{{0, 1, 2, 3, 4}, {4, 3, 2, 1, 0}} {
		data := testHeader
		for _, index := range order {
			data += rows[index]
		}
		plan, err := Parse([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		if plan.DuplicateCount != 3 || plan.UniqueTagCount != 2 {
			t.Fatalf("unexpected scoped duplicates: %+v", plan)
		}
		for _, group := range plan.Groups {
			for _, module := range group.Modules {
				for _, channel := range module.Channels {
					owner := group.ControllerName == "FCS2" || (module.Name == "A2_00" && channel.Channel == 1)
					if channel.Tag == "_TAG" && channel.Duplicate == owner {
						t.Fatalf("unexpected owner at %s/%s/%d: %+v", group.ControllerName, module.Name, channel.Channel, channel)
					}
					if channel.Duplicate && channel.DuplicateOf != "AO_A2/A2_00/1" {
						t.Fatalf("unstable duplicate owner: %+v", channel)
					}
				}
			}
		}
		parts := SplitByController(plan)
		if parts[0].DuplicateCount != 3 || parts[1].DuplicateCount != 0 {
			t.Fatal("split must retain duplicates only within each controller")
		}
	}
}

// Проверяет разрешение повторов AO на копии плана: исходные данные сохраняются, аннотации пересчитываются по выбранным
// владельцам.
func TestResolveDuplicatesClonesAndRecomputesAnnotations(t *testing.T) {
	if ResolveDuplicates(nil) != nil {
		t.Fatal("nil plan must remain nil")
	}
	plan, err := Parse([]byte(testHeader +
		testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "A11-01", "AOR") +
		testRow("FCS1", "A11-01", 0, "_TAG", "A11-00", "A11-01", "AOR")))
	if err != nil {
		t.Fatal(err)
	}
	plan.Warnings = []string{"original warning"}
	plan.DuplicateCount = 500
	plan.Groups[0].Modules[0].Channels[0].Duplicate = true
	plan.Groups[0].Modules[0].Channels[0].DuplicateOf = "stale"
	plan.Groups[0].Modules[1].Channels[0].Duplicate = false
	resolved := ResolveDuplicates(plan)
	if plan.DuplicateCount != 500 || plan.Groups[0].Modules[0].Channels[0].DuplicateOf != "stale" || plan.Groups[0].Modules[1].Channels[0].Duplicate {
		t.Fatal("resolving repeats mutated its input")
	}
	if resolved.DuplicateCount != 1 || resolved.Groups[0].Modules[0].Channels[0].Duplicate || resolved.Groups[0].Modules[0].Channels[0].DuplicateOf != "" || !resolved.Groups[0].Modules[1].Channels[0].Duplicate {
		t.Fatal("resolving repeats trusted stale annotations")
	}
	if !reflect.DeepEqual(resolved, ResolveDuplicates(resolved)) {
		t.Fatal("resolving repeats must be idempotent")
	}
	resolved.Groups[0].POUName = "changed"
	resolved.Groups[0].Modules[0].Name = "changed"
	resolved.Groups[0].Modules[0].Channels[0].Tag = "changed"
	resolved.Warnings[0] = "changed"
	if plan.Groups[0].POUName != "AO_A11" || plan.Groups[0].Modules[0].Name != "A11_00" || plan.Groups[0].Modules[0].Channels[0].Tag != "_TAG" || plan.Warnings[0] != "original warning" {
		t.Fatal("resolved result shares mutable slices with its input")
	}
	part := SplitByController(plan)[0]
	part.Groups[0].Modules[0].Channels[0].Tag = "partition changed"
	if plan.Groups[0].Modules[0].Channels[0].Tag != "_TAG" {
		t.Fatal("controller partition shares mutable channel slices")
	}
}

// Подаёт текстовому AO-парсеру повреждённые заголовки, адреса и выражения; ожидает диагностируемый отказ.
func TestParseMalformedMaps(t *testing.T) {
	row := testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "", "AO")
	cases := []struct {
		name string
		data string
		want string
	}{
		{"not TSV", "FCS,Module\nFCS1,A11-00", "строка 1"},
		{"empty", testHeader, "не содержит назначений"},
		{"unknown header", strings.Replace(testHeader, "Channel", "Mystery", 1) + row, "неизвестный столбец"},
		{"wrong field count", testHeader + "FCS1\tA11-00\n", "строка 2"},
		{"control character", testHeader + strings.Replace(row, "CAB-1", "CAB\x01-1", 1), "управляющие символы"},
		{"wrong module", testHeader + strings.Replace(row, "A11-00", "nonsense", 1), "строка 2"},
		{"wrong channel", testHeader + strings.Replace(row, "\t0\t", "\t4\t", 1), "диапазона 0–3"},
		{"assignment module mismatch", testHeader + strings.Replace(row, "*A11-00*", "*A11-01*", 1), "не совпадает"},
		{"assignment channel mismatch", testHeader + strings.Replace(row, "*_0.ValueDINT", "*_1.ValueDINT", 1), "не совпадает"},
		{"unknown statement", testHeader + strings.Replace(row, "REAL_TO_DINT", "OTHER_CALL", 1), "DCS AO"},
		{"unsupported object", testHeader + strings.Replace(row, "AN_v1", "AD3_v2", 1), "тип объекта"},
		{"unsupported IO", testHeader + strings.Replace(row, "\tAO\t", "\tAI\t", 1), "не поддерживается"},
		{"redundancy missing", testHeader + strings.Replace(row, "\tAO\t", "\tAOR\t", 1), "не согласован"},
		{"reverse range", testHeader + strings.Replace(row, "0.0, 100.0", "100, 0", 1), "min < max"},
		{"infinite range", testHeader + strings.Replace(row, "0.0, 100.0", "0, 1e9999", 1), "конечными"},
		{"conflicting channel", testHeader + row + strings.Replace(row, "_TAG.OUT", "_OTHER.OUT", 1), "строка 3"},
		{"conflicting cabinet", testHeader + row + strings.Replace(testRow("FCS1", "A11-00", 1, "_OTHER", "A11-00", "", "AO"), "CAB-1", "CAB-2", 1), "противоречат строке 2"},
		{"cross-group redundancy", testHeader + testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "A12-01", "AOR"), "должен относиться к группе"},
		{"unrelated module", testHeader + testRow("FCS1", "A11-02", 0, "_TAG", "A11-00", "A11-01", "AOR"), "не согласован"},
		{"conflicting partners", testHeader + testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "A11-01", "AOR") + testRow("FCS1", "A11-01", 0, "_TAG", "A11-01", "A11-02", "AOR"), "пара модулей"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// Проверяет поддержанные кодировки текстовой карты AO, чтобы одинаковые исходные назначения давали одинаковый план.
func TestSupportedEncodings(t *testing.T) {
	data := testHeader + testRow("FCS1", "A11-00", 0, "_TAG", "A11-00", "", "AO")
	utf16Bytes := func(order binary.ByteOrder, bom []byte) []byte {
		result := append([]byte{}, bom...)
		for _, unit := range utf16.Encode([]rune(data)) {
			var pair [2]byte
			order.PutUint16(pair[:], unit)
			result = append(result, pair[:]...)
		}
		return result
	}
	cp1251 := make([]byte, 0, len(data))
	for _, r := range data {
		if r < 128 {
			cp1251 = append(cp1251, byte(r))
		} else if r >= 'А' && r <= 'я' {
			cp1251 = append(cp1251, byte(r-'А')+0xc0)
		} else {
			t.Fatalf("test encoding does not support %q", r)
		}
	}
	cases := map[string][]byte{
		"UTF8":     []byte(data),
		"UTF8 BOM": append([]byte{0xef, 0xbb, 0xbf}, []byte(data)...),
		"UTF16LE":  utf16Bytes(binary.LittleEndian, []byte{0xff, 0xfe}),
		"UTF16BE":  utf16Bytes(binary.BigEndian, []byte{0xfe, 0xff}),
		"CP1251":   cp1251,
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			plan, err := Parse(encoded)
			if err != nil || plan.UniqueTagCount != 1 {
				t.Fatalf("encoding failed: plan=%+v error=%v", plan, err)
			}
			if name == "CP1251" && len(plan.Warnings) != 1 {
				t.Fatal("fallback encoding must be reported")
			}
		})
	}
	for _, invalid := range [][]byte{{0xff, 0xfe, 0x00}, {0xff, 0xfe, 0x00, 0xd8}, {0xfe, 0xff, 0xdc, 0x00}, {0xef, 0xbb, 0xbf, 0xff}, {0x98}} {
		if _, err := Parse(invalid); err == nil {
			t.Errorf("invalid encoded input %x accepted", invalid)
		}
	}
}
