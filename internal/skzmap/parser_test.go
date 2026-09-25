package skzmap

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/iomap"
)

func TestSourceWorkbooks(t *testing.T) {
	for _, test := range []struct {
		file                      string
		kind                      string
		groups, modules, channels int
	}{
		{"ai.xlsx", "AI", 4, 32, 488},
		{"do.xlsx", "DO", 2, 8, 144},
	} {
		t.Run(test.kind, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			modules, channels := 0, 0
			for _, group := range plan.Groups {
				if group.SCS != "3000_G_SC_B01" || group.Kind != test.kind || group.Key != group.SCS+":"+group.Kind+":"+group.Prefix || group.POUName != group.Kind+"_"+group.Prefix {
					t.Fatalf("bad group %+v", group)
				}
				for _, module := range group.Modules {
					modules++
					channels += len(module.Channels)
					if test.kind == "AI" && (module.Type != "AI16H" || module.ObjectType != "AD3_v2" || module.Capacity != 16) || test.kind == "DO" && (module.Type != "DO32P" || module.ObjectType != "D32V" || module.Capacity != 32) {
						t.Fatalf("bad module %+v", module)
					}
					for i, channel := range module.Channels {
						if channel.Reserve || channel.SourceRow < 2 || channel.Tag == "" || channel.Member != "" || i > 0 && module.Channels[i-1].Channel >= channel.Channel {
							t.Fatalf("bad channel %+v", channel)
						}
					}
				}
			}
			if len(plan.Groups) != test.groups || modules != test.modules || channels != test.channels || len(plan.Warnings) != 0 {
				t.Fatalf("groups=%d modules=%d channels=%d warnings=%v", len(plan.Groups), modules, channels, plan.Warnings)
			}
			if test.kind == "AI" {
				if len(plan.Groups[2].Modules[0].Channels) != 4 || len(plan.Groups[3].Modules[0].Channels) != 4 {
					t.Fatal("sparse source was filled with artificial reserves")
				}
				if !strings.HasSuffix(plan.Groups[1].Modules[0].Channels[0].Tag, "_reserve") {
					t.Fatal("redundant AI object was lost")
				}
			} else {
				for _, group := range plan.Groups {
					if len(group.Modules) != 4 || group.Modules[0].Channels[0].Tag != "_3101_BIALS_1001_DDVH" || group.Modules[1].Channels[0].Tag != "_3101_BIALS_1001_DDVH" {
						t.Fatal("four independent DO physical destinations were collapsed or rewritten")
					}
				}
			}
		})
	}
}

func tinySheet(kind string, expressions ...string) iomap.Sheet {
	header := map[string]string{"A": "Loop", "B": "SCS", "C": "Mashalling_cabinet", "D": "Module", "E": "Channel", "F": "SCS " + kind, "G": "Main_module", "H": "Redundant_module", "I": "I/O Type", "J": "Тип объекта", "K": "Шаблон", "L": "ControllerID", "M": "Марка"}
	rows := []iomap.Row{{Number: 1, Cells: header}}
	for i, expression := range expressions {
		cells := map[string]string{"A": "3101-TAG-01", "B": "PLC_850", "C": "CAB", "D": "A1-00", "E": "2", "F": expression, "G": "A1-00", "H": "A2-00", "I": "AIR-EP(3 WIRES)", "J": "AD3_v2", "K": "AD3_v2", "M": "_TAG"}
		if kind == "DO" {
			cells["I"], cells["J"], cells["K"] = "DOR-P", "", "DO-1"
		}
		rows = append(rows, iomap.Row{Number: i + 2, Cells: cells})
	}
	return iomap.Sheet{Name: kind, Rows: rows}
}

const xin = `_TAG.Xin := _IO_I*A1-00*_AI16H_2_VAL.Measurement;`
const xs = `_TAG.Xs := QUAL_STAT(_IO_I*A1-00*_AI16H_2_VAL.Quality);`
const dout = `_IO_Q*A1-00*_DO32P_2_VAL.Measurement := _TAG_DDVH;`

func TestAIPropertiesPairByPhysicalChannelAndCanBeReordered(t *testing.T) {
	first, err := ParseSheets([]iomap.Sheet{tinySheet("AI", xin, xs)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseSheets([]iomap.Sheet{tinySheet("AI", xs, xin)})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("pair depends on row order: %+v %+v", first, second)
	}
	channel := first.Groups[0].Modules[0].Channels[0]
	if channel.Channel != 2 || channel.Tag != "_TAG" || channel.SourceRow != 2 || channel.Reserve {
		t.Fatalf("bad pair %+v", channel)
	}
}

func TestDOExactDuplicatesCollapseButDistinctPhysicalTargetsStay(t *testing.T) {
	sheet := tinySheet("DO", dout, dout, strings.Replace(dout, "A1-00", "A1-01", 1))
	sheet.Rows[3].Cells["D"] = "A1-01"
	plan, err := ParseSheets([]iomap.Sheet{sheet})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || len(plan.Groups) != 1 || len(plan.Groups[0].Modules) != 2 || len(plan.Groups[0].Modules[0].Channels) != 1 {
		t.Fatalf("duplicate behavior %+v", plan)
	}
	if plan.Groups[0].Modules[0].Channels[0].Tag != plan.Groups[0].Modules[1].Channels[0].Tag {
		t.Fatal("repeated RHS changed")
	}
}

func TestDOOptionalMemberIsPreserved(t *testing.T) {
	sheet := tinySheet("DO", `_IO_Q*A1-00*_DO32P_2_VAL.Measurement := _TAG._02;`)
	plan, err := ParseSheets([]iomap.Sheet{sheet})
	if err != nil {
		t.Fatal(err)
	}
	channel := plan.Groups[0].Modules[0].Channels[0]
	if channel.Tag != "_TAG" || channel.Member != "_02" {
		t.Fatalf("member was lost: %+v", channel)
	}
}

func TestRejectIncompleteConflictingAndExecutableRows(t *testing.T) {
	tests := map[string]iomap.Sheet{
		"missing quality":         tinySheet("AI", xin),
		"missing value":           tinySheet("AI", xs),
		"duplicate AI target":     tinySheet("AI", xin, xs, xin),
		"wrong quality transform": tinySheet("AI", xin, strings.Replace(xs, "QUAL_STAT", "ANY_OTHER", 1)),
		"additional statement":    tinySheet("AI", xin+" _TAG.Xs := 0;", xs),
		"wrong physical channel":  tinySheet("AI", strings.Replace(xin, "AI16H_2", "AI16H_3", 1), xs),
		"wrong physical module":   tinySheet("AI", strings.Replace(xin, "A1-00", "A2-00", 1), xs),
		"wrong physical type":     tinySheet("AI", strings.Replace(xin, "AI16H", "DO32P", 1), xs),
		"conflicting DO target":   tinySheet("DO", dout, strings.Replace(dout, "_TAG_DDVH", "_OTHER_DDVH", 1)),
		"executable DO RHS":       tinySheet("DO", strings.Replace(dout, "_TAG_DDVH", "FUNC(_TAG_DDVH)", 1)),
		"out of bounds":           tinySheet("DO", strings.Replace(dout, "DO32P_2", "DO32P_32", 1)),
		"blank meaningful row":    tinySheet("AI", xin, xs, ""),
	}
	tests["conflicting DO target"].Rows[2].Cells["M"] = "_OTHER"
	tests["out of bounds"].Rows[1].Cells["E"] = "32"
	for name, sheet := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSheets([]iomap.Sheet{sheet}); err == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}

func TestRejectMetadataConflictsAndAmbiguousHeaders(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*iomap.Sheet)
	}{
		{"wrong mark", func(s *iomap.Sheet) { s.Rows[1].Cells["M"] = "_OTHER" }},
		{"contradictory pair", func(s *iomap.Sheet) { s.Rows[2].Cells["G"] = "A1-01" }},
		{"contradictory cabinet", func(s *iomap.Sheet) { s.Rows[2].Cells["C"] = "OTHER" }},
		{"wrong object type", func(s *iomap.Sheet) { s.Rows[1].Cells["J"] = "OTHER" }},
		{"missing mandatory header", func(s *iomap.Sheet) { delete(s.Rows[0].Cells, "E") }},
		{"duplicate header", func(s *iomap.Sheet) { s.Rows[0].Cells["N"] = "Module" }},
		{"ambiguous direction", func(s *iomap.Sheet) { s.Rows[0].Cells["N"] = "SCS DO" }},
		{"invalid controller ID", func(s *iomap.Sheet) { s.Rows[1].Cells["L"] = "x" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := tinySheet("AI", xin, xs)
			test.mutate(&s)
			if _, err := ParseSheets([]iomap.Sheet{s}); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func TestOneAITagCannotWriteFromDifferentPhysicalChannels(t *testing.T) {
	sheet := tinySheet("AI", xin, xs, strings.Replace(xin, "AI16H_2", "AI16H_3", 1), strings.Replace(xs, "AI16H_2", "AI16H_3", 1))
	sheet.Rows[3].Cells["E"], sheet.Rows[4].Cells["E"] = "3", "3"
	if _, err := ParseSheets([]iomap.Sheet{sheet}); err == nil {
		t.Fatal("multiple AI sources assigned to the same logical object")
	}
}

func TestBothKindsAndNumericCrateOrder(t *testing.T) {
	ai := tinySheet("AI", xin, xs)
	do := tinySheet("DO", strings.Replace(dout, "A1-00", "A10-01", 1))
	do.Rows[1].Cells["D"], do.Rows[1].Cells["G"], do.Rows[1].Cells["H"] = "A10-01", "A10-01", "A11-01"
	doTwo := tinySheet("DO", strings.Replace(dout, "A1-00", "A2-01", 1))
	doTwo.Rows[1].Cells["D"], doTwo.Rows[1].Cells["G"], doTwo.Rows[1].Cells["H"] = "A2-01", "A2-01", "A3-01"
	plan, err := ParseSheets([]iomap.Sheet{do, ai, doTwo})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Groups) != 3 || plan.Groups[0].Kind != "AI" || plan.Groups[1].Prefix != "A2" || plan.Groups[2].Prefix != "A10" {
		t.Fatalf("groups %+v", plan.Groups)
	}
}

func TestCorruptedTableHeaderCannotDisappearBesideValidTable(t *testing.T) {
	ai := tinySheet("AI", xin, xs)
	do := tinySheet("DO", dout)
	do.Rows[0].Cells["F"] = "SCS unsupported"
	if _, err := ParseSheets([]iomap.Sheet{ai, do}); err == nil {
		t.Fatal("a second assignment table with a broken header was silently skipped")
	}
}

func numberedModuleSheet(kind, number string) iomap.Sheet {
	expressions := []string{xin, xs}
	if kind == "DO" {
		expressions = []string{dout}
	}
	for i := range expressions {
		expressions[i] = strings.ReplaceAll(expressions[i], "A1-00", "A1-"+number)
		expressions[i] = strings.ReplaceAll(expressions[i], "_TAG", "_TAG_"+number)
	}
	sheet := tinySheet(kind, expressions...)
	for _, row := range sheet.Rows[1:] {
		row.Cells["D"], row.Cells["G"], row.Cells["H"] = "A1-"+number, "A1-"+number, "A2-"+number
		row.Cells["M"] = "_TAG_" + number
	}
	return sheet
}

func TestExtendedModuleNumbersAndNumericOrder(t *testing.T) {
	for _, kind := range []string{"AI", "DO"} {
		t.Run(kind, func(t *testing.T) {
			plan, err := ParseSheets([]iomap.Sheet{
				numberedModuleSheet(kind, "4095"),
				numberedModuleSheet(kind, "100"),
				numberedModuleSheet(kind, "99"),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Groups) != 1 || len(plan.Groups[0].Modules) != 3 {
				t.Fatalf("unexpected groups: %+v", plan.Groups)
			}
			for i, name := range []string{"A1-99", "A1-100", "A1-4095"} {
				module := plan.Groups[0].Modules[i]
				if module.Name != name || len(module.Channels) != 1 || module.Channels[0].Channel != 2 {
					t.Fatalf("module order or sparse channel changed: %+v", module)
				}
			}
			for _, number := range []string{"4096", "04095"} {
				if _, err := ParseSheets([]iomap.Sheet{numberedModuleSheet(kind, number)}); err == nil {
					t.Fatalf("accepted out-of-range module number %s", number)
				}
			}
		})
	}
}

func TestModuleNumberNormalizationKeepsAtLeastTwoDigits(t *testing.T) {
	for _, input := range []string{"A1-1", "A1_01", "A1-001", "A1-0001"} {
		name, prefix, slot, err := normalizeModule(input)
		if err != nil || name != "A1-01" || prefix != "A1" || slot != 1 {
			t.Fatalf("normalize %s: %s %s %d %v", input, name, prefix, slot, err)
		}
	}
}
