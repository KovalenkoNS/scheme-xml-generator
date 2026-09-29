// Проверки планирования и XLS технологических объектов по инвентарю ПЛК, включая сигналы и резервы.
package techobjects

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/iomap"
	"scheme-xml-generator/internal/xls"
)

// Создаёт инвентарь AI/AO/DI/DO с сигналами и резервами для проверок таблицы технологических объектов.
func testSource() *iomap.Plan {
	c := iomap.Controller{Key: "B01:cabinet", Name: "3000_D_SC_B01", SourceController: "FCS1", Cabinet: "3000_D_SC_B01"}
	for _, spec := range []struct {
		rack string
		slot int
		kind string
		size int
	}{{"A12", 13, "DO32P", 32}, {"A11", 1, "AOC4H", 4}, {"A10", 10, "DI32", 32}, {"A11", 0, "AOC4H", 4}, {"A10", 2, "AI16H", 16}} {
		m := iomap.Module{Name: fmt.Sprintf("%s_%02d", spec.rack, spec.slot), Rack: spec.rack, Slot: spec.slot, Type: spec.kind, Capacity: spec.size}
		for i := 0; i < spec.size; i++ {
			channel := iomap.Channel{Channel: i, Reserve: true}
			if i == 0 && spec.kind != "DO32P" {
				channel.Reserve = false
				channel.SourceTag, channel.Description = "3107-LS-66309A", "Защита от перелива"
				if spec.kind == "AOC4H" {
					channel.Tag, channel.Redundant = "_SHARED_AO", spec.slot == 1
					channel.PeerModule = fmt.Sprintf("A11_%02d", 1-spec.slot)
				}
			}
			m.Channels = append(m.Channels, channel)
		}
		c.Modules = append(c.Modules, m)
	}
	return &iomap.Plan{Controllers: []iomap.Controller{c}}
}

// Проверяет план и XLS технологических объектов: диагностика модулей и резервы входят в одну книгу с согласованными
// итогами.
func TestModuleAndReserveObjectsShareOneWorkbook(t *testing.T) {
	source := testSource()
	preview, err := Preview(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 1 || preview[0].Summary != (Summary{5, 21, 26}) || preview[0].Types != (TypeCounts{1, 2, 1, 1}) {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	plans, err := Prepare(source, []iomap.Selection{{Key: "B01:cabinet"}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	if plan.Summary != preview[0].Summary {
		t.Fatal("preview and generation disagree")
	}
	byTag := map[string]Object{}
	for _, object := range plan.Objects {
		if _, exists := byTag[object.Tag]; exists {
			t.Fatalf("duplicate object: %s", object.Tag)
		}
		byTag[object.Tag] = object
	}
	ai := byTag["_3000_D_SC_B01_A10_02_AI16H"]
	if ai.Type != "AI_DIAG16_AD3v1_kvit" || ai.Template != "AIDIAG16_kvit" || ai.Name != "Диагностика AI/AO AI16H" || ai.Sign != "A10_02" || ai.Texting[2] != "n16:A10-02.0:не вывод::не вывод\nn64:A10-02.1:не вывод::не вывод\nn2048:A10-02.2:не вывод::не вывод\nn4096:A10-02.3:не вывод::не вывод" {
		t.Fatalf("AI differs from native profile: %+v", ai)
	}
	for _, module := range []string{"A11_00", "A11_01"} {
		ao := byTag["_3000_D_SC_B01_"+module+"_AOC4H"]
		if ao.Type != ai.Type || ao.Template != ai.Template || ao.Sign != module || ao.Texting[2] != "n16:A11-00(01).0:не вывод::не вывод" {
			t.Fatalf("AO pair differs from native profile: %+v", ao)
		}
	}
	di := byTag["_3000_D_SC_B01_A10_10"]
	if di.Type != "D32V" || di.Template != "D32V" || di.Name != "_3000_D_SC_B01_IO_DI32_A10_10" || di.Sign != "A10_10" {
		t.Fatalf("DI differs from native profile: %+v", di)
	}
	lines := strings.Split(di.Texting[0], "\n")
	if len(lines) != 32 || lines[0] != "n1:3107-LS-66309A Защита от перелива (0->1):не вывод::не вывод" || lines[31] != "n-2147483648:_3000_D_SC_B01_A10_10.i31:не вывод::не вывод" || !strings.Contains(di.Texting[1], "n256:Резерв:") {
		t.Fatalf("wrong digital events: %+v", di.Texting)
	}
	do := byTag["_3000_D_SC_B01_A12_13"]
	if do.Type != "D32V" || do.Template != "D32V" || do.Name != "_3000_D_SC_B01_IO_DO32P_A12_13" || strings.Count(do.Texting[1], ":Резерв:") != 4 {
		t.Fatalf("wrong DO object: %+v", do)
	}
	for _, object := range plan.Objects[5:] {
		wantTemplate := map[string]string{"AD3_v2": "AD3_v2", "AN_v1": "AN"}[object.Type]
		if wantTemplate == "" || object.Template != wantTemplate || object.Name != object.Tag || object.Sign != "" || object.Texting != ([3]string{}) {
			t.Fatalf("wrong spare object: %+v", object)
		}
		if strings.HasSuffix(object.Tag, "_0") || strings.Contains(object.Tag, "_A10_10_") || strings.Contains(object.Tag, "_A12_13_") {
			t.Fatalf("active/redundant channel or digital bit became a spare: %+v", object)
		}
	}
	data, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
		t.Fatal("result is not a binary XLS workbook")
	}
	if plan.Objects[0].Tag != "_3000_D_SC_B01_A10_02_AI16H" || source.Controllers[0].Modules[0].Name != "A12_13" {
		t.Fatal("unstable ordering or mutated source")
	}
}

// Проверяет переименование ПЛК и ресурс плана техобъектов; поздняя мутация исходного IO не меняет подготовленный XLS.
func TestRenameResourceAndSnapshot(t *testing.T) {
	source := testSource()
	plans, err := Prepare(source, []iomap.Selection{{Key: "B01:cabinet", Name: "3000_D_SC_B07_2"}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	before, err := Generate(plan)
	if err != nil {
		t.Fatal(err)
	}
	source.Controllers[0].Name = "CORRUPTED"
	source.Controllers[0].Modules[0].Channels[0].Reserve = false
	source.Controllers[0].Modules[0].Channels[0].Description = "CORRUPTED"
	after, err := Generate(plan)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("prepared objects retain mutable source data")
	}
	if plan.Resource != 2 || plan.ControllerName != "3000_D_SC_B07_2" {
		t.Fatal("lost target settings")
	}
	for _, object := range plan.Objects {
		if !strings.HasPrefix(object.Tag, "_3000_D_SC_B07_2_") || strings.Contains(fmt.Sprint(object), "3000_D_SC_B01") {
			t.Fatalf("foreign binding after rename: %+v", object)
		}
	}
}

// Сверяет 100 колонок и группы четырёх строк заголовка таблицы техобъектов с контрактом импорта XLS.
func TestXLSNativeHeader(t *testing.T) {
	rows := headerRows()
	if len(rows) != 4 || len(rows[0]) != 100 {
		t.Fatal("wrong native header dimensions")
	}
	for i, key := range []string{"-", "-", "Mode", "MARKA", "OBJTYPE", "NAME", "DISC", "OBJSIGN", "OBJNUMBER", "PLC_VARNAME", "ARH_PER", "KKS", "OBJDPARAM", "SREZCONTROL", "USERGROUP", "EVGROUP", "PLCNAME", "PLC_ADRESS", "PLC_GR", "TEMPLATE", "TEXTING", "TEXTING", "TEXTING"} {
		if rows[3][i] != xls.Text(key) {
			t.Fatalf("column %d header differs from native XLS", i+1)
		}
	}
	if rows[0][0] != xls.Text("Cлужебные") || rows[1][19] != xls.Text("ОБЩЕЕ СВ-ВО") || rows[1][20] != xls.Text("[]D32_Вых") || rows[1][21] != xls.Text("[]D32_Отекстовка групп") || rows[1][22] != xls.Text("[]AI_DIAG_Отекстовка групп") {
		t.Fatal("native parameter groups changed")
	}
}

// Проверяет отказ планировщика техобъектов при неверном выборе ПЛК, ресурсе и повреждённом инвентаре модулей.
func TestInvalidSelectionAndInventory(t *testing.T) {
	source := testSource()
	for _, selected := range [][]iomap.Selection{
		nil,
		{{Key: "unknown"}},
		{{Key: "B01:cabinet"}, {Key: "B01:cabinet"}},
		{{Key: "B01:cabinet", Name: "bad/name"}},
		{{Key: "B01:cabinet", Name: strings.Repeat("A", 101)}},
	} {
		if _, err := Prepare(source, selected, 1); err == nil {
			t.Fatalf("accepted invalid selection: %+v", selected)
		}
	}
	for _, resource := range []int{0, -1, 2147483648} {
		if _, err := Prepare(source, []iomap.Selection{{Key: "B01:cabinet"}}, resource); err == nil {
			t.Fatal("accepted invalid resource")
		}
	}
	for _, mutate := range []func(*iomap.Module){
		func(m *iomap.Module) { m.Type = "UNKNOWN" },
		func(m *iomap.Module) { m.Name = "bad" },
		func(m *iomap.Module) { m.Channels = m.Channels[:1] },
		func(m *iomap.Module) { m.Channels[0].Channel = 2 },
	} {
		bad := testSource()
		mutate(&bad.Controllers[0].Modules[0])
		if _, err := Prepare(bad, []iomap.Selection{{Key: "B01:cabinet"}}, 1); err == nil {
			t.Fatal("accepted invalid module")
		}
	}
	other := source.Controllers[0]
	other.Key = "second"
	source.Controllers = append(source.Controllers, other)
	if _, err := Prepare(source, []iomap.Selection{{Key: "B01:cabinet", Name: "PLC_A"}, {Key: "second", Name: "plc_a"}}, 1); err == nil {
		t.Fatal("accepted duplicate target PLC names")
	}
}

// Проверяет экранирование разделителей событий в техобъектах и диагностику AO без выдуманного резервного модуля.
func TestEventTextDelimitersAndUnpairedAO(t *testing.T) {
	if got := eventText(1, "Насос: пуск\r\nкоманда"); got != "n1:Насос — пуск команда:не вывод::не вывод" {
		t.Fatalf("event delimiters were not escaped: %q", got)
	}
	m := iomap.Module{Name: "A11_00", Type: "AOC4H"}
	if got := diagnosticObject("PLC", m).Texting; !reflect.DeepEqual(got, [3]string{"", "", "n16:A11-00.0:не вывод::не вывод"}) {
		t.Fatalf("invented redundant AO module: %v", got)
	}
}
