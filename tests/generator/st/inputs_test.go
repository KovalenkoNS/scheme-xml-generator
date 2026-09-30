// Проверки ST используют единые тестовые источники ПЛК и назначений.
package generator_test

import (
	"fmt"
	"scheme-xml-generator/internal/generator/planning"
	stassignment "scheme-xml-generator/internal/generator/st/assignment"
	"scheme-xml-generator/internal/inputs/assignments"
	"testing"
)

// assignmentTestSource строит разреженные AI/DO-назначения двух POU для проверки планирования и снимков данных.
func assignmentTestSource() *assignments.Plan {
	return &assignments.Plan{Groups: []assignments.Group{
		{Key: "PLC:AI:A1", ControllerName: "PLC", Kind: "AI", Prefix: "A1", POUName: "AI_A1", Modules: []assignments.Module{
			{Name: "A1-00", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []assignments.Channel{{Channel: 0, Tag: "_SENSOR_main", SourceRow: 2}, {Channel: 15, Tag: "_SPARE", Reserve: true, SourceRow: 4}}},
			{Name: "A1-02", Type: "AI16H", ObjectType: "AD3_v2", Capacity: 16, Channels: []assignments.Channel{{Channel: 2, Tag: "_SENSOR_reserve", SourceRow: 6}}},
		}},
		{Key: "PLC:DO:A3", ControllerName: "PLC", Kind: "DO", Prefix: "A3", POUName: "DO_A3", Modules: []assignments.Module{
			{Name: "A3-03", Type: "DO32P", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 0, Tag: "_OUTPUT_DDVH", SourceRow: 2}, {Channel: 31, Tag: "_ALARM_DDVH", SourceRow: 3}}},
			{Name: "A3-04", Type: "DO32P", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 5, Tag: "_OUTPUT_DDVH", SourceRow: 4}}},
		}},
	}}
}

// assignmentTestRequest задаёт явные уникальные ModuleID для всех выбранных групп тестового ST-запроса.
func assignmentTestRequest(source *assignments.Plan, kind string) stassignment.ModuleMappingRequest {
	request := stassignment.ModuleMappingRequest{Kind: kind}
	nextID := int64(0)
	for _, group := range source.Groups {
		choice := stassignment.ModuleGroupRequest{GroupKey: group.Key}
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

// assignmentTestPlans вызывает реальный планировщик для тестового источника и прекращает сценарий при ошибке подготовки.
func assignmentTestPlans(t *testing.T, kind string) []stassignment.ControllerPlan {
	t.Helper()
	source := assignmentTestSource()
	plans, err := planning.PrepareModulePlans(source, assignmentTestRequest(source, kind))
	if err != nil {
		t.Fatal(err)
	}
	return plans
}

// assignmentTestModuleCount передаёт явно заданное число модулей в запрос расширения тестового ST-плана.
func assignmentTestModuleCount(count int) *int { return &count }

// assignmentTestGroupModules находит модули исходной группы в подготовленном плане для проверки расширения и коллизий.
func assignmentTestGroupModules(plan stassignment.ControllerPlan, key string) []stassignment.PhysicalModule {
	var modules []stassignment.PhysicalModule
	for _, pou := range plan.POUs {
		if pou.GroupKey == key {
			modules = append(modules, pou.Modules...)
		}
	}
	return modules
}

// assignmentSparseDigitalGroup строит разреженные DI/DO-модули для проверки общего предела физических каналов.
func assignmentSparseDigitalGroup(scs, kind, prefix string, count int) assignments.Group {
	group := assignments.Group{Key: scs + ":" + kind + ":" + prefix, ControllerName: scs, Kind: kind, Prefix: prefix, POUName: kind + "_" + prefix}
	moduleType, member := "DO32P", ""
	if kind == "DI" {
		moduleType, member = "DI32", "C1"
	}
	for i := 0; i < count; i++ {
		group.Modules = append(group.Modules, assignments.Module{Name: fmt.Sprintf("%s-%02d", prefix, i), Type: moduleType, ObjectType: "D32V", Capacity: 32,
			Channels: []assignments.Channel{{Channel: 0, Tag: fmt.Sprintf("_%s_%s_%d", scs, kind, i), Member: member, SourceRow: i + 2}}})
	}
	return group
}

// assignmentDITestSource задаёт независимые DI-получатели C1/C2 и свободные каналы для проверки физической перекладки.
func assignmentDITestSource() *assignments.Plan {
	return &assignments.Plan{Groups: []assignments.Group{{Key: "PLC:DI:A11", ControllerName: "PLC", Kind: "DI", Prefix: "A11", POUName: "DI_A11", Modules: []assignments.Module{
		{Name: "A11-05", Type: "DI32", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 0, Tag: "_INPUT_A", Member: "C1", SourceRow: 2}, {Channel: 31, Tag: "_RESERVE_A", Member: "C1", SourceRow: 3, Reserve: true}}},
		{Name: "A11-07", Type: "DI32", ObjectType: "D32V", Capacity: 32, Channels: []assignments.Channel{{Channel: 17, Tag: "_INPUT_B", Member: "C1", SourceRow: 4}}},
	}}}}
}
