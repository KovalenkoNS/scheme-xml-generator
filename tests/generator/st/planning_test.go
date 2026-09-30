// Проверки ST сверяют ограничения планов, профилей и физического оборудования.
package generator_test

import (
	cpuprofile "scheme-xml-generator/internal/domain/controller"
	"scheme-xml-generator/internal/generator/addressing"
	xmlidentity "scheme-xml-generator/internal/generator/identity"
	"scheme-xml-generator/internal/generator/planning"
	stgen "scheme-xml-generator/internal/generator/st"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"strings"
	"testing"
)

// TestModuleCountAndNewIDsGuards проверяет отказ планировщика при потере исходных модулей, лишних, отсутствующих и повторных ModuleID.
func TestModuleCountAndNewIDsGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*stassignment.ModuleMappingRequest)
	}{
		{"zero", func(r *stassignment.ModuleMappingRequest) { r.POUs[0].ModuleCount = assignmentTestModuleCount(0) }},
		{"negative", func(r *stassignment.ModuleMappingRequest) { r.POUs[0].ModuleCount = assignmentTestModuleCount(-1) }},
		{"below source", func(r *stassignment.ModuleMappingRequest) { r.POUs[0].ModuleCount = assignmentTestModuleCount(1) }},
		{"above cap", func(r *stassignment.ModuleMappingRequest) { r.POUs[0].ModuleCount = assignmentTestModuleCount(4097) }},
		{"new ID absent", func(r *stassignment.ModuleMappingRequest) { r.POUs[0].ModuleCount = assignmentTestModuleCount(3) }},
		{"new ID null", func(r *stassignment.ModuleMappingRequest) {
			r.POUs[0].ModuleCount = assignmentTestModuleCount(3)
			r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, nil)
		}},
		{"new ID duplicate", func(r *stassignment.ModuleMappingRequest) {
			r.POUs[0].ModuleCount = assignmentTestModuleCount(3)
			id := int64(2)
			r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, &id)
		}},
		{"extra ID", func(r *stassignment.ModuleMappingRequest) {
			id := int64(100)
			r.POUs[0].ModuleIDs = append(r.POUs[0].ModuleIDs, &id)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := assignmentTestSource()
			request := assignmentTestRequest(source, "st")
			tc.change(&request)
			if _, err := planning.PrepareModulePlans(source, request); err == nil {
				t.Fatal("accepted invalid module count or IDs")
			}
		})
	}
}

// TestModuleRejectsUnknownCPUAndPhysicalProfile проверяет отказ ST-экспорта при неизвестном CPU, драйвере и неподтверждённом сочетании CPU715/Measurement.
func TestModuleRejectsUnknownCPUAndPhysicalProfile(t *testing.T) {
	for _, mode := range []string{"st"} {
		plan := assignmentTestPlans(t, mode)[0]
		for _, tc := range []struct{ cpu, profile string }{
			{"TENIX-CPU999", ""}, {"", ""}, {cpuprofile.ControllerCPU715, "unknown"}, {cpuprofile.ControllerCPU850, "unknown"},
		} {
			t.Run(mode+"/"+tc.cpu+"/"+tc.profile, func(t *testing.T) {
				ctx := addressing.DefaultModuleContext()
				ctx.ControllerTypeName, ctx.PhysicalProfile = tc.cpu, tc.profile
				if _, err := (stgen.Generator{}).GenerateModuleMapping(plan, ctx, xmlidentity.IDRange{POUID: 1000, T11Start: 2000, CardStart: 3000}); err == nil {
					t.Fatal("accepted unsupported CPU/profile")
				}
			})
		}
	}
	ctx := addressing.DefaultModuleContext()
	ctx.ControllerTypeName, ctx.PhysicalProfile = cpuprofile.ControllerCPU715, addressing.PhysicalProfileMeasurement
	if _, err := (stgen.Generator{}).GenerateModuleMapping(assignmentTestPlans(t, "st")[0], ctx, xmlidentity.IDRange{POUID: 1000}); err == nil {
		t.Fatal("accepted unverified CPU715 Measurement/Quality ST")
	}
}

// TestModuleDOFullPhysicalChannelLimitForPreparedAndDirectPlans проверяет предел 4096 выходов и в подготовленном, и в напрямую переданном ST-плане.
func TestModuleDOFullPhysicalChannelLimitForPreparedAndDirectPlans(t *testing.T) {
	source := &assignments.Plan{Groups: []assignments.Group{assignmentSparseDigitalGroup("PLC", "DO", "A33", 128)}}
	plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, "st"))
	if err != nil || plans[0].AssignmentCount != 4096 || plans[0].SignalCount != 128 {
		t.Fatalf("4096 physical channel boundary rejected: %+v, %v", plans, err)
	}
	if _, err := (stgen.Generator{}).GenerateModuleMapping(plans[0], addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 100}); err != nil {
		t.Fatal("boundary generation", err)
	}
	source.Groups[0] = assignmentSparseDigitalGroup("PLC", "DO", "A33", 129)
	if _, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, "st")); err == nil {
		t.Fatal("sparse DO bypassed full physical ST channel limit")
	}
	direct := plans[0]
	direct.POUs[0].Modules = append(direct.POUs[0].Modules, direct.POUs[0].Modules[0])
	direct.POUs[0].Modules[128].Name = "A33-128"
	for i := range direct.POUs[0].Modules {
		id := int64(i)
		direct.POUs[0].Modules[i].ID = &id
	}
	if _, err := planning.RequirementsForController(direct); err == nil {
		t.Fatal("direct requirements bypassed physical channel limit")
	}
	if _, err := (stgen.Generator{}).GenerateModuleMapping(direct, addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 100}); err == nil {
		t.Fatal("direct generation bypassed physical channel limit")
	}
}

// TestModuleDOAndDIFullPhysicalLimitAcrossPLCs проверяет общий предел физических каналов нескольких ПЛК; DI-статус не считается отдельным каналом.
func TestModuleDOAndDIFullPhysicalLimitAcrossPLCs(t *testing.T) {
	source := &assignments.Plan{Groups: []assignments.Group{
		assignmentSparseDigitalGroup("PLC", "DO", "A33", 64),
		assignmentSparseDigitalGroup("OTHER", "DI", "A11", 64),
	}}
	plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, "st"))
	if err != nil || len(plans) != 2 {
		t.Fatalf("global 4096 channel boundary rejected: %+v, %v", plans, err)
	}
	if plans[0].AssignmentCount+plans[1].AssignmentCount != 4160 || plans[0].SignalCount+plans[1].SignalCount != 128 {
		t.Fatal("DI status assignments are not physical channels", plans)
	}
	source.Groups[1] = assignmentSparseDigitalGroup("OTHER", "DI", "A11", 65)
	if _, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, "st")); err == nil {
		t.Fatal("separate PLCs bypassed shared DO+DI physical channel limit")
	}
}

// TestModuleDIInvalidMembersDuplicatesAndModuleIDs проверяет единственного физического владельца DI-получателя C1/C2 и обязательные уникальные ModuleID.
func TestModuleDIInvalidMembersDuplicatesAndModuleIDs(t *testing.T) {
	for _, kind := range []string{"st"} {
		for _, member := range []string{"", "C3", "c1", "c2", ".C1", ".C2"} {
			source := assignmentDITestSource()
			source.Groups[0].Modules[0].Channels[0].Member = member
			if _, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind)); err == nil {
				t.Fatalf("accepted unsupported DI member %q in %s", member, kind)
			}
		}
		source := assignmentDITestSource()
		source.Groups[0].Modules[1].Channels[0].Tag = "_input_a"
		if _, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind)); err == nil || !strings.Contains(err.Error(), "нескольким физическим") {
			t.Fatalf("accepted two drivers of one DDR target in %s: %v", kind, err)
		}
	}
	for _, id := range []*int64{nil, func() *int64 { v := int64(0); return &v }(), func() *int64 { v := int64(-1); return &v }()} {
		source := assignmentDITestSource()
		request := assignmentTestRequest(source, "st")
		request.POUs[0].ModuleIDs[1] = id
		if _, err := planning.PrepareModulePlans(source, request); err == nil {
			t.Fatal("accepted missing, repeated or negative DI physical ID")
		}
	}
}

// TestModuleSyntheticAIReserveCannotReuseSourceTag Проверяет, что расширение AI не захватывает реальные теги того же ПЛК, включая невыбранные группы и прямой план.
func TestModuleSyntheticAIReserveCannotReuseSourceTag(t *testing.T) {
	for _, kind := range []string{"st"} {
		for _, unselected := range []bool{false, true} {
			source := assignmentTestSource()
			source.Groups = source.Groups[:1]
			source.Groups[0].Modules = source.Groups[0].Modules[:1]
			collision := "_PLC_A1_01_0"
			if unselected {
				other := assignments.Group{Key: "PLC:AI:A9", ControllerName: "PLC", Kind: "AI", Prefix: "A9", POUName: "AI_A9", Modules: []assignments.Module{
					{Name: "A9-00", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []assignments.Channel{{Channel: 0, Tag: strings.ToLower(collision), SourceRow: 9}}},
				}}
				source.Groups = append(source.Groups, other)
			} else {
				source.Groups[0].Modules[0].Channels[0].Tag = collision
			}
			request := assignmentTestRequest(source, kind)
			request.POUs = request.POUs[:1]
			request.POUs[0].ModuleCount = assignmentTestModuleCount(2)
			if kind == "st" {
				id := int64(100)
				request.POUs[0].ModuleIDs = append(request.POUs[0].ModuleIDs, &id)
			}
			if _, err := planning.PrepareModulePlans(source, request); err == nil {
				t.Fatalf("%s, unselected=%v: synthetic AI reused a source object", kind, unselected)
			}
			if unselected {
				source.Groups[1].ControllerName = "OTHER"
				if _, err := planning.PrepareModulePlans(source, request); err != nil {
					t.Fatal("unselected controller must not block another controller's names", err)
				}
			}
		}
		for _, reverse := range []bool{false, true} {
			source := assignmentTestSource()
			request := assignmentTestRequest(source, kind)
			request.POUs[0].ModuleCount = assignmentTestModuleCount(3)
			if kind == "st" {
				id := int64(100)
				request.POUs[0].ModuleIDs = append(request.POUs[0].ModuleIDs, &id)
			}
			plans, err := planning.PrepareModulePlans(source, request)
			if err != nil {
				t.Fatal(err)
			}
			modules := assignmentTestGroupModules(plans[0], request.POUs[0].GroupKey)
			modules[0].Channels[0].Tag = modules[2].Channels[0].Tag
			if reverse {

				plans[0].POUs[0].Modules[0], plans[0].POUs[0].Modules[2] = plans[0].POUs[0].Modules[2], plans[0].POUs[0].Modules[0]
			}
			if _, err := planning.RequirementsForController(plans[0]); err == nil {
				t.Fatalf("%s, reverse=%v: direct plan reused a synthetic AI object", kind, reverse)
			}
		}
	}
}

// TestModuleDIMixedPlanAndSeparatePLCNamespaces Проверяет смешанные направления и независимые пространства имён разных ПЛК в ST-плане.
func TestModuleDIMixedPlanAndSeparatePLCNamespaces(t *testing.T) {
	for _, kind := range []string{"st"} {
		source := assignmentTestSource()
		source.Groups = append(source.Groups, assignmentDITestSource().Groups...)
		other := assignmentDITestSource().Groups[0]
		other.ControllerName, other.Key = "PLC_OTHER", "PLC_OTHER:DI:A11"
		source.Groups = append(source.Groups, other)
		plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind))
		if err != nil || len(plans) != 2 {
			t.Fatalf("mixed plans: %+v, %v", plans, err)
		}
		wantAssignments := 75
		if kind == "st" {
			wantAssignments = 136
		}
		if plans[0].AssignmentCount != wantAssignments || plans[0].RepeatedAssignmentCount != 1 || plans[1].AssignmentCount != 66 || plans[1].RepeatedAssignmentCount != 0 {
			t.Fatalf("mixed assignment counts: %+v", plans)
		}
		for _, plan := range plans {
			if _, err := (stgen.Generator{}).GenerateModuleMapping(plan, addressing.DefaultModuleContext(), xmlidentity.IDRange{POUID: 100, T11Start: 200, CardStart: 300}); err != nil {
				t.Fatal(err)
			}
		}
	}
}
