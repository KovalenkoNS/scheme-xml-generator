// Планировщик модулей проверяет выбор POU, резервные каналы, аппаратные ID и точные требования до генерации.
package planning

import (
	"fmt"
	"scheme-xml-generator/internal/generator/identifiers"
	"scheme-xml-generator/internal/generator/moduleprofile"
	moduleid "scheme-xml-generator/internal/generator/modules"

	"scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/generator/contracts"

	"strings"
)

// PrepareModulePlans Преобразует разобранный IO-лист и выбор пользователя в планы физических ST-назначений.
// Проверяет группы, количество и явные ModuleID до передачи генераторам.
func PrepareModulePlans(source *assignments.Plan, request contracts.ModuleMappingRequest) ([]contracts.ControllerPlan, error) {
	if source == nil || len(source.Groups) == 0 || len(source.Groups) > 128 || len(request.POUs) == 0 || len(request.POUs) > 128 || request.Kind != "st" {
		return nil, fmt.Errorf("Модули: выберите ST и от 1 до 128 POU; FBD требует библиотечный план")
	}
	if request.DOFBDProfile != "" {
		return nil, fmt.Errorf("устаревший профиль DO FBD не используется в ST")
	}
	available := map[string]bool{}
	sourceModules := map[string]bool{}
	sourceTags := map[string]bool{}
	for _, group := range source.Groups {
		if group.Key == "" || available[group.Key] {
			return nil, fmt.Errorf("Модули: пустой или повторный ключ POU")
		}
		available[group.Key] = true
		for _, module := range group.Modules {
			sourceModules[strings.ToUpper(group.ControllerName)+"|"+module.Name] = true
			for _, channel := range module.Channels {
				sourceTags[strings.ToUpper(group.ControllerName+"|"+channel.Tag)] = true
			}
		}
	}
	selected := map[string]contracts.ModuleGroupRequest{}
	for _, choice := range request.POUs {
		if _, exists := selected[choice.GroupKey]; exists || !available[choice.GroupKey] {
			return nil, fmt.Errorf("Модули: неизвестная или повторно выбранная POU %q", choice.GroupKey)
		}
		selected[choice.GroupKey] = choice
	}
	var plans []contracts.ControllerPlan
	controllers := map[string]int{}
	totalModules, totalSignals, totalPOUs, mappedChannels := 0, 0, 0, 0
	for _, group := range source.Groups {
		choice, ok := selected[group.Key]
		if !ok {
			continue
		}
		moduleCount := len(group.Modules)
		if choice.ModuleCount != nil {
			moduleCount = *choice.ModuleCount
		}
		if len(group.Modules) == 0 || moduleCount < 1 || moduleCount < len(group.Modules) || moduleCount > 4096 {
			return nil, fmt.Errorf("Модули %s: число модулей должно быть от %d до 4096 и сохранять все модули Excel", group.Key, max(1, len(group.Modules)))
		}
		profile, supported := moduleprofile.Lookup(group.Kind)
		if !supported {
			return nil, fmt.Errorf("Модули %s: неизвестный тип модулей %q", group.Key, group.Kind)
		}
		capacity := profile.Capacity
		maxSlot := -1
		groupSignals := 0
		for _, module := range group.Modules {
			slot, valid := moduleid.ModuleSlot(module.Name, group.Prefix)
			if !valid {
				return nil, fmt.Errorf("Модули %s: неверное имя модуля %q", group.Key, module.Name)
			}
			maxSlot = max(maxSlot, slot)
			totalSignals += len(module.Channels)
			groupSignals += len(module.Channels)
		}
		added := moduleCount - len(group.Modules)
		if maxSlot+added > 4095 {
			return nil, fmt.Errorf("Модули %s: номер добавленного модуля превышает 4095", group.Key)
		}
		totalModules += moduleCount
		totalSignals += added * capacity
		mappedChannels += groupSignals + added*capacity
		if request.Kind == "st" && profile.AllPhysicalSTChannels {
			mappedChannels += moduleCount*capacity - groupSignals - added*capacity
		}
		if totalModules > 4096 || totalSignals > 4096 || mappedChannels > 4096 {
			return nil, fmt.Errorf("Модули: не более 4096 модулей и 4096 каналов за одну генерацию")
		}
		totalPOUs++
		if totalPOUs > 128 {
			return nil, fmt.Errorf("Модули: не более 128 ST POU за одну генерацию")
		}
		if len(choice.ModuleIDs) != moduleCount {
			return nil, fmt.Errorf("Модули %s: ST требует по одному ID на модуль", group.Key)
		}
		pou := contracts.ModuleGroup{GroupKey: group.Key, Name: group.POUName + "_channels", Kind: group.Kind, Prefix: group.Prefix}
		for _, module := range group.Modules {
			copyModule := contracts.PhysicalModule{Name: module.Name, Type: module.Type, ObjectType: module.ObjectType, Capacity: module.Capacity, Channels: append([]assignments.Channel(nil), module.Channels...)}
			pou.Modules = append(pou.Modules, copyModule)
		}
		for extra := 1; extra <= added; extra++ {
			name := fmt.Sprintf("%s-%02d", group.Prefix, maxSlot+extra)
			if sourceModules[strings.ToUpper(group.ControllerName)+"|"+name] {
				return nil, fmt.Errorf("Модули %s: добавленный модуль %s уже существует в другой POU исходного файла", group.Key, name)
			}
			module := contracts.PhysicalModule{Name: name, Type: profile.HardwareType, ObjectType: profile.ObjectType, Capacity: capacity}
			for channel := 0; channel < capacity; channel++ {
				tag := fmt.Sprintf("%s_%d", moduleid.ModuleInstanceTag(group.ControllerName, name), channel)
				if group.Kind == "AI" && sourceTags[strings.ToUpper(group.ControllerName+"|"+tag)] {
					return nil, fmt.Errorf("Модули %s: имя добавленного резервного AI %s уже используется в исходном файле", group.Key, tag)
				}
				module.Channels = append(module.Channels, assignments.Channel{Channel: channel, Tag: tag, Reserve: true})
			}
			pou.Modules = append(pou.Modules, module)
		}
		for index := range pou.Modules {
			if choice.ModuleIDs[index] == nil {
				return nil, fmt.Errorf("Модули %s/%s: не указан физический ID", group.Key, pou.Modules[index].Name)
			}
			id := *choice.ModuleIDs[index]
			pou.Modules[index].ID = &id
		}
		key := strings.ToUpper(group.ControllerName)
		index, exists := controllers[key]
		if !exists {
			index = len(plans)
			controllers[key] = index
			plans = append(plans, contracts.ControllerPlan{ControllerName: group.ControllerName, Kind: request.Kind, Warnings: append([]string(nil), source.Warnings...)})
		} else if plans[index].ControllerName != group.ControllerName {
			return nil, fmt.Errorf("Модули: неоднозначный регистр имени ПЛК %q", group.ControllerName)
		}
		plans[index].POUs = append(plans[index].POUs, pou)
	}
	for index := range plans {
		if _, err := ValidateControllerPlan(&plans[index]); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

// IsSyntheticReserve Отличает созданный парсером резервный канал от пользовательского сигнала.
// Подготовка плана сохраняет свободные слоты без выдуманных назначений.
func IsSyntheticReserve(channel assignments.Channel) bool {
	return channel.Reserve && channel.SourceRow == 0
}

// ValidateControllerPlan Проверяет подготовленные модули, каналы и привязки перед ST-экспортом.
// Возвращает потребность в ID; неверные планы не должны доходить до allocator.
func ValidateControllerPlan(plan *contracts.ControllerPlan) (contracts.DocumentRequirements, error) {
	var req contracts.DocumentRequirements
	if !identifiers.ControllerNamePattern.MatchString(plan.ControllerName) || plan.Kind != "st" || len(plan.POUs) == 0 || len(plan.POUs) > 128 {
		return req, fmt.Errorf("Модули: неверный ПЛК, режим или число POU")
	}
	plan.ModuleCount, plan.SignalCount, plan.AssignmentCount, plan.RepeatedAssignmentCount = 0, 0, 0, 0
	pouNames, modules, hardware := map[string]bool{}, map[string]bool{}, map[int64]bool{}
	assigned := map[string]bool{}
	reservedAI, diTargets := map[string]bool{}, map[string]bool{}
	mappedChannels := 0

	for _, pou := range plan.POUs {
		wantName := pou.Kind + "_" + pou.Prefix + "_channels"
		profile, supported := moduleprofile.Lookup(pou.Kind)
		if !supported || !moduleid.RackPrefixPattern.MatchString(pou.Prefix) || pou.Name != wantName || pouNames[strings.ToUpper(pou.Name)] || len(pou.Modules) == 0 {
			return req, fmt.Errorf("Модули: неверная или повторная POU %q", pou.Name)
		}
		pouNames[strings.ToUpper(pou.Name)] = true
		for _, module := range pou.Modules {
			_, validSlot := moduleid.ModuleSlot(module.Name, pou.Prefix)
			capacity := profile.Capacity
			if !validSlot || module.Type != profile.HardwareType || module.ObjectType != profile.ObjectType || module.Capacity != capacity || modules[module.Name] || len(module.Channels) == 0 && !profile.AllowEmpty || len(module.Channels) > capacity {
				return req, fmt.Errorf("Модули %s: неверный или повторный модуль %q", pou.Name, module.Name)
			}
			modules[module.Name] = true
			mappedChannels += len(module.Channels)
			moduleAssignments, signalAssignments := profile.AssignmentCounts(plan.Kind)
			plan.AssignmentCount += moduleAssignments
			if plan.Kind == "st" && profile.AllPhysicalSTChannels {
				mappedChannels += capacity - len(module.Channels)
			}
			if mappedChannels > 4096 {
				return req, fmt.Errorf("Модули: не более 4096 физических каналов за одну генерацию")
			}
			plan.ModuleCount++
			if plan.ModuleCount > 4096 {
				return req, fmt.Errorf("Модули: не более 4096 модулей")
			}
			if module.ID == nil || *module.ID < 0 || *module.ID > contracts.MaxTransportID || hardware[*module.ID] {
				return req, fmt.Errorf("Модули %s/%s: ID должен быть указан, уникален в ПЛК и лежать в диапазоне 0..%d", pou.Name, module.Name, contracts.MaxTransportID)
			}
			hardware[*module.ID] = true
			lastChannel := -1
			for _, channel := range module.Channels {
				if channel.Channel <= lastChannel || channel.Channel >= capacity || !identifiers.ObjectNamePattern.MatchString(channel.Tag) || channel.Member != "" && !identifiers.ObjectNamePattern.MatchString(channel.Member) {
					return req, fmt.Errorf("Модули %s/%s: неверный тег или номер канала %d", pou.Name, module.Name, channel.Channel)
				}
				if pou.Kind == "AI" && channel.Member != "" {
					return req, fmt.Errorf("Модули: ожидается имя экземпляра AI или логической переменной DO без поля: %s.%s", channel.Tag, channel.Member)
				}
				if !IsSyntheticReserve(channel) && !profile.AcceptsReceiver(channel.Member) {
					return req, fmt.Errorf("DI: ожидается поле C1 или C2 объекта DDR_v1: %s.%s", channel.Tag, channel.Member)
				}
				lastChannel = channel.Channel
				plan.SignalCount++
				if plan.SignalCount > 4096 {
					return req, fmt.Errorf("Модули: не более 4096 каналов")
				}
				plan.AssignmentCount += signalAssignments
				key := strings.ToUpper(channel.Tag + "." + channel.Member)
				if pou.Kind == "DI" && !IsSyntheticReserve(channel) {
					if diTargets[key] {
						return req, fmt.Errorf("DI: получатель %s.%s подключён к нескольким физическим каналам", channel.Tag, channel.Member)
					}
					diTargets[key] = true
				}
				if pou.Kind != "DI" && assigned[key] {
					if pou.Kind == "AI" && (IsSyntheticReserve(channel) || reservedAI[key]) {
						return req, fmt.Errorf("Модули: имя резервного AI %s совпадает с другим назначением", channel.Tag)
					}
					plan.RepeatedAssignmentCount += profile.AssignmentsPerSignal
				}
				if pou.Kind != "DI" {
					assigned[key] = true
				}
				if pou.Kind == "AI" && IsSyntheticReserve(channel) {
					reservedAI[key] = true
				}
			}
		}
	}
	req.POUCount, req.SignalCount = len(plan.POUs), plan.SignalCount
	return req, nil
}

// RequirementsForController Считает ресурсы проверенного плана модулей для резервирования XML ID.
// Не изменяет исходный план или постоянное состояние генератора.
func RequirementsForController(plan contracts.ControllerPlan) (contracts.DocumentRequirements, error) {
	return ValidateControllerPlan(&plan)
}
