package generator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"scheme-xml-generator/internal/skzmap"
)

func skzTestSource() *skzmap.Plan {
	return &skzmap.Plan{Groups: []skzmap.Group{
		{Key: "PLC:AI:A1", SCS: "PLC", Kind: "AI", Prefix: "A1", POUName: "AI_A1", Modules: []skzmap.Module{
			{Name: "A1-00", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []skzmap.Channel{{Channel: 0, Tag: "_SENSOR_main", SourceRow: 2}, {Channel: 15, Tag: "_SPARE", Reserve: true, SourceRow: 4}}},
			{Name: "A1-02", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []skzmap.Channel{{Channel: 2, Tag: "_SENSOR_reserve", SourceRow: 6}}},
		}},
		{Key: "PLC:DO:A3", SCS: "PLC", Kind: "DO", Prefix: "A3", POUName: "DO_A3", Modules: []skzmap.Module{
			{Name: "A3-03", Type: "DO32P", ObjectType: "D32V", Capacity: 32, Channels: []skzmap.Channel{{Channel: 0, Tag: "_OUTPUT_DDVH", SourceRow: 2}, {Channel: 31, Tag: "_ALARM_DDVH", SourceRow: 3}}},
			{Name: "A3-04", Type: "DO32P", ObjectType: "D32V", Capacity: 32, Channels: []skzmap.Channel{{Channel: 5, Tag: "_OUTPUT_DDVH", SourceRow: 4}}},
		}},
	}}
}

func skzTestRequest(source *skzmap.Plan, kind string) SKZRequest {
	request := SKZRequest{Kind: kind}
	nextID := int64(0)
	for _, group := range source.Groups {
		choice := SKZPOURequest{GroupKey: group.Key}
		for range group.Modules {
			id := nextID
			if kind == "st" {
				choice.ModuleIDs = append(choice.ModuleIDs, &id)
			}
			nextID++
		}
		request.POUs = append(request.POUs, choice)
	}
	return request
}

func skzTestPlans(t *testing.T, kind string) []SKZPlan {
	t.Helper()
	source := skzTestSource()
	plans, err := PrepareSKZPlans(source, skzTestRequest(source, kind))
	if err != nil {
		t.Fatal(err)
	}
	return plans
}

func skzTestModuleCount(count int) *int { return &count }

func skzTestGroupModules(plan SKZPlan, key string) []SKZModule {
	var modules []SKZModule
	for _, pou := range plan.POUs {
		if pou.GroupKey == key {
			modules = append(modules, pou.Modules...)
		}
	}
	return modules
}

func TestSKZAdditionalModulesPreserveSparseSourceAndMatchSTFBD(t *testing.T) {
	for _, kind := range []string{"st", "fbd"} {
		t.Run(kind, func(t *testing.T) {
			source := skzTestSource()
			request := skzTestRequest(source, kind)
			for index := range request.POUs {
				request.POUs[index].ModuleCount = skzTestModuleCount(3)
				if kind == "st" {
					id := int64(100 + index)
					request.POUs[index].ModuleIDs = append(request.POUs[index].ModuleIDs, &id)
				}
			}
			plans, err := PrepareSKZPlans(source, request)
			if err != nil {
				t.Fatal(err)
			}
			plan := plans[0]
			if plan.ModuleCount != 6 || plan.SignalCount != 54 || plan.AssignmentCount != 73 || plan.RepeatedAssignmentCount != 1 {
				t.Fatalf("counts %+v", plan)
			}
			for index, group := range source.Groups {
				modules := skzTestGroupModules(plan, group.Key)
				for j, original := range group.Modules {
					if modules[j].Name != original.Name || !reflect.DeepEqual(modules[j].Channels, original.Channels) {
						t.Fatal("source module or sparse channels changed")
					}
				}
				added := modules[2]
				name, capacity := "A1-03", 16
				if index == 1 {
					name, capacity = "A3-05", 32
				}
				if added.Name != name || added.Capacity != capacity || len(added.Channels) != capacity {
					t.Fatalf("new module %+v", added)
				}
				for channel, actual := range added.Channels {
					want := skzmap.Channel{Channel: channel, Tag: fmt.Sprintf("%s_%d", skzModuleTag("PLC", name), channel), Reserve: true}
					if actual != want {
						t.Fatalf("reserve channel %+v, want %+v", actual, want)
					}
				}
				*request.POUs[index].ModuleCount = 1
				if kind == "st" {
					*request.POUs[index].ModuleIDs[2] = 999
				}
			}
			source.Groups[0].Modules[0].Channels[0].Tag = "CORRUPTED"
			req, err := RequirementsForSKZ(plan)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (Generator{}).GenerateSKZ(plan, DefaultSKZContext(), IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000})
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(result.XML, []byte("CORRUPTED")) || bytes.Contains(result.XML, []byte("_IO_I999")) || bytes.Contains(result.XML, []byte("_IO_Q999")) {
				t.Fatal("plan snapshot changed")
			}
			if kind == "st" {
				if req != (DocumentRequirements{POUCount: 2, SignalCount: 54}) {
					t.Fatal(req)
				}
				for channel := 0; channel < 16; channel++ {
					for _, want := range []string{
						fmt.Sprintf("_PLC_A1_03_%d.Xin := _IO_I100_AI16H_%d_VAL.Measurement;", channel, channel),
						fmt.Sprintf("_PLC_A1_03_%d.Xs := QUAL_STAT(_IO_I100_AI16H_%d_VAL.Quality);", channel, channel),
					} {
						if !bytes.Contains(result.XML, []byte(want)) {
							t.Fatalf("missing %q", want)
						}
					}
				}
				for channel := 0; channel < 32; channel++ {
					want := fmt.Sprintf("_IO_Q101_DO32P_%d_VAL.Measurement := _PLC_A3_05._%02d;", channel, channel)
					if !bytes.Contains(result.XML, []byte(want)) {
						t.Fatalf("missing %q", want)
					}
				}
				if bytes.Contains(result.XML, []byte("_IO_I0_AI16H_1_VAL")) || bytes.Contains(result.XML, []byte("_IO_Q2_DO32P_1_VAL")) {
					t.Fatal("filled an unlisted source channel")
				}
			} else {
				if req != (DocumentRequirements{POUCount: 4, SignalCount: 54, T11Count: 180, CardCount: 62}) || result.Summary.Blocks != 139 || result.Summary.Graphics != 38 || result.Summary.Links != 3 {
					t.Fatalf("requirements %+v; summary %+v", req, result.Summary)
				}
				var doc outputDocument
				if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
					t.Fatal(err)
				}
				calls := map[string]bool{}
				for _, pou := range doc.POUS.Items {
					for _, block := range pou.ISAGraf.Blocks.Items {
						if block.ObjectType == "37" {
							calls[block.Info] = true
						}
					}
				}
				for channel := 0; channel < 16; channel++ {
					if !calls[fmt.Sprintf("_PLC_A1_03_%d", channel)] {
						t.Fatal("new AI reserve object missing", channel)
					}
				}
				if !calls["_PLC_A3_05"] || bytes.Contains(result.XML, []byte("_PLC_A3_05_")) {
					t.Fatal("new DO must have only its D32V object")
				}
			}
		})
	}
}

func TestSKZModuleCountAndNewIDsGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SKZRequest)
	}{
		{"zero", func(r *SKZRequest) { r.POUs[0].ModuleCount = skzTestModuleCount(0) }},
		{"negative", func(r *SKZRequest) { r.POUs[0].ModuleCount = skzTestModuleCount(-1) }},
		{"below source", func(r *SKZRequest) { r.POUs[0].ModuleCount = skzTestModuleCount(1) }},
		{"above cap", func(r *SKZRequest) { r.POUs[0].ModuleCount = skzTestModuleCount(4097) }},
		{"new ID absent", func(r *SKZRequest) { r.POUs[0].ModuleCount = skzTestModuleCount(3) }},
		{"new ID null", func(r *SKZRequest) {
			r.POUs[0].ModuleCount = skzTestModuleCount(3)
			r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, nil)
		}},
		{"new ID duplicate", func(r *SKZRequest) {
			r.POUs[0].ModuleCount = skzTestModuleCount(3)
			id := int64(2)
			r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, &id)
		}},
		{"extra ID", func(r *SKZRequest) { id := int64(100); r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, &id) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := skzTestSource()
			request := skzTestRequest(source, "st")
			tc.change(&request)
			if _, err := PrepareSKZPlans(source, request); err == nil {
				t.Fatal("accepted invalid module count or IDs")
			}
		})
	}
}

func TestSKZAppendsAfterHighestSlotAndRejectsModuleCollision(t *testing.T) {
	source := skzTestSource()
	source.Groups = source.Groups[:1]
	source.Groups[0].Modules[0].Name = "A1-99"
	request := skzTestRequest(source, "fbd")
	request.POUs[0].ModuleCount = skzTestModuleCount(3)
	request.POUs[0].ModuleIDs = []*int64{nil}
	plans, err := PrepareSKZPlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	added := plans[0].POUs[2].Modules[0]
	if added.Name != "A1-100" || added.Channels[15].Tag != "_PLC_A1_100_15" || added.ID != nil {
		t.Fatalf("new module %+v", added)
	}
	for _, name := range []string{"A1-000", "A1-4095", "A1-4096"} {
		source.Groups[0].Modules[0].Name = name
		if _, err := PrepareSKZPlans(source, request); err == nil {
			t.Fatal("accepted invalid or overflowing slot", name)
		}
	}
	source = skzTestSource()
	source.Groups[1].Key, source.Groups[1].Prefix, source.Groups[1].POUName = "PLC:DO:A1", "A1", "DO_A1"
	source.Groups[1].Modules[0].Name, source.Groups[1].Modules[1].Name = "A1-03", "A1-04"
	request = skzTestRequest(source, "fbd")
	if _, err := PrepareSKZPlans(source, request); err != nil {
		t.Fatal("nonoverlapping source modules should be accepted", err)
	}
	request.POUs[0].ModuleCount = skzTestModuleCount(3)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("accepted collision with another selected group's module")
	}
	request.POUs = request.POUs[:1]
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("accepted collision with an unselected source group's module")
	}
}

func TestSKZExpandedChannelLimitAcrossControllers(t *testing.T) {
	source := skzTestSource()
	source.Groups = source.Groups[1:]
	source.Groups[0].Modules = source.Groups[0].Modules[:1]
	source.Groups[0].Modules[0].Channels = nil
	for channel := 0; channel < 32; channel++ {
		source.Groups[0].Modules[0].Channels = append(source.Groups[0].Modules[0].Channels, skzmap.Channel{Channel: channel, Tag: fmt.Sprintf("_SOURCE_%d", channel)})
	}
	request := skzTestRequest(source, "fbd")
	request.POUs[0].ModuleCount = skzTestModuleCount(128)
	plans, err := PrepareSKZPlans(source, request)
	if err != nil || plans[0].SignalCount != 4096 {
		t.Fatalf("boundary plans: %v", err)
	}
	extra := plans[0].POUs[0].Modules[127]
	extra.Name = "A3-131"
	plans[0].POUs[0].Modules = append(plans[0].POUs[0].Modules, extra)
	if _, err := RequirementsForSKZ(plans[0]); err == nil {
		t.Fatal("accepted direct plan above 4096 channels")
	}
	request.POUs[0].ModuleCount = skzTestModuleCount(129)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("accepted expansion above 4096 channels")
	}
	other := source.Groups[0]
	other.SCS, other.Key = "OTHER", "OTHER:DO:A3"
	source.Groups = append(source.Groups, other)
	request = skzTestRequest(source, "fbd")
	for index := range request.POUs {
		request.POUs[index].ModuleCount = skzTestModuleCount(64)
	}
	if plans, err := PrepareSKZPlans(source, request); err != nil || len(plans) != 2 || plans[0].SignalCount+plans[1].SignalCount != 4096 {
		t.Fatalf("global boundary: %v", err)
	}
	request.POUs[1].ModuleCount = skzTestModuleCount(65)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("per-controller counts bypassed the global channel limit")
	}
}

func TestSKZAIFBDExpandedPOULimit(t *testing.T) {
	source := skzTestSource()
	request := skzTestRequest(source, "fbd")
	request.POUs[0].ModuleCount = skzTestModuleCount(127)
	plans, err := PrepareSKZPlans(source, request)
	if err != nil || len(plans[0].POUs) != 128 {
		t.Fatalf("127 AI module POUs plus one DO POU: %v", err)
	}
	req, err := RequirementsForSKZ(plans[0])
	if err != nil || req.POUCount != 128 {
		t.Fatalf("actual POU reservation %+v: %v", req, err)
	}
	request.POUs[0].ModuleCount = skzTestModuleCount(128)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("accepted 129 POU after AI expansion")
	}
	request.POUs = request.POUs[:1]
	if plans, err := PrepareSKZPlans(source, request); err != nil || len(plans[0].POUs) != 128 {
		t.Fatalf("128 AI module boundary: %v", err)
	}
	request.POUs[0].ModuleCount = skzTestModuleCount(129)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("accepted 129 AI module POUs")
	}
	source.Groups = source.Groups[:1]
	other := source.Groups[0]
	other.SCS, other.Key = "OTHER", "OTHER:AI:A1"
	source.Groups = append(source.Groups, other)
	request = skzTestRequest(source, "fbd")
	for index := range request.POUs {
		request.POUs[index].ModuleCount = skzTestModuleCount(64)
	}
	if plans, err := PrepareSKZPlans(source, request); err != nil || len(plans) != 2 || len(plans[0].POUs)+len(plans[1].POUs) != 128 {
		t.Fatalf("global 128 POU boundary: %v", err)
	}
	request.POUs[1].ModuleCount = skzTestModuleCount(65)
	if _, err := PrepareSKZPlans(source, request); err == nil {
		t.Fatal("controller separation bypassed the global POU limit")
	}
}

func TestSKZAIFBDPlanMustHaveOneMatchingModulePerPOU(t *testing.T) {
	for _, change := range []func(*SKZPlan){
		func(p *SKZPlan) { p.POUs[0].Name = "AI_A1" },
		func(p *SKZPlan) { p.POUs[0].Name = "AI_A1_02" },
		func(p *SKZPlan) { p.POUs[0].Modules = append(p.POUs[0].Modules, p.POUs[1].Modules...) },
		func(p *SKZPlan) { p.POUs[0].Modules = nil },
	} {
		plan := skzTestPlans(t, "fbd")[0]
		change(&plan)
		if _, err := RequirementsForSKZ(plan); err == nil {
			t.Fatal("accepted AI FBD POU with wrong name or module composition")
		}
	}
}

func TestSKZSourceDOReserveKeepsItsLogicalBinding(t *testing.T) {
	for _, expand := range []bool{false, true} {
		source := skzTestSource()
		// A real Excel row labelled "Резерв" still carries a logical BOOL tag.
		source.Groups[1].Modules[0].Channels[0].Reserve = true
		request := skzTestRequest(source, "fbd")
		if expand {
			request.POUs[1].ModuleCount = skzTestModuleCount(3)
		}
		plans, err := PrepareSKZPlans(source, request)
		if err != nil {
			t.Fatal(err)
		}
		req, err := RequirementsForSKZ(plans[0])
		if err != nil {
			t.Fatal(err)
		}
		want := DocumentRequirements{POUCount: 3, SignalCount: 6, T11Count: 35, CardCount: 13}
		if expand {
			want.SignalCount += 32
			want.T11Count++
			want.CardCount++
		}
		if req != want {
			t.Fatalf("expanded=%v: requirements %+v, want %+v", expand, req, want)
		}
		result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000})
		if err != nil {
			t.Fatal(err)
		}
		if result.Summary.Links != 3 || bytes.Count(result.XML, []byte(`GROBJTYPE="31" Info="_OUTPUT_DDVH"`)) != 2 {
			t.Fatal("source reserve BOOL or its link was omitted")
		}
	}
}

func TestSKZSyntheticAIReserveCannotReuseSourceTag(t *testing.T) {
	for _, kind := range []string{"st", "fbd"} {
		for _, unselected := range []bool{false, true} {
			source := skzTestSource()
			source.Groups = source.Groups[:1]
			source.Groups[0].Modules = source.Groups[0].Modules[:1]
			collision := "_PLC_A1_01_0"
			if unselected {
				other := skzmap.Group{Key: "PLC:AI:A9", SCS: "PLC", Kind: "AI", Prefix: "A9", POUName: "AI_A9", Modules: []skzmap.Module{
					{Name: "A9-00", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []skzmap.Channel{{Channel: 0, Tag: strings.ToLower(collision), SourceRow: 9}}},
				}}
				source.Groups = append(source.Groups, other)
			} else {
				source.Groups[0].Modules[0].Channels[0].Tag = collision
			}
			request := skzTestRequest(source, kind)
			request.POUs = request.POUs[:1]
			request.POUs[0].ModuleCount = skzTestModuleCount(2)
			if kind == "st" {
				id := int64(100)
				request.POUs[0].ModuleIDs = append(request.POUs[0].ModuleIDs, &id)
			}
			if _, err := PrepareSKZPlans(source, request); err == nil {
				t.Fatalf("%s, unselected=%v: synthetic AI reused a source object", kind, unselected)
			}
			if unselected {
				source.Groups[1].SCS = "OTHER"
				if _, err := PrepareSKZPlans(source, request); err != nil {
					t.Fatal("unselected controller must not block another controller's names", err)
				}
			}
		}
		for _, reverse := range []bool{false, true} {
			source := skzTestSource()
			request := skzTestRequest(source, kind)
			request.POUs[0].ModuleCount = skzTestModuleCount(3)
			if kind == "st" {
				id := int64(100)
				request.POUs[0].ModuleIDs = append(request.POUs[0].ModuleIDs, &id)
			}
			plans, err := PrepareSKZPlans(source, request)
			if err != nil {
				t.Fatal(err)
			}
			modules := skzTestGroupModules(plans[0], request.POUs[0].GroupKey)
			modules[0].Channels[0].Tag = modules[2].Channels[0].Tag
			if reverse {
				if kind == "fbd" {
					plans[0].POUs[0], plans[0].POUs[2] = plans[0].POUs[2], plans[0].POUs[0]
				} else {
					plans[0].POUs[0].Modules[0], plans[0].POUs[0].Modules[2] = plans[0].POUs[0].Modules[2], plans[0].POUs[0].Modules[0]
				}
			}
			if _, err := RequirementsForSKZ(plans[0]); err == nil {
				t.Fatalf("%s, reverse=%v: direct plan reused a synthetic AI object", kind, reverse)
			}
		}
	}
}

func TestSKZSTNativeCPU850AssignmentsAndSnapshot(t *testing.T) {
	source := skzTestSource()
	request := skzTestRequest(source, "st")
	*request.POUs[0].ModuleIDs[1] = 24
	*request.POUs[1].ModuleIDs[0], *request.POUs[1].ModuleIDs[1] = 19, 20
	plans, err := PrepareSKZPlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].AssignmentCount != 9 || plans[0].RepeatedAssignmentCount != 1 || plans[0].SignalCount != 6 {
		t.Fatalf("counts %+v", plans[0])
	}
	source.Groups[0].Modules[0].Channels[0].Tag = "CORRUPTED"
	*request.POUs[0].ModuleIDs[0] = 999
	req, err := RequirementsForSKZ(plans[0])
	if err != nil || req != (DocumentRequirements{POUCount: 2, SignalCount: 6}) {
		t.Fatalf("requirements %+v: %v", req, err)
	}
	result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var doc outputAOSTDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
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
	} {
		if !bytes.Contains(result.XML, []byte(want)) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"CORRUPTED", "_IO_I999", "_IO_Q19_DO32P_1_VAL", "_IO_I0_AI16H_1_VAL", "REAL_TO_DINT", "<ISAGraf", "<ISACARDSINFO", "<ISAOBJSINFO"} {
		if bytes.Contains(result.XML, []byte(forbidden)) {
			t.Fatalf("unexpected %q", forbidden)
		}
	}
	if result.Summary.Cards != 0 || result.Summary.Blocks != 0 || result.Summary.T11First != 0 || result.Summary.POUs[0].POUNumber != "4" || result.Summary.POUs[1].POUNumber != "5" {
		t.Fatalf("summary %+v", result.Summary)
	}
}

func TestSKZFBDUsesNativeCallsAndDOInputWiring(t *testing.T) {
	plans := skzTestPlans(t, "fbd")
	req, err := RequirementsForSKZ(plans[0])
	if err != nil || req != (DocumentRequirements{POUCount: 3, SignalCount: 6, T11Count: 35, CardCount: 13}) {
		t.Fatalf("requirements %+v: %v", req, err)
	}
	ids := IDRange{T11Start: 100000, CardStart: 200000, POUID: 300000}
	result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), ids)
	if err != nil {
		t.Fatal(err)
	}
	var doc outputDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Blocks != 26 || result.Summary.Links != 3 || result.Summary.Graphics != 6 || result.Summary.Cards != 13 || result.Summary.T11Last != ids.T11Start+34 {
		t.Fatalf("summary %+v", result.Summary)
	}
	for index, pou := range doc.POUS.Items {
		if pou.ID != fmt.Sprint(ids.POUID+int64(index)) || pou.Number != fmt.Sprint(4+index) || result.Summary.POUs[index].POUID != ids.POUID+int64(index) || result.Summary.POUs[index].POUNumber != pou.Number {
			t.Fatal("split POU identifiers/numbers are not sequential", pou.ID, pou.Number)
		}
	}
	if len(doc.POUS.Items) != 3 || doc.POUS.Items[0].Name != "AI_A1_00" || doc.POUS.Items[1].Name != "AI_A1_02" || doc.POUS.Items[2].Name != "DO_A3" || bytes.Contains(result.XML, []byte("_channels")) || bytes.Contains(result.XML, []byte("_IO_")) || bytes.Contains(result.XML, []byte("<STCODE")) {
		t.Fatal("FBD has ST names or physical assignments")
	}
	types := map[string]string{}
	for _, typ := range doc.ISAObjects.Items {
		types[typ.ID] = typ.Info
	}
	if !reflect.DeepEqual(types, map[string]string{"17480": "AD3_v2", "1933": "D32V_v1"}) {
		t.Fatal(types)
	}
	ai := doc.POUS.Items[0].ISAGraf.Blocks.Items
	aiSecond := doc.POUS.Items[1].ISAGraf.Blocks.Items
	if len(ai) != 14 || len(aiSecond) != 7 || ai[0].Info != "_SENSOR_main" || aiSecond[0].Info != "_SENSOR_reserve" || *aiSecond[0].Params.Initial != skzAD3Initial {
		t.Fatal("main/reserve AI calls lost")
	}
	if ai[7].Graphics.X != "1470" || ai[7].Graphics.Y != "3720" || aiSecond[0].Graphics.Y != "580" {
		t.Fatal("sparse physical positions shifted", ai)
	}
	ai = append(ai, aiSecond...)
	for index := 0; index < len(ai); index += 7 {
		owner := ai[index]
		for offset, member := range []string{".Out", ".Stat"} {
			field := ai[index+offset+1]
			if field.Info != owner.Info+member || field.Params.CardID != owner.Params.CardID || field.Params.Text != member || field.Params.Commented != "true" {
				t.Fatalf("unbound or changed member %s", field.Info)
			}
		}
		if ai[index+3].Info != owner.Info+"_MOS" || ai[index+4].Info != owner.Info+"_SRV" || ai[index+3].Params.CardID == owner.Params.CardID || ai[index+4].Params.CardID == owner.Params.CardID || ai[index+3].Params.CardID == ai[index+4].Params.CardID {
			t.Fatal("unbound or reused auxiliary cards")
		}
	}
	byID := map[string]outputBlock{}
	for _, block := range doc.POUS.Items[2].ISAGraf.Blocks.Items {
		byID[block.T11ID] = block
	}
	want := map[string]bool{"_OUTPUT_DDVH|_PLC_A3_03|i00": true, "_ALARM_DDVH|_PLC_A3_03|i31": true, "_OUTPUT_DDVH|_PLC_A3_04|i05": true}
	var repeatCard string
	for _, link := range doc.POUS.Items[2].ISAGraf.Links.Items {
		from, to := strings.Split(link.FirstPoint.Value, "|"), strings.Split(link.LastPoint.Last, "|")
		input, module := byID[from[0]], byID[to[0]]
		key := input.Info + "|" + module.Info + "|" + to[2]
		if !want[key] || from[2] != "0" || input.Params.ISAObjectID != "-9" || module.Params.ISAObjectID != "1933" {
			t.Fatalf("wrong DO wiring %s", key)
		}
		delete(want, key)
		if input.Info == "_OUTPUT_DDVH" {
			if repeatCard != "" && input.Params.CardID != repeatCard {
				t.Fatal("repeated BOOL must share its card")
			}
			repeatCard = input.Params.CardID
		}
	}
	if len(want) != 0 {
		t.Fatal("missing wiring", want)
	}
	seen := map[string]bool{}
	for _, pou := range doc.POUS.Items {
		for _, block := range pou.ISAGraf.Blocks.Items {
			if seen[block.T11ID] {
				t.Fatal("duplicate T11", block.T11ID)
			}
			seen[block.T11ID] = true
		}
	}
}

func TestSKZRejectsInvalidIDsAndPlans(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SKZRequest)
	}{
		{"missing", func(r *SKZRequest) { r.POUs[0].ModuleIDs = nil }},
		{"null", func(r *SKZRequest) { r.POUs[0].ModuleIDs[0] = nil }},
		{"negative", func(r *SKZRequest) { *r.POUs[0].ModuleIDs[0] = -1 }},
		{"overflow", func(r *SKZRequest) { *r.POUs[0].ModuleIDs[0] = maxTransportID + 1 }},
		{"duplicate across POU", func(r *SKZRequest) { *r.POUs[1].ModuleIDs[0] = 0 }},
		{"duplicate selection", func(r *SKZRequest) { r.POUs = append(r.POUs, r.POUs[0]) }},
		{"unknown selection", func(r *SKZRequest) { r.POUs[0].GroupKey = "UNKNOWN" }},
		{"unknown mode", func(r *SKZRequest) { r.Kind = "FBD" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := skzTestSource()
			request := skzTestRequest(source, "st")
			tc.change(&request)
			if _, err := PrepareSKZPlans(source, request); err == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*skzmap.Plan)
	}{
		{"duplicate channel", func(p *skzmap.Plan) { p.Groups[0].Modules[0].Channels[1].Channel = 0 }},
		{"negative channel", func(p *skzmap.Plan) { p.Groups[0].Modules[0].Channels[0].Channel = -1 }},
		{"overflow channel", func(p *skzmap.Plan) { p.Groups[0].Modules[0].Channels[1].Channel = 16 }},
		{"code injection", func(p *skzmap.Plan) { p.Groups[0].Modules[0].Channels[0].Tag = "_TAG; END_PROGRAM" }},
		{"wrong type", func(p *skzmap.Plan) { p.Groups[0].Modules[0].ObjectType = "AN_v1" }},
		{"wrong prefix", func(p *skzmap.Plan) { p.Groups[0].Modules[0].Name = "A2-00" }},
		{"member FBD source", func(p *skzmap.Plan) { p.Groups[1].Modules[0].Channels[0].Member = "Out" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := skzTestSource()
			tc.change(source)
			if _, err := PrepareSKZPlans(source, skzTestRequest(source, "fbd")); err == nil {
				t.Fatal("accepted invalid source")
			}
		})
	}
	plans := skzTestPlans(t, "st")
	ctx := DefaultSKZContext()
	ctx.ControllerTypeName = "TENIX-CPU715"
	if _, err := (Generator{}).GenerateSKZ(plans[0], ctx, IDRange{POUID: 1}); err == nil {
		t.Fatal("accepted CPU715")
	}
	if _, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: maxTransportID}); err == nil {
		t.Fatal("accepted POU overflow")
	}
	fbd := skzTestPlans(t, "fbd")[0]
	for _, ids := range []IDRange{{T11Start: 0, CardStart: 1, POUID: 1}, {T11Start: maxTransportID, CardStart: 1, POUID: 1}, {T11Start: 1, CardStart: maxTransportID, POUID: 1}} {
		if _, err := (Generator{}).GenerateSKZ(fbd, DefaultSKZContext(), ids); err == nil {
			t.Fatal("accepted invalid FBD reservation", ids)
		}
	}
}

func TestSKZControllerIsolationAndFBDIgnoresSTSettings(t *testing.T) {
	source := skzTestSource()
	source.Groups = source.Groups[:1]
	other := source.Groups[0]
	other.SCS, other.Key = "OTHER", "OTHER:AI:A1"
	source.Groups = append(source.Groups, other)
	request := skzTestRequest(source, "st")
	*request.POUs[1].ModuleIDs[0], *request.POUs[1].ModuleIDs[1] = 0, 1
	plans, err := PrepareSKZPlans(source, request)
	if err != nil || len(plans) != 2 {
		t.Fatalf("%+v: %v", plans, err)
	}
	request.Kind = "fbd"
	request.POUs[0].ModuleIDs = nil
	request.POUs[1].ModuleIDs = []*int64{nil, nil}
	plans, err = PrepareSKZPlans(source, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range plans {
		for _, pou := range plan.POUs {
			for _, module := range pou.Modules {
				if module.ID != nil {
					t.Fatal("FBD carries physical ID")
				}
			}
		}
	}
}

func TestSKZRepeatedAIHasOneCallButKeepsEveryPhysicalAssignment(t *testing.T) {
	source := skzTestSource()
	source.Groups = source.Groups[:1]
	source.Groups[0].Modules[1].Channels[0].Tag = "_SENSOR_main"
	for _, kind := range []string{"st", "fbd"} {
		plans, err := PrepareSKZPlans(source, skzTestRequest(source, kind))
		if err != nil {
			t.Fatal(err)
		}
		req, err := RequirementsForSKZ(plans[0])
		if err != nil {
			t.Fatal(err)
		}
		result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "st" {
			if plans[0].AssignmentCount != 6 || plans[0].RepeatedAssignmentCount != 2 || bytes.Count(result.XML, []byte("_SENSOR_main.Xin :=")) != 2 {
				t.Fatal("repeated AI physical assignments lost")
			}
		} else if req.T11Count != 18 || req.CardCount != 6 || result.Summary.Blocks != 14 || bytes.Count(result.XML, []byte(`GROBJTYPE="37" Info="_SENSOR_main"`)) != 1 {
			t.Fatal("repeated AI object called more than once")
		}
	}
}

func TestSKZActualWorkbookCountsAndDOReferenceAssignments(t *testing.T) {
	for _, tc := range []struct {
		name                                                string
		modules, signals, assignments, blocks, links, cards int
	}{
		{"ai.xlsx", 32, 488, 976, 3416, 0, 1464}, {"do.xlsx", 8, 144, 144, 152, 144, 44},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "skzmap", "testdata", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			source, err := skzmap.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"st", "fbd"} {
				request := skzTestRequest(source, kind)
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
				plans, err := PrepareSKZPlans(source, request)
				if err != nil {
					t.Fatal(err)
				}
				if len(plans) != 1 || plans[0].ModuleCount != tc.modules || plans[0].SignalCount != tc.signals || plans[0].AssignmentCount != tc.assignments {
					t.Fatalf("counts %+v", plans)
				}
				result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: 100000, T11Start: 200000, CardStart: 300000})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "fbd" {
					pouCount := len(source.Groups)
					if tc.name == "ai.xlsx" {
						pouCount = tc.modules
					}
					if result.Summary.POUCount != pouCount || result.Summary.Blocks != tc.blocks || result.Summary.Links != tc.links || result.Summary.Cards != tc.cards {
						t.Fatalf("FBD counts %+v", result.Summary)
					}
					if tc.name == "ai.xlsx" {
						var doc outputDocument
						if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
							t.Fatal(err)
						}
						for index, pou := range plans[0].POUs {
							if len(pou.Modules) != 1 {
								t.Fatal("AI modules combined into one POU")
							}
							module, actual := pou.Modules[0], doc.POUS.Items[index]
							if actual.Name != "AI_"+strings.ReplaceAll(module.Name, "-", "_") || len(actual.ISAGraf.Blocks.Items) != 7*len(module.Channels) || len(actual.Graphics.Items) != 2*len(module.Channels) || len(result.Summary.POUs[index].IOModules) != 1 {
								t.Fatal("incorrect module POU name or composition", actual.Name)
							}
							for channelIndex, channel := range module.Channels {
								owner := actual.ISAGraf.Blocks.Items[7*channelIndex]
								if owner.Info != channel.Tag {
									t.Fatal("Excel signal was moved into another module's POU", channel.Tag, actual.Name)
								}
								for offset := 1; offset <= 2; offset++ {
									if actual.ISAGraf.Blocks.Items[7*channelIndex+offset].Params.CardID != owner.Params.CardID {
										t.Fatal("AD3 field binding changed on split", channel.Tag)
									}
								}
							}
						}
					}
				} else if tc.name == "do.xlsx" {
					var doc outputAOSTDocument
					if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
						t.Fatal(err)
					}
					for _, pou := range doc.POUS.Items {
						if pou.Name != "DO_A3_channels" {
							continue
						}
						count := 0
						for _, line := range strings.Split(pou.Code, "\n") {
							if !strings.Contains(line, ":=") {
								continue
							}
							count++
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
						if count != 72 {
							t.Fatal("DO_A3 assignment count", count)
						}
					}
				}
			}
		})
	}
}

func TestSKZAINativeFragmentAndImportBindingOrder(t *testing.T) {
	nativeData, err := os.ReadFile(filepath.Join("testdata", "SOGO_AI_fbd.xml"))
	if err != nil {
		t.Fatal(err)
	}
	// The untouched supplied fixture contains raw backspace separators in
	// KLPath. They are irrelevant to graph binding; normalize only this read.
	nativeData = bytes.ReplaceAll(nativeData, []byte{8}, []byte{'\\'})
	var native outputDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(nativeData, utf8BOM), &native); err != nil {
		t.Fatal(err)
	}
	workbook, err := os.ReadFile(filepath.Join("..", "skzmap", "testdata", "ai.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := skzmap.Parse(workbook)
	if err != nil {
		t.Fatal(err)
	}
	var group skzmap.Group
	for _, candidate := range source.Groups {
		if candidate.Prefix == "A1" {
			group = candidate
		}
	}
	if len(group.Modules) == 0 || group.Modules[0].Name != "A1-00" {
		t.Fatal("missing native A1-00 module")
	}
	group.Modules = group.Modules[:1]
	source.Groups = []skzmap.Group{group}
	plans, err := PrepareSKZPlans(source, SKZRequest{Kind: "fbd", POUs: []SKZPOURequest{{GroupKey: group.Key}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Generator{}).GenerateSKZ(plans[0], DefaultSKZContext(), IDRange{POUID: 100000, T11Start: 200000, CardStart: 300000})
	if err != nil {
		t.Fatal(err)
	}
	var actual outputDocument
	if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &actual); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Blocks != 112 || result.Summary.Cards != 48 || result.Summary.Graphics != 32 || result.Summary.Links != 0 {
		t.Fatalf("native composition changed %+v", result.Summary)
	}
	if native.POUS.Items[0].ISAGraf.Blocks.Items[0].ObjectType != "31" || actual.POUS.Items[0].ISAGraf.Blocks.Items[0].ObjectType != "37" {
		t.Fatal("fixture no longer captures members-before-owner import defect")
	}
	cardNames := func(doc outputDocument) map[string]string {
		cards := map[string]string{}
		for _, card := range doc.ISACards.Items {
			cards[card.ID] = strings.ToUpper(card.Info)
		}
		return cards
	}
	nativeCards, actualCards := cardNames(native), cardNames(actual)
	canonical := func(block outputBlock, cards map[string]string) outputBlock {
		block.T11ID, block.Info = "", strings.ToUpper(block.Info)
		if block.Params.CardID != "0" {
			block.Params.CardID = cards[block.Params.CardID]
		}
		// The native page has a few manually shifted auxiliary elements.
		// Keep call anchors exact; use the first fragment's relative layout
		// consistently for fields/labels without copying those incidental moves.
		if block.ObjectType != "37" {
			block.Graphics.X, block.Graphics.Y = "", ""
		}
		return block
	}
	blockKey := func(block outputBlock) string { return block.ObjectType + "|" + strings.ToUpper(block.Info) }
	nativeBlocks := map[string][]outputBlock{}
	for _, block := range native.POUS.Items[0].ISAGraf.Blocks.Items {
		key := blockKey(block)
		nativeBlocks[key] = append(nativeBlocks[key], canonical(block, nativeCards))
	}
	for _, block := range actual.POUS.Items[0].ISAGraf.Blocks.Items {
		key := blockKey(block)
		candidates := nativeBlocks[key]
		if len(candidates) == 0 || !reflect.DeepEqual(canonical(block, actualCards), candidates[0]) {
			t.Fatalf("native fragment differs at %s\ngot: %+v\nwant: %+v", key, canonical(block, actualCards), candidates)
		}
		if len(candidates) == 1 {
			delete(nativeBlocks, key)
		} else {
			nativeBlocks[key] = candidates[1:]
		}
	}
	if len(nativeBlocks) != 0 {
		t.Fatal("missing native blocks")
	}
	// All seven blocks retain the first native fragment's exact geometry.
	for _, block := range actual.POUS.Items[0].ISAGraf.Blocks.Items[:7] {
		found := false
		for _, reference := range native.POUS.Items[0].ISAGraf.Blocks.Items[:7] {
			if blockKey(block) == blockKey(reference) && block.Graphics == reference.Graphics {
				found = true
			}
		}
		if !found {
			t.Fatal("first native fragment geometry changed", block.Info)
		}
	}
	nativeGraphics := map[string][]outputPrimitive{}
	for _, primitive := range native.POUS.Items[0].Graphics.Items {
		primitive.SourceT11ID, primitive.X, primitive.Y = "", "", ""
		nativeGraphics[primitive.Params] = append(nativeGraphics[primitive.Params], primitive)
	}
	for index, primitive := range actual.POUS.Items[0].Graphics.Items {
		if index < 2 && (primitive.X != native.POUS.Items[0].Graphics.Items[index].X || primitive.Y != native.POUS.Items[0].Graphics.Items[index].Y) {
			t.Fatal("first native graphic geometry changed")
		}
		primitive.SourceT11ID, primitive.X, primitive.Y = "", "", ""
		key := primitive.Params
		candidates := nativeGraphics[key]
		if len(candidates) == 0 || primitive != candidates[0] {
			t.Fatalf("native graphic differs\ngot: %+v\nwant: %+v", primitive, candidates)
		}
		if len(candidates) == 1 {
			delete(nativeGraphics, key)
		} else {
			nativeGraphics[key] = candidates[1:]
		}
	}
	if len(nativeGraphics) != 0 {
		t.Fatal("missing native graphics")
	}
	for _, channel := range group.Modules[0].Channels {
		for _, suffix := range []string{"", "_MOS", "_SRV"} {
			found := false
			for _, card := range actual.ISACards.Items {
				if card.Info == channel.Tag+suffix {
					found = true
				}
			}
			if !found {
				t.Fatalf("Excel spelling lost: %s%s", channel.Tag, suffix)
			}
		}
	}
}

func TestSKZFBDReferenceValidationRejectsBrokenEdges(t *testing.T) {
	plan := skzTestPlans(t, "fbd")[0]
	result, err := (Generator{}).GenerateSKZ(plan, DefaultSKZContext(), IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*outputDocument){
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[0].Params.CardID = "999" },
		func(d *outputDocument) {
			d.POUS.Items[2].ISAGraf.Links.Items[0].LastPoint.Last = d.POUS.Items[0].ISAGraf.Blocks.Items[0].T11ID + "|True|i00|0,0,100,20"
		},
		func(d *outputDocument) {
			d.POUS.Items[0].ISAGraf.Blocks.Items[0].T11ID = d.POUS.Items[0].ISAGraf.Blocks.Items[1].T11ID
		},
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[0].Params.ISAObjectID = "888" },
		func(d *outputDocument) { d.ISAObjects.Items = nil },
		func(d *outputDocument) { d.ISAObjects.Items[0].Info = "AN_v1" },
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[0].T11ID = "0" },
		func(d *outputDocument) {
			d.POUS.Items[2].ISAGraf.Links.Items[0].LastPoint.Last = strings.Replace(d.POUS.Items[2].ISAGraf.Links.Items[0].LastPoint.Last, "|i00|", "|i32|", 1)
		},
		func(d *outputDocument) {
			blocks := d.POUS.Items[0].ISAGraf.Blocks.Items
			blocks[0], blocks[1] = blocks[1], blocks[0]
		},
		func(d *outputDocument) {
			d.POUS.Items[0].ISAGraf.Blocks.Items[1].Params.CardID = d.POUS.Items[0].ISAGraf.Blocks.Items[7].Params.CardID
		},
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[1].Params.Text = ".OutWrong" },
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[1].Params.Commented = "false" },
		func(d *outputDocument) { d.POUS.Items[0].ISAGraf.Blocks.Items[3].Info = "_FOREIGN_MOS" },
		func(d *outputDocument) {
			d.POUS.Items[0].Graphics.Items[0].SourceT11ID = d.POUS.Items[0].ISAGraf.Blocks.Items[0].T11ID
		},
	} {
		var doc outputDocument
		if err := xml.Unmarshal(bytes.TrimPrefix(result.XML, utf8BOM), &doc); err != nil {
			t.Fatal(err)
		}
		change(&doc)
		data, err := serializeSCADA(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateGeneratedSKZFBD(data, doc, 35); err == nil {
			t.Fatal("accepted invalid FBD")
		}
	}
}
