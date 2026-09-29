// Проверки ST сверяют выходные присваивания и неизменность подготовленных данных.
package generator_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
	"scheme-xml-generator/internal/generator"
	"scheme-xml-generator/internal/generator/addressing"
	"scheme-xml-generator/internal/generator/contracts"
	cpuprofile "scheme-xml-generator/internal/generator/controller"
	"scheme-xml-generator/internal/generator/planning"
	"scheme-xml-generator/internal/generator/xmlcodec"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/inputs/assignments"
	"strings"
	"testing"
)

// TestModuleSTNativeCPU850AssignmentsAndSnapshot сверяет адреса CPU850 и сводку ST; изменения исходной карты и запроса не должны менять готовый план.
func TestModuleSTNativeCPU850AssignmentsAndSnapshot(t *testing.T) {
	source := assignmentTestSource()
	request := assignmentTestRequest(source, "st")
	*request.POUs[0].ModuleIDs[1] = 24
	*request.POUs[1].ModuleIDs[0], *request.POUs[1].ModuleIDs[1] = 19, 20
	plans, err := planning.PrepareModulePlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].AssignmentCount != 70 || plans[0].RepeatedAssignmentCount != 1 || plans[0].SignalCount != 6 {
		t.Fatalf("counts %+v", plans[0])
	}
	source.Groups[0].Modules[0].Channels[0].Tag = "CORRUPTED"
	*request.POUs[0].ModuleIDs[0] = 999
	req, err := planning.RequirementsForController(plans[0])
	if err != nil || req != (contracts.DocumentRequirements{POUCount: 2, SignalCount: 6}) {
		t.Fatalf("requirements %+v: %v", req, err)
	}
	result, err := (generator.Generator{}).GenerateModuleMapping(plans[0], addressing.DefaultModuleContext(), contracts.IDRange{POUID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var doc xmlmodel.OutputSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Common.ControllerType != "TENIX-CPU850" || doc.Common.ControllerID != "189311" || doc.Common.ResourceID != "647" || len(doc.POUS.Items) != 2 {
		t.Fatalf("context/POUs %+v", doc)
	}
	for _, want := range []string{
		"PROGRAM AI_A1_channels", "_SENSOR_main.Xin := _IO_I0_AI16H_0_VAL.Measurement;",
		"_SENSOR_main.Xs := QUAL_STAT(_IO_I0_AI16H_0_VAL.Quality);",
		"_SPARE.Xin := _IO_I0_AI16H_15_VAL.Measurement;", "_SENSOR_reserve.Xin := _IO_I24_AI16H_2_VAL.Measurement;",
		"PROGRAM DO_A3_channels", "_IO_Q19_DO32P_0_VAL.Measurement := _PLC_A3_03._00;",
		"_IO_Q19_DO32P_31_VAL.Measurement := _PLC_A3_03._31;", "_IO_Q20_DO32P_5_VAL.Measurement := _PLC_A3_04._05;",
		"_IO_Q19_DO32P_1_VAL.Measurement := _PLC_A3_03._01;", "_IO_Q20_DO32P_31_VAL.Measurement := _PLC_A3_04._31;",
	} {
		if !bytes.Contains(result.XML, []byte(want)) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"CORRUPTED", "_IO_I999", "_IO_I0_AI16H_1_VAL", "REAL_TO_DINT", "<ISAGraf", "<ISACARDSINFO", "<ISAOBJSINFO"} {
		if bytes.Contains(result.XML, []byte(forbidden)) {
			t.Fatalf("unexpected %q", forbidden)
		}
	}
	if result.Summary.Cards != 0 || result.Summary.Blocks != 0 || result.Summary.T11First != 0 || result.Summary.POUs[0].POUNumber != "4" || result.Summary.POUs[1].POUNumber != "5" {
		t.Fatalf("summary %+v", result.Summary)
	}
}

// TestModuleSTControllerProfilesPreserveSparseAIAndMapAllDOChannels сверяет весь ST-текст AI/DO и аппаратные префиксы для подтверждённых сочетаний CPU и драйвера.
func TestModuleSTControllerProfilesPreserveSparseAIAndMapAllDOChannels(t *testing.T) {
	source := assignmentTestSource()
	request := assignmentTestRequest(source, "st")
	*request.POUs[0].ModuleIDs[1] = 24
	*request.POUs[1].ModuleIDs[0], *request.POUs[1].ModuleIDs[1] = 19, 20
	for index := range request.POUs {
		request.POUs[index].ModuleCount = assignmentTestModuleCount(3)
		id := int64(100 + index)
		request.POUs[index].ModuleIDs = append(request.POUs[index].ModuleIDs, &id)
	}
	plans, err := planning.PrepareModulePlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	for _, tc := range []struct {
		name, cpu, profile string
		measurement        bool
	}{
		{"715 default", cpuprofile.ControllerCPU715, "", false},
		{"715 explicit legacy", cpuprofile.ControllerCPU715, addressing.PhysicalProfileLegacy, false},
		{"850 default", cpuprofile.ControllerCPU850, "", true},
		{"850 explicit measurement", cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement, true},
		{"850 explicit legacy", cpuprofile.ControllerCPU850, addressing.PhysicalProfileLegacy, false},
		{"trimmed context", " " + cpuprofile.ControllerCPU715 + " ", " " + addressing.PhysicalProfileLegacy + " ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := addressing.DefaultModuleContext()
			ctx.ControllerTypeName, ctx.PhysicalProfile = tc.cpu, tc.profile
			result, err := (generator.Generator{}).GenerateModuleMapping(plan, ctx, contracts.IDRange{POUID: 1000})
			if err != nil {
				t.Fatal(err)
			}
			var doc xmlmodel.OutputSTDocument
			if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Common.ControllerType != strings.TrimSpace(tc.cpu) || len(doc.POUS.Items) != 2 || doc.POUS.Items[0].ID != "1000" || doc.POUS.Items[1].ID != "1001" {
				t.Fatalf("controller or POU IDs changed: %+v", doc)
			}
			wantAI := "PROGRAM AI_A1_channels\n\n(* A1_00 *)\n_SENSOR_main.Xin := _IO_IU0_0.ValueDINT;\n_SENSOR_main.Xs := _IO_IU0_0.Status;\n_SPARE.Xin := _IO_IU0_15.ValueDINT;\n_SPARE.Xs := _IO_IU0_15.Status;\n\n(* A1_02 *)\n_SENSOR_reserve.Xin := _IO_IU24_2.ValueDINT;\n_SENSOR_reserve.Xs := _IO_IU24_2.Status;\n\n(* A1_03 *)\n"
			wantDO := "PROGRAM DO_A3_channels\n\n"
			aiPrefix, doPrefix := "_IO_IU100", "_IO_QU101"
			profile := addressing.PhysicalProfileLegacy
			if tc.measurement {
				wantAI = "PROGRAM AI_A1_channels\n\n(* A1_00 *)\n_SENSOR_main.Xin := _IO_I0_AI16H_0_VAL.Measurement;\n_SENSOR_main.Xs := QUAL_STAT(_IO_I0_AI16H_0_VAL.Quality);\n_SPARE.Xin := _IO_I0_AI16H_15_VAL.Measurement;\n_SPARE.Xs := QUAL_STAT(_IO_I0_AI16H_15_VAL.Quality);\n\n(* A1_02 *)\n_SENSOR_reserve.Xin := _IO_I24_AI16H_2_VAL.Measurement;\n_SENSOR_reserve.Xs := QUAL_STAT(_IO_I24_AI16H_2_VAL.Quality);\n\n(* A1_03 *)\n"
				aiPrefix, doPrefix, profile = "_IO_I100_AI16H", "_IO_Q101_DO32P", addressing.PhysicalProfileMeasurement
			}
			for channel := 0; channel < 16; channel++ {
				if tc.measurement {
					wantAI += fmt.Sprintf("_PLC_A1_03_%d.Xin := _IO_I100_AI16H_%d_VAL.Measurement;\n_PLC_A1_03_%d.Xs := QUAL_STAT(_IO_I100_AI16H_%d_VAL.Quality);\n", channel, channel, channel, channel)
				} else {
					wantAI += fmt.Sprintf("_PLC_A1_03_%d.Xin := _IO_IU100_%d.ValueDINT;\n_PLC_A1_03_%d.Xs := _IO_IU100_%d.Status;\n", channel, channel, channel, channel)
				}
			}
			for index, id := range []int{19, 20, 101} {
				wantDO += fmt.Sprintf("(* A3_%02d *)\n", index+3)
				for channel := 0; channel < 32; channel++ {
					if tc.measurement {
						wantDO += fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := _PLC_A3_%02d._%02d;\n", id, channel, index+3, channel)
					} else {
						wantDO += fmt.Sprintf("_IO_QU%d_%d.Value := _PLC_A3_%02d._%02d;\n", id, channel, index+3, channel)
					}
				}
				wantDO += "\n"
			}
			wantAI += "\nEND_PROGRAM"
			wantDO += "END_PROGRAM"
			if doc.POUS.Items[0].Code != wantAI || doc.POUS.Items[1].Code != wantDO {
				t.Fatalf("ST physical profile, sparse AI or full DO assignments changed:\nAI: %s\nDO: %s", doc.POUS.Items[0].Code, doc.POUS.Items[1].Code)
			}
			if strings.Count(doc.POUS.Items[0].Code+doc.POUS.Items[1].Code, ":=") != 134 || result.Summary.SignalCount != 54 || result.Summary.IOModuleCount != 6 || result.Summary.POUs[0].IOModules[2].BindingPrefix != aiPrefix || result.Summary.POUs[1].IOModules[2].BindingPrefix != doPrefix {
				t.Fatalf("physical summary mismatch: %+v", result.Summary)
			}
			warnings := strings.Join(result.Warnings, "\n")
			if strings.Contains(warnings, "QUAL_STAT") != tc.measurement || !strings.Contains(warnings, profile) || strings.Contains(warnings, "нативный ST-эталон не предоставлен") == tc.measurement {
				t.Fatal("wrong dependency or verification warning", warnings)
			}
		})
	}
}

// TestModuleDOFullPhysicalChannelsWithMissingTailAndHoles проверяет каждое из 32 физических DO-присваиваний независимо от пропусков логических сигналов.
func TestModuleDOFullPhysicalChannelsWithMissingTailAndHoles(t *testing.T) {
	source := &assignments.Plan{Groups: []assignments.Group{{Key: "PLC:DO:A33", ControllerName: "PLC", Kind: "DO", Prefix: "A33", POUName: "DO_A33", Modules: []assignments.Module{
		{Name: "A33-08", Type: "DO32P", ObjectType: "D32V", Capacity: 32},
		{Name: "A33-09", Type: "DO32P", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{
			{Channel: 0, Tag: "_OUTPUT_0_DDVH", SourceRow: 30},
			{Channel: 17, Tag: "_HOLE_DDVH", SourceRow: 31},
			{Channel: 31, Tag: "_LAST_DDVH", SourceRow: 32},
		}},
	}}}}
	for channel := 0; channel < 28; channel++ {
		source.Groups[0].Modules[0].Channels = append(source.Groups[0].Modules[0].Channels, assignments.Channel{Channel: channel, Tag: fmt.Sprintf("_OUTPUT_%d_DDVH", channel), SourceRow: channel + 2})
	}
	snapshot := make([][]assignments.Channel, 2)
	for i, module := range source.Groups[0].Modules {
		snapshot[i] = append([]assignments.Channel(nil), module.Channels...)
	}
	plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, "st"))
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	if plan.SignalCount != 31 || plan.AssignmentCount != 64 || plan.RepeatedAssignmentCount != 1 {
		t.Fatal("logical mappings and physical assignments must be counted separately", plan)
	}
	for _, tc := range []struct{ cpu, profile string }{
		{cpuprofile.ControllerCPU715, ""}, {cpuprofile.ControllerCPU715, addressing.PhysicalProfileLegacy},
		{cpuprofile.ControllerCPU850, ""}, {cpuprofile.ControllerCPU850, addressing.PhysicalProfileMeasurement}, {cpuprofile.ControllerCPU850, addressing.PhysicalProfileLegacy},
	} {
		t.Run(tc.cpu+"/"+tc.profile, func(t *testing.T) {
			ctx := addressing.DefaultModuleContext()
			ctx.ControllerTypeName, ctx.PhysicalProfile = tc.cpu, tc.profile
			result, err := (generator.Generator{}).GenerateModuleMapping(plan, ctx, contracts.IDRange{POUID: 100})
			if err != nil {
				t.Fatal(err)
			}
			var document xmlmodel.OutputSTDocument
			if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, xmlcodec.Utf8BOM), &document); err != nil {
				t.Fatal(err)
			}
			code := document.POUS.Items[0].Code
			if strings.Count(code, ":=") != 64 || strings.Contains(code, "_DDVH") || result.Summary.SignalCount != 31 || len(result.Summary.POUs[0].Signals) != 31 {
				t.Fatal("full physical ST must not invent logical signals", code, result.Summary)
			}
			for module := 0; module < 2; module++ {
				if result.Summary.POUs[0].IOModules[module].SignalCount != len(snapshot[module]) {
					t.Fatal("module summary must retain map signal count")
				}
				for channel := 0; channel < 32; channel++ {
					want := fmt.Sprintf("_IO_QU%d_%d.Value := _PLC_A33_%02d._%02d;", module, channel, module+8, channel)
					if tc.cpu == cpuprofile.ControllerCPU850 && tc.profile != addressing.PhysicalProfileLegacy {
						want = fmt.Sprintf("_IO_Q%d_DO32P_%d_VAL.Measurement := _PLC_A33_%02d._%02d;", module, channel, module+8, channel)
					}
					if strings.Count(code, want) != 1 {
						t.Fatal("missing or repeated physical channel", want)
					}
				}
			}
		})
	}
	for i, module := range source.Groups[0].Modules {
		if !reflect.DeepEqual(module.Channels, snapshot[i]) || !reflect.DeepEqual(plan.POUs[0].Modules[i].Channels, snapshot[i]) {
			t.Fatal("ST expanded source or snapshot logical channels")
		}
	}
}

// TestModuleDIC1C2ShareOwnerAcrossPOUs Проверяет независимые физические назначения C1/C2 одного DI-объекта в разных POU.
func TestModuleDIC1C2ShareOwnerAcrossPOUs(t *testing.T) {
	for _, kind := range []string{"st"} {
		source := &assignments.Plan{Groups: []assignments.Group{
			{Key: "PLC:DI:A11", ControllerName: "PLC", Kind: "DI", Prefix: "A11", POUName: "DI_A11", Modules: []assignments.Module{{Name: "A11-05", Type: "DI32", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 0, Tag: "_LZS_MAIN", Member: "C1", SourceRow: 2}}}}},
			{Key: "PLC:DI:A12", ControllerName: "PLC", Kind: "DI", Prefix: "A12", POUName: "DI_A12", Modules: []assignments.Module{{Name: "A12-01", Type: "DI32", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 17, Tag: "_lzs_main", Member: "C2", SourceRow: 3}}}}},
		}}
		plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind))
		if err != nil {
			t.Fatal(err)
		}
		plan := plans[0]
		if plan.SignalCount != 2 || plan.AssignmentCount != 66 || plan.RepeatedAssignmentCount != 0 {
			t.Fatalf("paired receiver counts: %+v", plan)
		}
		result, err := (generator.Generator{}).GenerateModuleMapping(plan, addressing.DefaultModuleContext(), contracts.IDRange{POUID: 100, T11Start: 200, CardStart: 300})
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(result.XML, []byte(":=")) != 66 || bytes.Contains(result.XML, []byte("_LZS_MAIN")) || bytes.Contains(result.XML, []byte(".C2")) {
			t.Fatal("paired DDR fields must not alter module ST mapping")
		}
		for _, member := range []string{"C1", "C2"} {
			source.Groups[0].Modules[0].Channels[0].Member = member
			source.Groups[1].Modules[0].Channels[0].Member = member
			if _, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind)); err == nil {
				t.Fatal("accepted case-insensitive duplicate target", member, kind)
			}
		}
	}
}

// TestModuleDIEmptySpareOnlyModules Проверяет ST для пустого резервного DI-модуля: все физические входы и статус сохраняются.
func TestModuleDIEmptySpareOnlyModules(t *testing.T) {
	for _, kind := range []string{"st"} {
		source := assignmentDITestSource()
		source.Groups[0].Modules[0].Channels = nil
		source.Groups[0].Modules[1].Channels = []assignments.Channel{}
		plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind))
		if err != nil {
			t.Fatal(err)
		}
		plan := plans[0]
		if plan.ModuleCount != 2 || plan.SignalCount != 0 || plan.AssignmentCount != 66 || plan.RepeatedAssignmentCount != 0 {
			t.Fatalf("spare-only counts: %+v", plan)
		}
		result, err := (generator.Generator{}).GenerateModuleMapping(plan, addressing.DefaultModuleContext(), contracts.IDRange{POUID: 100, T11Start: 200, CardStart: 300})
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(result.XML, []byte(":=")) != 66 || !bytes.Contains(result.XML, []byte("_PLC_A11_05.i31 := _IO_I0_DI32_31_VAL.Measurement;")) || !bytes.Contains(result.XML, []byte("_PLC_A11_07.Stat := ANY_TO_DWORD (QUAL_STAT(_IO_I1_DI32_0_VAL.Quality));")) {
			t.Fatal("spare-only modules must preserve all physical ST assignments")
		}
		for group := range assignmentTestSource().Groups {
			other := assignmentTestSource()
			other.Groups[group].Modules[0].Channels = nil
			if _, err := planning.PrepareModulePlans(other, assignmentTestRequest(other, kind)); err == nil {
				t.Fatal("empty AI/DO module must still be rejected", group, kind)
			}
		}
	}
}
