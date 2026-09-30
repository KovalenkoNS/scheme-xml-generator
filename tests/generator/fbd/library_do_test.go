// Module-level acceptance uses public APIs and synthetic library metadata.
// Changed ISA IDs prove the new path does not substitute a built-in D32 type.
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
	"scheme-xml-generator/internal/appserver"
	"scheme-xml-generator/internal/config"
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/allocation"
	"scheme-xml-generator/internal/generator/fbd"
	fbdrequest "scheme-xml-generator/internal/generator/fbd/request"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/library"
	"strings"
	"testing"
	"testing/fstest"
)

// Расширяет синтетический DIO-1 библиотечным определением QUAL_STAT.
// Собирает Document для resolver с ISA-номерами, отличными от native-профиля.
func moduleLibrary() *library.TemplateRef {
	ref := dioTemplate()
	qualityPrimitive := library.Primitive{ID: "92", ObjectType: "36", TypeName: "QUAL_STAT", ISAObjectID: "91002", LibraryName: "FixtureQuality", CardID: "0", Params: "[TEXT]=\n[CI]=1\n[CO]=1\n[COMMENT]=False"}
	qualityTemplate := library.Template{ID: "91", Name: "Quality", Contents: library.Contents{Primitives: []library.Primitive{qualityPrimitive}}}
	quality := library.ObjectType{ID: "90", Name: "Quality carrier", Templates: library.Templates{Items: []library.Template{qualityTemplate}}}
	ref.Library.Document = &library.Document{Version: library.Version{Value: "29"}, Sections: []library.Section{{Number: "2", Other: library.Other{ObjectTypes: []library.ObjectType{*ref.Owner, quality}}}}}
	return ref
}

// Создаёт запрос одного DO-модуля с 32 позициями каналов для теста.
// Возвращает CPU850/Measurement, явный ModuleID и независимые имена BOOL.
func moduleRequest(ref *library.TemplateRef) fbd.LibraryDORequest {
	id := int64(17)
	group, number := int64(45), int64(60)
	channels := make([]fbd.LibraryDOChannelRequest, 32)
	for i := range channels {
		channels[i] = fbd.LibraryDOChannelRequest{Channel: i, Tag: fmt.Sprintf("_CPU_TAG_%02d_DDVH", i), Invert: true}
	}
	return fbd.LibraryDORequest{TemplateKey: ref.Key, PLCName: "CPU", Context: &fbdrequest.GenerationContext{ControllerTypeName: cpuprofile.ControllerCPU850}, PhysicalProfile: addressing.PhysicalProfileMeasurement, POUs: []fbd.LibraryDOPOURequest{{Name: "DO_PROGRAM", GroupID: &group, POUNumber: &number, Modules: []fbd.LibraryDOModuleRequest{{Name: "A13-01", ID: &id, Channels: channels}}}}}
}

// Проверяет модульную схему: 32 BOOL/NOT, один D32 и одна диагностика.
// Сверяет каждый вход iNN, отсутствие связи Ct0, библиотечные типы и расход ID.
func TestLibraryDOThirtyTwoSignalsUseOneD32AndOneQualityChain(t *testing.T) {
	ref := moduleLibrary()
	request := moduleRequest(ref)
	before, _ := json.Marshal(ref)
	g := fbd.Generator{Config: config.Default()}
	plan, err := g.PrepareLibraryDO(ref, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Requirements(); got.T11Count != 133 || got.CardCount != 34 || got.SignalCount != 32 || got.POUCount != 1 {
		t.Fatalf("requirements=%+v", got)
	}
	result, err := g.GenerateLibraryDO(plan, xmlidentity.IDRange{T11Start: 100, CardStart: 1000, POUID: 2000})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(ref)
	if !bytes.Equal(before, after) {
		t.Fatal("mutated library")
	}
	blocks, links := graph(t, result.XML)
	kinds := map[string]int{}
	byID := map[string]block{}
	digital := ""
	for _, b := range blocks {
		kinds[b.Kind]++
		byID[b.ID] = b
		if b.Kind == "37" {
			digital = b.ID
			if b.ISA != "91001" || b.Info != "_CPU_A13_01" {
				t.Fatalf("D32 source=%+v", b)
			}
		}
		if b.Kind == "36" && b.ISA != "91002" {
			t.Fatalf("QUAL_STAT substituted: %+v", b)
		}
		if b.Kind == "38" && (b.ISA != "6127" || b.Info != "_IO_I17_DO32P_0_VAL_DIAG.Quality") {
			t.Fatalf("driver source=%+v", b)
		}
	}
	if kinds["37"] != 1 || kinds["38"] != 1 || kinds["36"] != 1 || kinds["31"] != 32 || kinds["35"] != 32 || len(links) != 66 {
		t.Fatalf("graph kinds=%v links=%d", kinds, len(links))
	}
	inputs := map[string]int{}
	qualityEdges := 0
	for _, edge := range links {
		from, to := strings.Split(edge.First, "|"), strings.Split(edge.Last, "|")
		if to[0] == digital {
			inputs[to[2]]++
			if to[2] != "sts" && byID[from[0]].Kind != "35" {
				t.Fatal("inverted channel bypassed NOT")
			}
		}
		if byID[from[0]].Kind == "38" && byID[to[0]].Kind == "36" && to[2] == "QUAL" {
			qualityEdges++
		}
	}
	for i := 0; i < 32; i++ {
		if inputs[fmt.Sprintf("i%02d", i)] != 1 {
			t.Fatalf("channel %d fan-in=%v", i, inputs)
		}
	}
	if inputs["sts"] != 1 || inputs["Ct0"] != 0 || qualityEdges != 1 {
		t.Fatalf("quality/Ct0=%v", inputs)
	}
	if result.Summary.T11Last != 232 || result.Summary.Cards != 34 || result.Summary.POUs[0].POUGroupID != "45" || result.Summary.POUs[0].POUNumber != "60" {
		t.Fatalf("summary=%+v", result.Summary)
	}
	if !bytes.Contains(result.XML, []byte(`LibName="FixtureQuality"`)) {
		t.Fatal("library dictionary lost")
	}
}

// Проверяет два независимых модуля, пропуски каналов и смешанную инверсию.
// Повторное имя BOOL делит карточку, но не объединяет D32 или входы модулей.
func TestLibraryDOMultipleModulesKeepHolesInversionAndSharedTags(t *testing.T) {
	ref := moduleLibrary()
	request := moduleRequest(ref)
	id := int64(18)
	request.POUs[0].Modules = append(request.POUs[0].Modules, fbd.LibraryDOModuleRequest{Name: "A13-02", ID: &id, Channels: []fbd.LibraryDOChannelRequest{{Channel: 0, Tag: "_CPU_TAG_00_DDVH", Invert: false}, {Channel: 31, Tag: "_CPU_EXTRA", Invert: true}}})
	g := fbd.Generator{Config: config.Default()}
	plan, err := g.PrepareLibraryDO(ref, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Requirements(); got.T11Count != 144 || got.CardCount != 37 {
		t.Fatalf("requirements=%+v", got)
	}
	result, err := g.GenerateLibraryDO(plan, xmlidentity.IDRange{T11Start: 100, CardStart: 1000, POUID: 2000})
	if err != nil {
		t.Fatal(err)
	}
	blocks, links := graph(t, result.XML)
	byID := map[string]block{}
	second := ""
	d32, not := 0, 0
	for _, b := range blocks {
		byID[b.ID] = b
		if b.Kind == "37" {
			d32++
			if b.Info == "_CPU_A13_02" {
				second = b.ID
			}
		}
		if b.Kind == "35" {
			not++
		}
	}
	if d32 != 2 || not != 33 {
		t.Fatalf("D32=%d NOT=%d", d32, not)
	}
	ports := map[string]string{}
	for _, edge := range links {
		from, to := strings.Split(edge.First, "|"), strings.Split(edge.Last, "|")
		if to[0] == second {
			ports[to[2]] = byID[from[0]].Kind
		}
	}
	if len(ports) != 3 || ports["i00"] != "31" || ports["i31"] != "35" || ports["sts"] != "36" {
		t.Fatalf("module holes/inversion lost: %v", ports)
	}
}

// Проверяет отказ подготовки плана при неполной библиотеке и неверном оборудовании.
// Охватывает QUAL_STAT, ModuleID, повторный канал и неподтверждённый CPU/драйвер.
func TestLibraryDORejectsMissingMetadataAndInvalidHardware(t *testing.T) {
	cases := map[string]func(*library.TemplateRef, *fbd.LibraryDORequest){
		"missing QUAL_STAT": func(ref *library.TemplateRef, _ *fbd.LibraryDORequest) {
			ref.Library.Document.Sections[0].Other.ObjectTypes = ref.Library.Document.Sections[0].Other.ObjectTypes[:1]
		},
		"conflicting QUAL_STAT": func(ref *library.TemplateRef, _ *fbd.LibraryDORequest) {
			owner := ref.Library.Document.Sections[0].Other.ObjectTypes[1]
			other := owner
			other.Templates = library.Templates{Items: append([]library.Template(nil), owner.Templates.Items...)}
			other.Templates.Items[0].Contents.Primitives = append([]library.Primitive(nil), owner.Templates.Items[0].Contents.Primitives...)
			other.Templates.Items[0].Contents.Primitives[0].ISAObjectID = "777"
			ref.Library.Document.Sections[0].Other.ObjectTypes = append(ref.Library.Document.Sections[0].Other.ObjectTypes, other)
		},
		"missing ModuleID": func(_ *library.TemplateRef, request *fbd.LibraryDORequest) { request.POUs[0].Modules[0].ID = nil },
		"duplicate channel": func(_ *library.TemplateRef, request *fbd.LibraryDORequest) {
			request.POUs[0].Modules[0].Channels[1].Channel = 0
		},
		"unsupported CPU715": func(_ *library.TemplateRef, request *fbd.LibraryDORequest) {
			request.Context.ControllerTypeName = cpuprofile.ControllerCPU715
		},
		"unsupported driver": func(_ *library.TemplateRef, request *fbd.LibraryDORequest) {
			request.PhysicalProfile = addressing.PhysicalProfileLegacy
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			ref := moduleLibrary()
			request := moduleRequest(ref)
			change(ref, &request)
			if _, err := (fbd.Generator{Config: config.Default()}).PrepareLibraryDO(ref, request); err == nil {
				t.Fatal("accepted unsupported request")
			}
		})
	}
}

// Проверяет HTTP handler на изолированной библиотеке и выходном каталоге.
// Отказ не меняет состояние; успех создаёт один XML и один диапазон ID.
func TestLibraryDOHTTPAllocatesOnceAndPersistsModuleGraph(t *testing.T) {
	root := t.TempDir()
	libraryDir, outputDir := filepath.Join(root, "libraries"), filepath.Join(root, "output")
	for _, dir := range []string{libraryDir, outputDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ref := moduleLibrary()
	data, err := xml.Marshal(ref.Library.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(libraryDir, "fixture.xml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := library.NewRepository(libraryDir)
	catalog, err := repository.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	key := ""
	for _, item := range catalog.Templates {
		if item.ID == "12772" {
			key = item.Key
		}
	}
	if key == "" {
		t.Fatal("template missing")
	}
	statePath := filepath.Join(root, "state.json")
	allocator, err := allocation.NewAllocator(statePath, config.Default().IDs)
	if err != nil {
		t.Fatal(err)
	}
	app := appserver.New(repository, config.Default(), allocator, outputDir, fstest.MapFS{}, log.New(io.Discard, "", 0))
	request := moduleRequest(ref)
	request.TemplateKey = key
	post := func(value fbd.LibraryDORequest) *httptest.ResponseRecorder {
		data, _ := json.Marshal(value)
		response := httptest.NewRecorder()
		incoming := httptest.NewRequest(http.MethodPost, "http://localhost/api/generate/library-do", bytes.NewReader(data))
		incoming.Header.Set("Content-Type", "application/json")
		app.Handler().ServeHTTP(response, incoming)
		return response
	}
	request.POUs[0].Modules[0].ID = nil
	failed := post(request)
	if failed.Code != http.StatusBadRequest {
		t.Fatalf("failedstatus=%d %s", failed.Code, failed.Body.String())
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("failed request changed allocator")
	}
	id := int64(17)
	request.POUs[0].Modules[0].ID = &id
	ok := post(request)
	if ok.Code != http.StatusCreated {
		t.Fatalf("status=%d %s", ok.Code, ok.Body.String())
	}
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("outputs=%v err=%v", files, err)
	}
	data, err = os.ReadFile(filepath.Join(outputDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	blocks, _ := graph(t, data)
	count := 0
	for _, b := range blocks {
		if b.Kind == "37" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("D32 count=%d", count)
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var cursor map[string]int64
	if err = json.Unmarshal(state, &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor["nextT11"] != config.Default().IDs.NextT11+133 {
		t.Fatalf("allocation=%v", cursor)
	}
}

// Проверяет сборку 32 каналов по фактическому DIO-1 и QUAL_STAT пользовательской библиотеки.
// Читает источник и генерирует в памяти; импорт SCADA не моделируется.
func TestActualLibraryModuleAssemblyWhenAvailable(t *testing.T) {
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
			ref, _ := repository.Resolve(item.Key)
			request := moduleRequest(ref)
			g := fbd.Generator{Config: config.Default()}
			plan, err := g.PrepareLibraryDO(ref, request)
			if err != nil {
				t.Fatal(err)
			}
			result, err := g.GenerateLibraryDO(plan, xmlidentity.IDRange{T11Start: 100, CardStart: 1000, POUID: 2000})
			if err != nil {
				t.Fatal(err)
			}
			blocks, links := graph(t, result.XML)
			if len(blocks) != 67 || len(links) != 66 {
				t.Fatalf("blocks=%d links=%d", len(blocks), len(links))
			}
			return
		}
	}
	t.Fatal("DIO-1 missing")
}
