// Проверки чтения библиотек XML, каталога шаблонов, совместимости и сохранности исходных файлов.
package library

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Читает XML-фильтр по одному байту; запрещённые C0 заменяются, а допустимые пробельные символы и UTF-8 сохраняются.
func TestXMLControlFilterReplacesOnlyForbiddenC0BytesAcrossSmallReads(t *testing.T) {
	input := []byte("A\t\n\rЖ")
	expected := append([]byte(nil), input...)
	for value := byte(0); value < 0x20; value++ {
		if value == '\t' || value == '\n' || value == '\r' {
			continue
		}
		input = append(input, value)
		expected = append(expected, ' ')
	}
	filter := &xmlControlFilter{reader: bytes.NewReader(input), counts: make(map[byte]int)}
	actual := make([]byte, 0, len(input))
	buffer := make([]byte, 1)
	for {
		count, err := filter.Read(buffer)
		actual = append(actual, buffer[:count]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("filtered bytes=%v want %v", actual, expected)
	}
	if len(filter.counts) != 29 {
		t.Fatalf("replaced control codes=%d want 29", len(filter.counts))
	}
}

// Копирует известную библиотеку AD3_v2 в временный каталог для проверок репозитория, не меняя установленный оригинал.
func testLibraryDirectory(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "libraries", "Library AD3_v2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "Library AD3_v2.xml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return directory
}

// Проверяет каталог AD3_v2: восемь шаблонов, их блоки, связи и графика совпадают с библиотечным источником.
func TestRepositoryLoadsEightTemplates(t *testing.T) {
	repository := NewRepository(testLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Errors) != 0 {
		t.Fatalf("load errors: %+v", catalog.Errors)
	}
	if len(catalog.Libraries) != 1 {
		t.Fatalf("libraries=%d, want 1", len(catalog.Libraries))
	}
	if catalog.Libraries[0].SupportedTemplateCount != 8 {
		t.Fatalf("supported templates=%d, want 8", catalog.Libraries[0].SupportedTemplateCount)
	}
	if len(catalog.Templates) != 8 {
		t.Fatalf("templates=%d, want 8", len(catalog.Templates))
	}
	expected := map[string][3]int{
		"17509": {33, 25, 1},
		"17510": {7, 0, 2},
		"18402": {31, 27, 1},
		"18555": {29, 20, 11},
		"18556": {44, 35, 1},
		"18558": {44, 37, 5},
		"19962": {45, 32, 32},
		"19963": {34, 27, 8},
	}
	for _, item := range catalog.Templates {
		counts, ok := expected[item.ID]
		if !ok {
			t.Fatalf("unexpected template %s", item.ID)
		}
		if item.BlockCount != counts[0] || item.LinkCount != counts[1] || item.GraphicCount != counts[2] {
			t.Errorf("template %s counts=%d/%d/%d want %d/%d/%d", item.ID, item.BlockCount, item.LinkCount, item.GraphicCount, counts[0], counts[1], counts[2])
		}
		if item.PrimitiveCount != counts[0]+counts[1]+counts[2] {
			t.Errorf("template %s primitive count mismatch", item.ID)
		}
		wantAI := item.ID == "17510" || item.ID == "18555" || item.ID == "19962"
		if gotAI := slicesContains(item.IOCapabilities, "AI"); gotAI != wantAI {
			t.Errorf("template %s AI capability=%v want %v (all=%v)", item.ID, gotAI, wantAI, item.IOCapabilities)
		}
	}
	resolved, missing := repository.ResolveMany([]string{catalog.Templates[0].Key, catalog.Templates[0].Key, "missing"})
	if len(resolved) != 1 || len(missing) != 1 || missing[0] != "missing" {
		t.Fatalf("ResolveMany resolved=%d missing=%v", len(resolved), missing)
	}
}

// Проверяет наличие ожидаемой возможности IO в строковом списке каталога библиотеки для точных утверждений теста.
func slicesContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// Проверяет признак AO в каталоге: требуется свободный выход REAL_TO_DINT с точным IsaObjId791, как у библиотечного
// генератора.
func TestIOCapabilitiesUseTheSameExactAOAnchorAsTheGenerator(t *testing.T) {
	template := &Template{Contents: Contents{Primitives: []Primitive{{
		ID: "10", ObjectType: "36", ISAObjectID: "792", TypeName: "REAL_TO_DINT",
	}}}}
	if capabilities := ioCapabilities(template); slicesContains(capabilities, "AO") {
		t.Fatalf("REAL_TO_DINT with the wrong ISA object was marked AO-compatible: %v", capabilities)
	}
	template.Contents.Primitives[0].ISAObjectID = "791"
	if capabilities := ioCapabilities(template); !slicesContains(capabilities, "AO") {
		t.Fatalf("free IsaObjId=791 REAL_TO_DINT was not marked AO-compatible: %v", capabilities)
	}
	template.Contents.Primitives = append(template.Contents.Primitives, Primitive{
		ID: "11", ObjectType: "20", Params: "[FP]=10|False|Result|0,0,100,20\n[LP]=12|True|0|0,0,100,20",
	})
	if capabilities := ioCapabilities(template); slicesContains(capabilities, "AO") {
		t.Fatalf("occupied REAL_TO_DINT.Result was marked AO-compatible: %v", capabilities)
	}
}

// Проверяет перенос геометрии библиотечного шаблона: координаты сдвигаются, маркер ветвления '*' остаётся на той же
// точке цепи.
func TestShiftPointsPreservesBranchMarker(t *testing.T) {
	input := "(380,600);*(380,600);(380,670);(590,670);"
	got, err := ShiftPoints(input, 300, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := "(680,700);*(680,700);(680,770);(890,770);"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Проверяет загрузку шаблона 19962: пустое значение IV сохраняет сам факт наличия поля у примитива, не превращаясь в
// отсутствующее.
func TestEmptyPrimitiveIVPresenceIsPreserved(t *testing.T) {
	repository := NewRepository(testLibraryDirectory(t))
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, item := range catalog.Templates {
		if item.ID == "19962" {
			key = item.Key
		}
	}
	ref, ok := repository.Resolve(key)
	if !ok {
		t.Fatal("template 19962 not found")
	}
	found := 0
	for _, primitive := range ref.Template.Contents.Primitives {
		if primitive.ObjectType == "37" && primitive.CardID == "0" {
			found++
			if primitive.InitialValue == nil || *primitive.InitialValue != "" {
				t.Errorf("primitive %s: empty IV presence lost", primitive.ID)
			}
		}
	}
	if found != 4 {
		t.Fatalf("cardless GT37=%d, want 4", found)
	}
}

// Проверяет чтение библиотеки с запрещённым XML-символом: каталог получает предупреждение, а исходные байты файла не
// изменяются.
func TestRepositorySanitizesInvalidXMLControlsWithoutChangingSource(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "vendor.xml")
	source := []byte(`<?xml version="1.0" encoding="UTF-8"?><root><SCADATA_VER VER="29"/><SECTION Num="2"><OTHER><OBJTYPE ID="1" Name="Owner"><TEMPLATES><TEMPLATE ID="2" Name="Control"><DISC/><DPARAMS>3</DPARAMS><HEIGHT>10</HEIGHT><WIDTH>10</WIDTH><FONCOLOR>0</FONCOLOR><CONTENTS><grprim><ID>3</ID><GROBJTYPE>34</GROBJTYPE><X>0</X><Y>0</Y><WIDTH>10</WIDTH><HEIGHT>10</HEIGHT><PARAMS>[TEXT]=before ` + string([]byte{0x12}) + ` after</PARAMS><CARDID>0</CARDID><OBJMSID>-100</OBJMSID></grprim></CONTENTS></TEMPLATE></TEMPLATES></OBJTYPE></OTHER></SECTION></root>`)
	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(directory)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Errors) != 0 || len(catalog.Libraries) != 1 || len(catalog.Templates) != 1 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	if len(catalog.Libraries[0].Warnings) != 1 || !strings.Contains(catalog.Libraries[0].Warnings[0], "U+0012×1") {
		t.Fatalf("control replacement warning missing: %+v", catalog.Libraries[0].Warnings)
	}
	ref, ok := repository.Resolve(catalog.Templates[0].Key)
	if !ok || !strings.Contains(ref.Template.Contents.Primitives[0].Params, "before   after") {
		t.Fatalf("sanitized PARAMS not loaded: %q", ref.Template.Contents.Primitives[0].Params)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, source) {
		t.Fatal("source library was changed during refresh")
	}
}

// Проверяет изоляцию синтаксически повреждённой библиотеки: каталог содержит ошибку файла, но не выдаёт его за
// загруженную библиотеку.
func TestRepositoryStillRejectsMalformedXML(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "broken.xml"), []byte(`<?xml version="1.0"?><root><SECTION>`), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(directory)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Libraries) != 0 || len(catalog.Errors) != 1 || catalog.Errors[0].File != "broken.xml" {
		t.Fatalf("malformed XML was not isolated as a load error: %+v", catalog)
	}
}

// Проверяет отказ анализа совместимости при повторных ID, неверных портах и геометрии; непонятная структура не
// объявляется поддержанной.
func TestTemplateCompatibilityRejectsFutureUnsafeStructures(t *testing.T) {
	owner := &ObjectType{ID: "owner", Name: "Owner", ISAObjects: ISAObjectList{Items: []ISAObject{
		{ID: "1", Prefix: "_DUP"},
		{ID: "2", Prefix: "_dup"},
	}}}
	template := &Template{ID: "unsafe", Name: "Unsafe", Width: "100", Height: "100", Contents: Contents{Primitives: []Primitive{
		{ID: "10", ObjectType: "37", X: "0", Y: "0", Width: "20", Height: "20", CardID: "1", ISAObjectID: "100", TypeName: "TYPE_A", LibraryName: "LIB", Params: "[TEXT]=\n[COMMENT]=false"},
		{ID: "10", ObjectType: "37", X: "30", Y: "0", Width: "20", Height: "20", CardID: "2", ISAObjectID: "100", TypeName: "TYPE_B", LibraryName: "LIB", Params: "[TEXT]=\n[COMMENT]=perhaps"},
		{ID: "12", ObjectType: "20", Params: "[PL]=(0,0);(30,0);\n[FP]=10|sideways|0|x\n[LP]=10|false|0|x"},
		{ID: "13", ObjectType: "7", Params: "[PL]=not-a-point-list"},
	}}}
	ref := &TemplateRef{Key: "unsafe", Library: &LoadedLibrary{FileName: "unsafe.xml"}, Owner: owner, Template: template}
	supported, warnings := TemplateCompatibility(ref)
	if supported {
		t.Fatal("unsafe template was marked supported")
	}
	joined := strings.Join(warnings, "\n")
	for _, fragment := range []string{"Повторный grprim ID=10", "Boolean COMMENT", "неизвестное направление", "противоречивые определения", "одинаковый Card.Info", "Графический примитив содержит некорректный PointList"} {
		if !strings.Contains(joined, fragment) {
			t.Errorf("compatibility warning %q missing in:\n%s", fragment, joined)
		}
	}
}

// Проверяет реальную полную библиотеку и возможности AI/AO/DI/DO; отсутствие частного файла явно отмечается SKIP.
func TestFullSinopecLibraryLoadsWhenPresent(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "libraries", "all_lb_sinopec.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local full Sinopec library is not present")
	} else if err != nil {
		t.Fatal(err)
	}
	library, err := loadLibrary(path, filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	templateCount, supportedCount := 0, 0
	wantedCapabilities := map[string]string{
		"17510": "AI",
		"17625": "AO",
		"18442": "DI",
		"18714": "DO",
	}
	foundCapabilities := make(map[string]bool, len(wantedCapabilities))
	walkObjectTypes(library.Document, func(owner *ObjectType) {
		for index := range owner.Templates.Items {
			templateCount++
			ref := &TemplateRef{Library: library, Owner: owner, Template: &owner.Templates.Items[index]}
			summary := summarizeTemplate(ref)
			if summary.Supported {
				supportedCount++
			}
			if capability, wanted := wantedCapabilities[summary.ID]; wanted && slicesContains(summary.IOCapabilities, capability) {
				foundCapabilities[summary.ID] = true
			}
		}
	})
	if templateCount != 377 {
		t.Fatalf("templates=%d, want 377", templateCount)
	}
	if supportedCount != 267 {
		t.Fatalf("supported templates=%d, want 267", supportedCount)
	}
	if len(library.Warnings) != 1 || !strings.Contains(library.Warnings[0], "U+0012×1") {
		t.Fatalf("unexpected warnings: %+v", library.Warnings)
	}
	for templateID, capability := range wantedCapabilities {
		if !foundCapabilities[templateID] {
			t.Errorf("template %s does not expose expected %s capability", templateID, capability)
		}
	}
}
