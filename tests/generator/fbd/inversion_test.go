// These external tests exercise public generation and HTTP boundaries. The
// synthetic template follows the library's DIO-1 topology with changed IDs;
// no private production models or copied generation code are used.
package generator_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"scheme-xml-generator/internal/appserver"
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/library"
)

// Создаёт минимальный синтетический DIO-1 для внешних тестов генератора.
// Сохраняет библиотечную BOOL→D32 топологию, меняя ISA ID для обнаружения подстановок.
func dioTemplate() *library.TemplateRef {
	template := &library.Template{ID: "12772", Name: "DIO-1", Width: "350", Height: "180", Contents: library.Contents{Primitives: []library.Primitive{
		{ID: "2", ObjectType: "37", CardID: "12", ISAObjectID: "91001", TypeName: "D32V_v1", LibraryName: "FixtureLibrary", X: "190", Y: "70", Width: "100", Height: "700", Params: "[TEXT]=\n[CI]=34\n[CO]=33\n[COMMENT]=False"},
		{ID: "1", ObjectType: "31", CardID: "11", ISAObjectID: "0", X: "40", Y: "90", Width: "100", Height: "20", Params: "[TEXT]=\n[CI]=1\n[CO]=1\n[COMMENT]=False"},
		{ID: "3", ObjectType: "20", Params: "[PL]=(140,100);(190,100);\n[FP]=1|False|0|0,0,0,0\n[LP]=2|True|i00|0,0,0,0\n[BN]=-1\n[CT]=0"},
	}}}
	owner := &library.ObjectType{ID: "2097", Name: "D32V", Templates: library.Templates{Items: []library.Template{*template}}, ISAObjects: library.ISAObjectList{Items: []library.ISAObject{
		{ID: "11", Prefix: "DI", TypeName: "BOOL", Kind: "1"},
		{ID: "12", TypeName: "D32V_v1", LibraryName: "FixtureLibrary", Kind: "4"},
	}}}
	return &library.TemplateRef{Key: "fixture:DIO-1", Owner: owner, Template: template, Library: &library.LoadedLibrary{Version: "29", FileName: "fixture.xml", FontStyles: map[string]library.FontStyle{}, TypeSignatures: map[string]library.Signature{}}}
}

// Подготавливает публичные ResolvedPOU для комбинаций инверсии в тестах.
// Каждый сигнал получает собственное имя; ссылка на исходный шаблон остаётся общей.
func resolved(ref *library.TemplateRef, invert ...bool) []generator.ResolvedPOU {
	signals := make([]generator.ResolvedSignal, len(invert))
	for i, value := range invert {
		signals[i] = generator.ResolvedSignal{Ref: ref, Request: generator.SignalRequest{TemplateKey: ref.Key, ObjectName: fmt.Sprintf("_CPU_SIGNAL_%d", i), NameMode: "base", Invert: value}}
	}
	return []generator.ResolvedPOU{{Request: generator.POURequest{Name: "DO_PROGRAM"}, Signals: signals}}
}

type block struct {
	ID, Kind, Info          string
	ISA, Card, CI, CO, Text string
	HasInitial              bool
}
type link struct{ First, Last string }

// Разбирает публичный XML-результат тестируемой генерации.
// Возвращает блоки и концы связей, не копируя закрытые модели renderer.
func graph(t *testing.T, data []byte) ([]block, []link) {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var blocks []block
	var links []link
	current := -1
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		element, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range element.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		switch element.Name.Local {
		case "Block":
			blocks = append(blocks, block{ID: attrs["T11ID"], Kind: attrs["GROBJTYPE"], Info: attrs["Info"]})
			current = len(blocks) - 1
		case "Params":
			if current >= 0 {
				b := &blocks[current]
				b.ISA, b.Card, b.CI, b.CO, b.Text = attrs["IsaObjId"], attrs["cardId"], attrs["CI"], attrs["CO"], attrs["T11Text"]
			}
		case "IV":
			if current >= 0 {
				blocks[current].HasInitial = true
			}
		case "Link":
			links = append(links, link{})
		case "FirstPoint":
			links[len(links)-1].First = attrs["FP"]
		case "LastPoint":
			links[len(links)-1].Last = attrs["LP"]
		}
	}
	return blocks, links
}

// Проверяет совместный выпуск инвертированного и прямого сигнала по публичному API.
// Сверяет источник типов, связи, расход ID и неизменность библиотеки.
func TestInversionUsesLibraryConnectionAndReservesExactIDs(t *testing.T) {
	ref := dioTemplate()
	input := resolved(ref, true, false)
	before, _ := json.Marshal(input)
	requirements, err := generator.RequirementsForDocument(input)
	if err != nil {
		t.Fatal(err)
	}
	if requirements.T11Count != 8 || requirements.CardCount != 4 {
		t.Fatalf("requirements=%+v", requirements)
	}
	result, err := (generator.Generator{Config: config.Default()}).GenerateDocument(generator.Request{}, input, generator.IDRange{T11Start: 100, CardStart: 200, POUID: 300})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.T11Last != 107 || result.Summary.Blocks != 5 || result.Summary.Links != 3 || result.Summary.Cards != 4 {
		t.Fatalf("summary=%+v", result.Summary)
	}
	after, _ := json.Marshal(input)
	if !bytes.Equal(before, after) {
		t.Fatal("generation mutated input/library")
	}
	blocks, links := graph(t, result.XML)
	byInfo := map[string]block{}
	seen := map[string]bool{}
	for _, b := range blocks {
		byInfo[b.Info] = b
		if seen[b.ID] {
			t.Fatal("duplicate block ID", b.ID)
		}
		seen[b.ID] = true
	}
	not := byInfo["NOT"]
	if not.Kind != "35" || not.ISA != "-32" || not.Card != "0" || not.CI != "1" || not.CO != "1" || not.Text != "" || not.HasInitial {
		t.Fatalf("NOT dialect=%+v", not)
	}
	if byInfo["_CPU_SIGNAL_0"].ISA != "91001" || !bytes.Contains(result.XML, []byte(`LibName="FixtureLibrary"`)) {
		t.Fatal("library type replaced by a built-in type")
	}
	want := map[link]bool{
		{byInfo["_CPU_SIGNAL_0DI"].ID + "|False|0|0,0,0,0", not.ID + "|True|0|0,0,0,0"}:                       false,
		{not.ID + "|False|Result|0,0,0,0", byInfo["_CPU_SIGNAL_0"].ID + "|True|i00|0,0,0,0"}:                  false,
		{byInfo["_CPU_SIGNAL_1DI"].ID + "|False|0|0,0,0,0", byInfo["_CPU_SIGNAL_1"].ID + "|True|i00|0,0,0,0"}: false,
	}
	for _, item := range links {
		item.First = strings.Join(strings.Split(item.First, "|")[:3], "|") + "|0,0,0,0"
		item.Last = strings.Join(strings.Split(item.Last, "|")[:3], "|") + "|0,0,0,0"
		if _, exists := want[item]; !exists {
			t.Fatalf("unexpected path=%+v", item)
		}
		want[item] = true
	}
	for item, found := range want {
		if !found {
			t.Fatalf("missing path=%+v", item)
		}
	}
}

// Проверяет, что инверсия канала не меняет соседний диагностический вход sts.
// Разбирает итоговый XML и сравнивает источник диагностической связи с исходным BOOL.
func TestInversionLeavesDiagnosticConnectionUntouched(t *testing.T) {
	ref := dioTemplate()
	quality := ref.Template.Contents.Primitives[2]
	quality.ID = "4"
	quality.Params = library.ReplaceParam(quality.Params, "LP", "2|True|sts|0,0,0,0")
	ref.Template.Contents.Primitives = append(ref.Template.Contents.Primitives, quality)
	result, err := (generator.Generator{Config: config.Default()}).GenerateDocument(generator.Request{}, resolved(ref, true), generator.IDRange{T11Start: 100, CardStart: 200, POUID: 300})
	if err != nil {
		t.Fatal(err)
	}
	blocks, links := graph(t, result.XML)
	boolID := ""
	notID := ""
	for _, b := range blocks {
		if b.Kind == "31" {
			boolID = b.ID
		}
		if b.Info == "NOT" {
			notID = b.ID
		}
	}
	found := false
	for _, item := range links {
		if strings.Contains(item.Last, "|True|sts|") {
			found = true
			if !strings.HasPrefix(item.First, boolID+"|") || strings.HasPrefix(item.First, notID+"|") {
				t.Fatal("diagnostic path inverted")
			}
		}
	}
	if !found {
		t.Fatal("diagnostic link lost")
	}
}

// Проверяет отказ расчёта требований для неподтверждённых типов, портов и связей.
// Ни одна отрицательная комбинация не должна доходить до резервирования ID.
func TestUnsupportedInversionFailsBeforeReservation(t *testing.T) {
	cases := map[string]func(*library.TemplateRef){
		"no path": func(ref *library.TemplateRef) {
			ref.Template.Contents.Primitives[2].Params = library.ReplaceParam(ref.Template.Contents.Primitives[2].Params, "LP", "2|True|sts|0,0,0,0")
		},
		"wrong source type": func(ref *library.TemplateRef) { ref.Owner.ISAObjects.Items[0].TypeName = "REAL" },
		"wrong source port": func(ref *library.TemplateRef) {
			ref.Template.Contents.Primitives[2].Params = library.ReplaceParam(ref.Template.Contents.Primitives[2].Params, "FP", "1|False|unknown|0,0,0,0")
		},
		"unknown channel": func(ref *library.TemplateRef) {
			ref.Template.Contents.Primitives[2].Params = library.ReplaceParam(ref.Template.Contents.Primitives[2].Params, "LP", "2|True|i32|0,0,0,0")
		},
		"wrong destination card": func(ref *library.TemplateRef) { ref.Owner.ISAObjects.Items[1].TypeName = "UnverifiedType" },
		"conversion": func(ref *library.TemplateRef) {
			ref.Template.Contents.Primitives[2].Params = library.ReplaceParam(ref.Template.Contents.Primitives[2].Params, "CT", "1")
		},
		"duplicate target": func(ref *library.TemplateRef) {
			item := ref.Template.Contents.Primitives[2]
			item.ID = "4"
			ref.Template.Contents.Primitives = append(ref.Template.Contents.Primitives, item)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			ref := dioTemplate()
			change(ref)
			if _, err := generator.RequirementsForDocument(resolved(ref, true)); err == nil || !strings.Contains(err.Error(), "инверсия") {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

// Вызывает публичный HTTP API с шаблоном без канальной связи D32.
// Проверяет HTTP 400 и сохранность каталога результатов и состояния allocator.
func TestHTTPInversionFailureDoesNotWriteAllocatorOrOutput(t *testing.T) {
	root := t.TempDir()
	libraryDir := filepath.Join(root, "libraries")
	outputDir := filepath.Join(root, "output")
	for _, directory := range []string{libraryDir, outputDir} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ref := dioTemplate()
	ref.Template.Contents.Primitives[2].Params = library.ReplaceParam(ref.Template.Contents.Primitives[2].Params, "LP", "2|True|sts|0,0,0,0")
	ref.Owner.Templates.Items = []library.Template{*ref.Template}
	document := library.Document{Version: library.Version{Value: "29"}, Sections: []library.Section{{Number: "2", Other: library.Other{ObjectTypes: []library.ObjectType{*ref.Owner}}}}}
	data, err := xml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(libraryDir, "fixture.xml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(libraryDir)
	catalog, err := repository.Refresh()
	if err != nil || len(catalog.Templates) != 1 {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
	statePath := filepath.Join(root, "state.json")
	allocator, err := generator.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	app := appserver.New(repository, generator.Generator{Config: config.Default()}, allocator, outputDir, fstest.MapFS{}, log.New(io.Discard, "", 0))
	request := generator.Request{POUs: []generator.POURequest{{Name: "DO", Signals: []generator.SignalRequest{{TemplateKey: catalog.Templates[0].Key, ObjectName: "_DO", NameMode: "base", Invert: true}}}}}
	data, _ = json.Marshal(request)
	response := httptest.NewRecorder()
	incoming := httptest.NewRequest(http.MethodPost, "http://localhost/api/generate", bytes.NewReader(data))
	incoming.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, incoming)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "инверсия") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err = os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("allocator changed: %v", err)
	}
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("output changed: files=%v err=%v", files, err)
	}
}

// Проверяет инверсию реального DIO-1 при наличии пользовательской библиотеки.
// Читает источник без изменения; отсутствие необязательного файла явно отмечается Skip.
func TestActualLibraryDIO1InversionWhenAvailable(t *testing.T) {
	directory := filepath.Join("..", "..", "..", "libraries")
	if _, err := os.Stat(filepath.Join(directory, "all_lb_sinopec.xml")); os.IsNotExist(err) {
		t.Skip("optional user library not present")
	}
	repository := library.NewRepository(directory)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range catalog.Templates {
		if item.LibraryFile == "all_lb_sinopec.xml" && item.ID == "12772" {
			ref, ok := repository.Resolve(item.Key)
			if !ok {
				t.Fatal("DIO-1 missing")
			}
			input := resolved(ref, true)
			requirements, err := generator.RequirementsForDocument(input)
			if err != nil {
				t.Fatal(err)
			}
			if requirements.T11Count != 5 || requirements.CardCount != 2 {
				t.Fatalf("requirements=%+v", requirements)
			}
			result, err := (generator.Generator{Config: config.Default()}).GenerateDocument(generator.Request{}, input, generator.IDRange{T11Start: 100, CardStart: 200, POUID: 300})
			if err != nil {
				t.Fatal(err)
			}
			blocks, links := graph(t, result.XML)
			if len(blocks) != 3 || len(links) != 2 {
				t.Fatalf("blocks=%d links=%d", len(blocks), len(links))
			}
			return
		}
	}
	t.Fatal("expected DIO-1 template not found")
}
