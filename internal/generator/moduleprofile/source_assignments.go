// Подтверждённый профиль превращает нейтральные IO-записи в SCADA-назначения; Excel-столбцы и библиотечные FBD-шаблоны здесь не разбираются.
package moduleprofile

import (
	"fmt"
	"scheme-xml-generator/internal/domain/assignments"
	iorecords "scheme-xml-generator/internal/domain/io"
	"scheme-xml-generator/internal/generator/identifiers"
	"sort"
	"strings"
)

// ApplySourceAssignments дополняет прочитанную книгу назначениями AI/DI/DO по подтверждённым образцам и решениям владельца.
// AI использует Loop + main/reserve, DI — Tag No и C1/C2, DO — Tag No + DDVH во всех явных размещениях.
func ApplySourceAssignments(plan *assignments.Plan) error {
	if plan.Source == nil {
		return nil
	}
	groups := map[string]*assignments.Group{}
	modules := map[string]*assignments.Module{}
	positions := map[string]assignments.Channel{}
	receivers := map[string]string{}
	preparedModules := map[string]bool{}
	controllerNames := map[string]string{}
	for gi := range plan.Groups {
		group := &plan.Groups[gi]
		controllerNames[strings.ToUpper(group.ControllerName)] = group.ControllerName
		groups[strings.ToUpper(group.Key)] = group
		for mi := range group.Modules {
			key := strings.ToUpper(group.ControllerName + ":" + group.Modules[mi].Name)
			modules[key] = &group.Modules[mi]
			preparedModules[key] = true
		}
	}
	created := map[string]*assignments.Group{}
	for _, record := range plan.Source.Records {
		controllerKey := strings.ToUpper(record.ControllerName)
		if controllerNames[controllerKey] == "" {
			controllerNames[controllerKey] = record.ControllerName
		}
		record.ControllerName = controllerNames[controllerKey]
		profile, ok := Lookup(record.Kind)
		if !ok {
			return fmt.Errorf("%s, строка%d: профиль физических назначений %s отсутствует", record.Sheet, record.Row, record.Kind)
		}
		for _, placement := range record.Placements {
			if placement.Channel < 0 || placement.Channel >= profile.Capacity {
				return fmt.Errorf("%s, строка%d: %s/%s, канал%d вне диапазона0…%d профиля%s", record.Sheet, record.Row, record.ControllerName, placement.Module, placement.Channel, profile.Capacity-1, profile.HardwareType)
			}
			key := strings.ToUpper(record.ControllerName + ":" + placement.Module)
			if preparedModules[key] {
				return fmt.Errorf("%s, строка %d: модуль %s уже задан подготовленной картой; исходные назначения не объединяются с ней молча", record.Sheet, record.Row, key)
			}
			module := modules[key]
			groupKey := record.ControllerName + ":" + record.Kind + ":" + placement.Rack
			if module == nil {
				group := created[groupKey]
				if group == nil {
					if groups[strings.ToUpper(groupKey)] != nil {
						return fmt.Errorf("%s, строка%d: исходная IO-карта повторяет подготовленную группу%s", record.Sheet, record.Row, groupKey)
					}
					group = &assignments.Group{Key: groupKey, ControllerName: record.ControllerName, Kind: record.Kind, Prefix: placement.Rack, POUName: record.Kind + "_" + placement.Rack, Modules: []assignments.Module{}}
					created[groupKey] = group
				}
				module = &assignments.Module{Name: placement.Module, Type: profile.HardwareType, ObjectType: profile.ObjectType, Template: profile.ObjectType, Capacity: profile.Capacity,
					MainModule: placementByRole(record, "main"), RedundantModule: placementByRole(record, "redundant"), IOType: record.IOType, MarshallingCabinet: record.MarshallingCabinet, ControllerID: record.ControllerID, SourceRow: record.Row, Channels: []assignments.Channel{}}
				modules[key] = module
			} else {
				if module.Type != profile.HardwareType {
					return fmt.Errorf("%s, строка%d: противоречивые типы модуля%s/%s", record.Sheet, record.Row, record.ControllerName, placement.Module)
				}
				if module.ControllerID != record.ControllerID {
					return fmt.Errorf("%s, строка%d: противоречивый ControllerID модуля%s/%s", record.Sheet, record.Row, record.ControllerName, placement.Module)
				}
				for _, field := range []struct {
					target *string
					value  string
				}{{&module.MainModule, placementByRole(record, "main")}, {&module.RedundantModule, placementByRole(record, "redundant")}, {&module.IOType, record.IOType}, {&module.MarshallingCabinet, record.MarshallingCabinet}} {
					if *field.target != field.value {
						*field.target = ""
					}
				}
			}
			tag, member, err := sourceReceiver(record, placement)
			if err != nil {
				return fmt.Errorf("%s, строка%d: %w", record.Sheet, record.Row, err)
			}
			position := fmt.Sprintf("%s/%d", key, placement.Channel)
			channel := assignments.Channel{Channel: placement.Channel, Tag: tag, Member: member, SourceRow: record.Row, Reserve: record.Reserve, Description: record.Description}
			switch strings.ToLower(strings.TrimSpace(record.Loop)) {
			case "spare", "reserve", "резерв":
				channel.Reserve = true
			}
			if previous, exists := positions[position]; exists {
				if record.Kind != "DO" || !strings.EqualFold(previous.Tag, tag) || previous.Member != member {
					return fmt.Errorf("%s, строка%d: повторное/конфликтующее назначение%s, первая строка%d", record.Sheet, record.Row, position, previous.SourceRow)
				}
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s, строка%d: совпадающее назначение%s объединено со строкой%d", record.Sheet, record.Row, position, previous.SourceRow))
				continue
			}
			positions[position] = channel
			if tag == "" {
				continue
			}
			if record.Kind != "DO" {
				owner := strings.ToUpper(record.ControllerName + ":" + tag + "." + member)
				if previous := receivers[owner]; previous != "" {
					return fmt.Errorf("%s, строка%d: получатель%s.%s назначен нескольким позициям%s и%s", record.Sheet, record.Row, tag, member, previous, position)
				}
				receivers[owner] = position
			}
			module.Channels = append(module.Channels, channel)
		}
		if record.Kind == "DI" && record.Reserve {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s, строка%d: SPARE сохраняет физические размещения без технологического получателя", record.Sheet, record.Row))
		}
	}
	for _, group := range created {
		for key, module := range modules {
			if strings.HasPrefix(key, strings.ToUpper(group.ControllerName+":"+group.Prefix+"-")) && module.Type == mustHardwareType(group.Kind) {
				sort.Slice(module.Channels, func(i, j int) bool { return module.Channels[i].Channel < module.Channels[j].Channel })
				group.Modules = append(group.Modules, *module)
			}
		}
		sort.Slice(group.Modules, func(i, j int) bool { return group.Modules[i].Name < group.Modules[j].Name })
		plan.Groups = append(plan.Groups, *group)
	}
	sort.Slice(plan.Groups, func(i, j int) bool { return plan.Groups[i].Key < plan.Groups[j].Key })
	return nil
}

// placementByRole возвращает явно прочитанное имя основного/резервного модуля для метаданных назначения.
func placementByRole(record iorecords.Record, role string) string {
	for _, placement := range record.Placements {
		if placement.Role == role {
			return placement.Module
		}
	}
	return ""
}

// sourceReceiver применяет подтверждённые имена к одному физическому размещению; без правил не создаёт вымышленных тегов.
func sourceReceiver(record iorecords.Record, placement iorecords.Placement) (string, string, error) {
	name, member := record.SignalName, ""
	switch record.Kind {
	case "DI":
		if placement.Role != "main" && placement.Role != "redundant" {
			return "", "", fmt.Errorf("DI: размещение%s не имеет подтверждённого входа DDR", placement.Role)
		}
		if record.Reserve {
			return "", "", nil
		}
		member = "C1"
		if placement.Role == "redundant" {
			member = "C2"
		}
	case "DO":
		if record.Reserve {
			return "", "", fmt.Errorf("DO: резерв без имени управляющего сигнала не задаёт назначение")
		}
		name += "_DDVH"
	case "AI":
		if strings.TrimSpace(record.Loop) == "" || record.Loop == "-" {
			return "", "", fmt.Errorf("AI: Loop не задан; имя основного/резервного объекта не определено")
		}
		name = record.Loop
		switch placement.Role {
		case "main":
			name += "_main"
		case "redundant":
			name += "_reserve"
		default:
			return "", "", fmt.Errorf("AI: размещение%s не имеет подтверждённого суффикса объекта", placement.Role)
		}
	}
	name = strings.ReplaceAll(name, "-", "_")
	if !strings.HasPrefix(name, "_") {
		name = "_" + name
	}
	if !identifiers.ObjectNamePattern.MatchString(name) {
		return "", "", fmt.Errorf("%s: недопустимое имя SCADA-объекта %q", record.Kind, name)
	}
	return name, member, nil
}

// mustHardwareType возвращает тип уже проверенного направления при окончательной сборке групп профиля.
func mustHardwareType(kind string) string { profile, _ := Lookup(kind); return profile.HardwareType }
