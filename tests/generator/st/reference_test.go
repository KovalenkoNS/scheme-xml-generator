// Проверки ST сопоставляют результат с независимыми книгами и XML-эталонами.
package generator_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"scheme-xml-generator/internal/generator/addressing"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/generator/planning"
	stgen "scheme-xml-generator/internal/generator/st"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/inputs/assignments"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestModuleRawWorkbookFullDOPhysicalAssignmentsAndLogicalCounts сверяет реальные DI/DO-книги со сводкой и полным числом физических ST-присваиваний.
func TestModuleRawWorkbookFullDOPhysicalAssignmentsAndLogicalCounts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "skzmap", "di.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := assignments.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"st"} {
		t.Run(kind, func(t *testing.T) {
			plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind))
			if err != nil {
				t.Fatal(err)
			}
			modules, signals, assignments, repeats, doAssignments, diAssignments := 0, 0, 0, 0, 0, 0
			for _, plan := range plans {
				modules += plan.ModuleCount
				signals += plan.SignalCount
				assignments += plan.AssignmentCount
				repeats += plan.RepeatedAssignmentCount
				result, err := (stgen.Generator{}).GenerateModuleMapping(plan, addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 100000, T11Start: 200000, CardStart: 300000})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "st" {
					var document xmlmodel.OutputSTDocument
					if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &document); err != nil {
						t.Fatal(err)
					}
					for i, pou := range document.POUS.Items {
						count := strings.Count(pou.Code, ":=")
						if plan.POUs[i].Kind == "DO" {
							doAssignments += count
							if count != len(plan.POUs[i].Modules)*32 {
								t.Fatal("DO POU is not fully mapped", pou.Name, count)
							}
						} else {
							diAssignments += count
						}
					}
				}
			}
			wantAssignments := 3598
			if kind == "st" {
				wantAssignments = 3954
				if doAssignments != 2304 || diAssignments != 1650 {
					t.Fatal("physical ST totals", doAssignments, diAssignments)
				}
			}
			if len(plans) != 3 || modules != 122 || signals != 3184 || assignments != wantAssignments || repeats != 1461 {
				t.Fatal("raw workbook counts", len(plans), modules, signals, assignments, repeats)
			}
		})
	}
}

// TestModuleActualWorkbookCountsAndDOReferenceAssignments Сверяет ST из реальных AI/DO-книг с числом модулей и полным набором физических DO-присваиваний.
func TestModuleActualWorkbookCountsAndDOReferenceAssignments(t *testing.T) {
	for _, tc := range []struct {
		name                                                string
		modules, signals, assignments, blocks, links, cards int
	}{
		{"ai.xlsx", 32, 488, 976, 3416, 0, 1464}, {"do.xlsx", 8, 144, 144, 152, 144, 44},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "skzmap", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			source, err := assignments.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"st"} {
				request := assignmentTestRequest(source, kind)
				if kind == "st" && tc.name == "do.xlsx" {
					for i, group := range source.Groups {
						if group.Prefix == "A3" {
							for j := range request.POUs[i].ModuleIDs {
								*request.POUs[i].ModuleIDs[j] = int64(19 + j)
							}
						} else {
							for j := range request.POUs[i].ModuleIDs {
								*request.POUs[i].ModuleIDs[j] = int64(23 + j)
							}
						}
					}
				}
				plans, err := planning.PrepareModulePlans(source, request)
				if err != nil {
					t.Fatal(err)
				}
				wantAssignments := tc.assignments
				if kind == "st" && tc.name == "do.xlsx" {
					wantAssignments = tc.modules * 32
				}
				if len(plans) != 1 || plans[0].ModuleCount != tc.modules || plans[0].SignalCount != tc.signals || plans[0].AssignmentCount != wantAssignments {
					t.Fatalf("counts %+v", plans)
				}
				result, err := (stgen.Generator{}).GenerateModuleMapping(plans[0], addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 100000, T11Start: 200000, CardStart: 300000})
				if err != nil {
					t.Fatal(err)
				}
				if tc.name == "do.xlsx" {
					var doc xmlmodel.OutputSTDocument
					if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
						t.Fatal(err)
					}
					for _, pou := range doc.POUS.Items {
						if pou.Name != "DO_A3_channels" {
							continue
						}
						count := 0
						seen := map[string]bool{}
						for _, line := range strings.Split(pou.Code, "\n") {
							if !strings.Contains(line, ":=") {
								continue
							}
							count++
							if seen[line] {
								t.Fatal("repeated DO native assignment", line)
							}
							seen[line] = true
							matched := false
							for module := 3; module <= 6; module++ {
								for ch := 0; ch < 32; ch++ {
									want := fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := _3000_G_SC_B01_A3_%02d._%02d;", module+16, ch, module, ch)
									if line == want {
										matched = true
									}
								}
							}
							if !matched {
								t.Fatal("outside SOGO_DO native assignment set", line)
							}
						}
						if count != 128 {
							t.Fatal("DO_A3 assignment count", count)
						}
					}
				}
			}
		})
	}
}

// assignmentDITopology Читает связи нативной DI-фикстуры для построения входа ST-проверки; фиксированного генератора FBD здесь нет.
func assignmentDITopology(t *testing.T, doc xmlmodel.OutputDocument) []string {
	t.Helper()
	cards := map[string]string{}
	for _, card := range doc.ISACards.Items {
		cards[card.ID] = card.Info
	}
	var result []string
	for _, pou := range doc.POUS.Items {
		blocks := map[string]xmlmodel.OutputBlock{}
		for _, block := range pou.ISAGraf.Blocks.Items {
			blocks[block.T11ID] = block
		}
		for _, link := range pou.ISAGraf.Links.Items {
			from, to := strings.Split(link.FirstPoint.Value, "|"), strings.Split(link.LastPoint.Last, "|")
			if len(from) != 4 || len(to) != 4 || from[1] != "False" || to[1] != "True" || to[2] != "0" {
				t.Fatal("invalid DI endpoints", link)
			}
			module, target := blocks[from[0]], blocks[to[0]]
			if module.ObjectType != "37" || module.Params.ISAObjectID != "1933" || target.ObjectType != "31" || target.Params.ISAObjectID != "885" || (target.Params.Text != ".C1" && target.Params.Text != ".C2") || target.Info != cards[target.Params.CardID]+target.Params.Text || target.Params.Initial == nil || *target.Params.Initial != "6(FALSE),TRUE,1,2(FALSE),0,6(FALSE)" {
				t.Fatal("DI must connect D32 outputs to DDR.C1/C2 owner cards", module, target)
			}
			result = append(result, module.Info+"|"+from[2]+"|"+target.Info)
		}
	}
	sort.Strings(result)
	return result
}

// TestModuleDINativeReferenceParity Сравнивает все DI ST-присваивания со штатным XML-образцом после разбора независимой топологии входа.
func TestModuleDINativeReferenceParity(t *testing.T) {
	fbdData, err := os.ReadFile(filepath.Join("../../fixtures/generator", "DI_850_FBD.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var native xmlmodel.OutputDocument
	// Native KLPath uses literal backspace separators. Normalize only the
	// in-memory test copy so encoding/xml can inspect the untouched fixture.
	if err := xml.Unmarshal(bytes.ReplaceAll(fbdData, []byte{8}, []byte{'/'}), &native); err != nil {
		t.Fatal(err)
	}
	if len(native.POUS.Items) != 1 || len(native.POUS.Items[0].ISAGraf.Blocks.Items) != 234 || len(native.POUS.Items[0].ISAGraf.Links.Items) != 225 || len(native.ISACards.Items) != 234 {
		t.Fatal("unexpected native DI reference composition")
	}
	group := assignments.Group{Key: "3000_S_SC_B01:DI:A11", ControllerName: "3000_S_SC_B01", Kind: "DI", Prefix: "A11", POUName: "DI_A11"}
	for slot := 5; slot <= 13; slot++ {
		group.Modules = append(group.Modules, assignments.Module{Name: fmt.Sprintf("A11-%02d", slot), Type: "DI32", ObjectType: "D32V", Capacity: 32})
	}
	nativeTopology := assignmentDITopology(t, native)
	for row, binding := range nativeTopology {
		parts := strings.Split(binding, "|")
		slot, err := strconv.Atoi(strings.TrimPrefix(parts[0], "_3000_S_SC_B01_A11_"))
		if err != nil {
			t.Fatal(err)
		}
		channel, err := strconv.Atoi(strings.TrimPrefix(parts[1], "_"))
		if err != nil {
			t.Fatal(err)
		}
		group.Modules[slot-5].Channels = append(group.Modules[slot-5].Channels, assignments.Channel{Channel: channel, Tag: strings.TrimSuffix(parts[2], ".C1"), Member: "C1", SourceRow: row + 1})
	}
	source := &assignments.Plan{Groups: []assignments.Group{group}}
	for _, kind := range []string{"st"} {
		request := assignmentTestRequest(source, kind)
		for i, id := range request.POUs[0].ModuleIDs {
			*id = int64(i + 5)
		}
		plans, err := planning.PrepareModulePlans(source, request)
		if err != nil {
			t.Fatal(err)
		}
		if plans[0].AssignmentCount != 297 || plans[0].RepeatedAssignmentCount != 0 || plans[0].SignalCount != 225 {
			t.Fatalf("native counts %+v", plans[0])
		}
		result, err := (stgen.Generator{}).GenerateModuleMapping(plans[0], addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000})
		if err != nil {
			t.Fatal(err)
		}

		stData, err := os.ReadFile(filepath.Join("../../fixtures/generator", "DI_850_ST.xml"))
		if err != nil {
			t.Fatal(err)
		}
		var expected, actual xmlmodel.OutputSTDocument
		if err := xml.Unmarshal(stData, &expected); err != nil {
			t.Fatal(err)
		}
		if err := xml.Unmarshal(result.XML, &actual); err != nil {
			t.Fatal(err)
		}
		assignments := func(code string) []string {
			var lines []string
			for _, line := range strings.Split(code, "\n") {
				if strings.Contains(line, ":=") {
					lines = append(lines, strings.TrimSpace(line))
				}
			}
			return lines
		}
		if !reflect.DeepEqual(assignments(expected.POUS.Items[0].Code), assignments(actual.POUS.Items[0].Code)) {
			t.Fatal("DI ST assignments differ from native reference")
		}
	}
}
