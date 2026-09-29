// Проверка физических модулей и построение объектов отдельного ПЛК.
package techobjects

import (
	"fmt"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/iomap"

	"sort"
	"strconv"
	"strings"
)

// prepareController строит набор технологических объектов одного ПЛК до сериализации XLS.
// Проверяет и сортирует модули, добавляет диагностику/аналоговые резервы и возвращает план со счётчиками.
func prepareController(controller iomap.Controller, name string, resource int) (Plan, error) {
	if !plcName.MatchString(name) || len(controller.Modules) == 0 || len(controller.Modules) > 4096 {
		return Plan{}, fmt.Errorf("неверное имя ПЛК или список модулей %q", name)
	}
	modules := append([]iomap.Module(nil), controller.Modules...)
	seen := make(map[string]bool)
	for _, module := range modules {
		capacity := hardware.ChannelCount(module.Type)
		parts := moduleName.FindStringSubmatch(module.Name)
		if capacity == 0 || module.Capacity != capacity || len(module.Channels) != capacity || parts == nil || module.Slot < 0 || module.Slot > 15 || module.Name != fmt.Sprintf("%s_%02d", module.Rack, module.Slot) || seen[module.Name] {
			return Plan{}, fmt.Errorf("%s: неверный или повторный модуль %s", name, module.Name)
		}
		seen[module.Name] = true
		for i, channel := range module.Channels {
			if channel.Channel != i {
				return Plan{}, fmt.Errorf("%s/%s: неверная последовательность каналов", name, module.Name)
			}
		}
	}
	sort.Slice(modules, func(i, j int) bool {
		a, b := modules[i], modules[j]
		if a.Rack != b.Rack {
			left, _ := strconv.Atoi(strings.TrimPrefix(a.Rack, "A"))
			right, _ := strconv.Atoi(strings.TrimPrefix(b.Rack, "A"))
			if left != right {
				return left < right
			}
			return a.Rack < b.Rack
		}
		return a.Slot < b.Slot
	})
	plan := Plan{ControllerName: name, Resource: resource, Summary: Summary{ModuleCount: len(modules)}}
	for _, module := range modules {
		plan.Objects = append(plan.Objects, diagnosticObject(name, module))
	}
	for _, module := range modules {
		objectType := map[string]string{"AI16H": "AD3_v2", "AOC4H": "AN_v1"}[module.Type]
		if objectType == "" {
			continue // DI/DO spare channels belong to the module's D32V object.
		}
		template := objectType
		if objectType == "AN_v1" {
			template = "AN"
		}
		for _, channel := range module.Channels {
			if channel.Reserve {
				tag := fmt.Sprintf("_%s_%s_%d", name, module.Name, channel.Channel)
				plan.Objects = append(plan.Objects, Object{Tag: tag, Type: objectType, Name: tag, Template: template})
				plan.Summary.ReserveCount++
			}
		}
	}
	plan.Summary.ObjectCount = len(plan.Objects)
	if plan.Summary.ObjectCount > 65532 { // Four native header rows precede objects.
		return Plan{}, fmt.Errorf("ПЛК %s: объекты не помещаются на один лист XLS (максимум 65532)", name)
	}
	return plan, nil
}
