// Адаптер исходных DO-строк: Tag No и все явно указанные физические назначения одного сигнала.
package raw

import (
	"fmt"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strconv"
	"strings"
)

// parseDORow переносит исходный DO в общие назначения каналов по фактическим модулям.
// Правило суффикса DDVH принадлежит входному формату; библиотечный генератор его не придумывает.
func (state *State) parseDORow(row xlsx.Row, columns map[string]string, modules map[string]*assembly.Module, plan *Plan) error {
	if err := validateFields(row, columns); err != nil {
		return err
	}
	get := func(name string) string { return strings.TrimSpace(row.Cells[columns[name]]) }
	scs := get("scs")
	if !fields.ControllerNamePattern.MatchString(scs) {
		return fmt.Errorf("недопустимое имя SCS %q", scs)
	}
	channel, err := strconv.Atoi(get("channel"))
	if err != nil || channel < 0 || channel > 31 {
		return fmt.Errorf("DO: канал %q вне диапазона 0…31", get("channel"))
	}
	controllerID := get("controllerid")
	if controllerID != "" {
		id, err := strconv.ParseInt(controllerID, 10, 64)
		if err != nil || id < 0 {
			return fmt.Errorf("ControllerID должен быть неотрицательным целым числом")
		}
	}
	tagNo := get("tagno")
	if empty(tagNo) || reserve(tagNo) {
		return fmt.Errorf("DO: Tag No %q не задаёт управляющую переменную; для SPARE/пустого резерва требуется явное имя, автоматическое имя не создаётся", tagNo)
	}
	tag := strings.ReplaceAll(tagNo, "-", "_")
	if !strings.HasPrefix(tag, "_") {
		tag = "_" + tag
	}
	tag += "_DDVH"
	if !rawIOTagPattern.MatchString(tag) {
		return fmt.Errorf("DO: неверный Tag No %q; имя с ведущим _ и суффиксом _DDVH должно содержать не более 160 букв, цифр и подчёркиваний", tagNo)
	}
	placements := []string{}
	byField := map[string]string{}
	seen := map[string]string{}
	for _, field := range []string{"mainmodule", "mainmodule2", "redundantmodule", "redundantmodule2"} {
		value := get(field)
		if field != "mainmodule" && empty(value) {
			continue
		}
		name, _, _, err := fields.NormalizeModule(strings.ToUpper(value))
		if err != nil {
			return fmt.Errorf("DO %s: %w", field, err)
		}
		if previous := seen[name]; previous != "" {
			return fmt.Errorf("DO: один модуль %s повторён в %s и %s", name, previous, field)
		}
		seen[name], byField[field] = field, name
		placements = append(placements, name)
	}
	for _, name := range placements {
		_, prefix, _, _ := fields.NormalizeModule(name)
		moduleKey := strings.ToUpper(scs + ":" + name)
		parsed := modules[moduleKey]
		if parsed != nil && parsed.Group.Kind != "DO" {
			return fmt.Errorf("противоречивые типы физического модуля %s/%s: %s и DO", scs, name, parsed.Group.Kind)
		}
		if parsed == nil {
			parsed = &assembly.Module{
				Module: Module{Name: name, Type: "DO32P", ObjectType: "D32V", Template: "D32V", Capacity: hardware.ChannelCount("DO32P"),
					MainModule: byField["mainmodule"], RedundantModule: byField["redundantmodule"], IOType: get("iotype"), MarshallingCabinet: get("mashallingcabinet"), ControllerID: controllerID, SourceRow: row.Number},
				Group:    Group{Key: scs + ":DO:" + prefix, ControllerName: scs, Kind: "DO", Prefix: prefix, POUName: "DO_" + prefix, Modules: []Module{}},
				Channels: map[int]*assembly.Channel{},
			}
			modules[moduleKey] = parsed
		} else {
			if parsed.Module.ControllerID != controllerID {
				return fmt.Errorf("DO: противоречивый ControllerID модуля %s/%s", scs, name)
			}
			// Cabinet and I/O flavour describe rows, not ownership of a module.
			if parsed.Module.IOType != get("iotype") {
				parsed.Module.IOType = ""
			}
			if parsed.Module.MarshallingCabinet != get("mashallingcabinet") {
				parsed.Module.MarshallingCabinet = ""
			}
			if parsed.Module.MainModule != byField["mainmodule"] {
				parsed.Module.MainModule = ""
			}
			if parsed.Module.RedundantModule != byField["redundantmodule"] {
				parsed.Module.RedundantModule = ""
			}
		}
		if previous := parsed.Channels[channel]; previous != nil {
			if !strings.EqualFold(previous.Channel.Tag, tag) || previous.Channel.Member != "" {
				return fmt.Errorf("DO: конфликт назначений %s/%s, канал %d (первая строка %d)", scs, name, channel, previous.Channel.SourceRow)
			}
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s/%s, канал %d: повторное назначение DO строки %d совпадает со строкой %d и объединено", scs, name, channel, row.Number, previous.Channel.SourceRow))
			continue
		}
		parsed.Channels[channel] = &assembly.Channel{Channel: Channel{Channel: channel, Tag: tag, SourceRow: row.Number, Reserve: reserve(get("loop"))}, SourceLoop: get("loop"), Parts: 1}
	}
	return nil
}
