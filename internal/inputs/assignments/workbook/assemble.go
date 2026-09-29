// Объединение разобранных назначений в устойчивый порядок ПЛК, групп, модулей и каналов.
package workbook

import (
	model "scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	"sort"
	"strconv"
	"strings"
)

// finish согласует имена ПЛК и собирает отсортированный план из результатов адаптеров.
// Содержит только обработку общих результатов адаптеров, без повторного чтения ячеек.
func finish(modules map[string]*assembly.Module, controllerNames map[string]string, plan *model.Plan) (*model.Plan, error) {
	groupIndices := map[string]int{}
	for _, parsed := range modules {
		parsed.Group.ControllerName = controllerNames[strings.ToUpper(parsed.Group.ControllerName)]
		parsed.Group.Key = parsed.Group.ControllerName + ":" + parsed.Group.Kind + ":" + parsed.Group.Prefix
		module := parsed.Module
		module.Channels = []model.Channel{}
		for _, channel := range parsed.Channels {
			module.Channels = append(module.Channels, channel.Channel)
		}
		sort.Slice(module.Channels, func(i, j int) bool { return module.Channels[i].Channel < module.Channels[j].Channel })
		gi, exists := groupIndices[parsed.Group.Key]
		if !exists {
			gi = len(plan.Groups)
			groupIndices[parsed.Group.Key] = gi
			plan.Groups = append(plan.Groups, parsed.Group)
		}
		plan.Groups[gi].Modules = append(plan.Groups[gi].Modules, module)
	}
	sort.Slice(plan.Groups, func(i, j int) bool {
		a, b := plan.Groups[i], plan.Groups[j]
		if a.ControllerName != b.ControllerName {
			return a.ControllerName < b.ControllerName
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		x, _ := strconv.Atoi(strings.TrimPrefix(a.Prefix, "A"))
		y, _ := strconv.Atoi(strings.TrimPrefix(b.Prefix, "A"))
		return x < y
	})
	for i := range plan.Groups {
		sort.Slice(plan.Groups[i].Modules, func(a, b int) bool {
			_, _, slotA, _ := fields.NormalizeModule(plan.Groups[i].Modules[a].Name)
			_, _, slotB, _ := fields.NormalizeModule(plan.Groups[i].Modules[b].Name)
			return slotA < slotB
		})
	}
	return plan, nil
}
