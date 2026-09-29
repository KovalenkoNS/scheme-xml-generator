// Адаптер исходных DI-строк: явные основные/резервные размещения и отдельные получатели каналов.
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

// ParseRow направляет поддержанные исходные DI/DO строки в соответствующий разбор.
// DI накапливает размещения и обнаруживает повторные физические позиции; остальные направления пропускает явно.
func (state *State) ParseRow(row xlsx.Row, columns map[string]string, modules map[string]*assembly.Module, plan *Plan) error {
	get := func(name string) string { return strings.TrimSpace(row.Cells[columns[name]]) }
	ioType := get("iotype")
	switch strings.ToUpper(strings.ReplaceAll(ioType, " ", "")) {
	case "DIR(I)-NAMUR", "DIR-VFC", "DIR(SCS1)":
	case "DOR-P", "DOR-VFC", "DOR(SCS3)":
		return state.parseDORow(row, columns, modules, plan)
	default:
		if strings.HasPrefix(strings.ToUpper(ioType), "DI") || strings.HasPrefix(strings.ToUpper(ioType), "DO") {
			return fmt.Errorf("исходная IO-карта: неподдерживаемый I/O Type %q", ioType)
		}
		state.Ignored++
		return nil
	}
	if columns["redundantmodule"] == "" {
		return fmt.Errorf("исходная DI-карта: не найден обязательный столбец redundantmodule")
	}
	if err := validateFields(row, columns); err != nil {
		return err
	}
	for _, name := range []string{"mainmodule2", "redundantmodule2"} {
		if !empty(get(name)) {
			return fmt.Errorf("DI: %s=%q не поддерживается; дополнительные физические размещения требуют отдельного правила", name, get(name))
		}
	}
	scs := get("scs")
	if !fields.ControllerNamePattern.MatchString(scs) {
		return fmt.Errorf("недопустимое имя SCS %q", scs)
	}
	main, _, _, err := fields.NormalizeModule(strings.ToUpper(get("mainmodule")))
	if err != nil {
		return fmt.Errorf("Main_module: %w", err)
	}
	redundant := get("redundantmodule")
	if empty(redundant) {
		redundant = ""
	} else {
		redundant, _, _, err = fields.NormalizeModule(strings.ToUpper(redundant))
		if err != nil || redundant == main {
			return fmt.Errorf("недопустимый Redundant_module %q", get("redundantmodule"))
		}
	}
	channel, err := strconv.Atoi(get("channel"))
	if err != nil || channel < 0 || channel > 31 {
		return fmt.Errorf("DI: канал %q вне диапазона 0…31", get("channel"))
	}
	controllerID := get("controllerid")
	if controllerID != "" {
		id, err := strconv.ParseInt(controllerID, 10, 64)
		if err != nil || id < 0 {
			return fmt.Errorf("ControllerID должен быть неотрицательным целым числом")
		}
	}
	tagNo := get("tagno")
	spare := strings.EqualFold(tagNo, "SPARE")
	tag := strings.ReplaceAll(tagNo, "-", "_")
	if !strings.HasPrefix(tag, "_") {
		tag = "_" + tag
	}
	if !spare && (empty(tagNo) || !rawIOTagPattern.MatchString(tag)) {
		return fmt.Errorf("DI: неверный Tag No %q; ожидается имя DDR из букв, цифр, дефисов и подчёркиваний", tagNo)
	}
	for index, name := range []string{main, redundant} {
		if name == "" {
			continue
		}
		_, prefix, _, _ := fields.NormalizeModule(name)
		moduleKey := strings.ToUpper(scs + ":" + name)
		parsed := modules[moduleKey]
		if parsed != nil && parsed.Group.Kind != "DI" {
			return fmt.Errorf("противоречивые типы физического модуля %s/%s: %s и DI", scs, name, parsed.Group.Kind)
		}
		position := fmt.Sprintf("%s/%d", moduleKey, channel)
		if sourceRow := state.positions[position]; sourceRow != 0 {
			return fmt.Errorf("DI: повторное или конфликтующее назначение %s/%s, канал %d (первая строка %d)", scs, name, channel, sourceRow)
		}
		state.positions[position] = row.Number
		if parsed == nil {
			parsed = &assembly.Module{
				Module: Module{Name: name, Type: "DI32", ObjectType: "D32V", Template: "D32V", Capacity: hardware.ChannelCount("DI32"),
					MainModule: main, RedundantModule: redundant, IOType: ioType, MarshallingCabinet: get("mashallingcabinet"), ControllerID: controllerID, SourceRow: row.Number},
				Group:    Group{Key: scs + ":DI:" + prefix, ControllerName: scs, Kind: "DI", Prefix: prefix, POUName: "DI_" + prefix, Modules: []Module{}},
				Channels: map[int]*assembly.Channel{},
			}
			modules[moduleKey] = parsed
		} else {
			if parsed.Module.ControllerID != controllerID {
				return fmt.Errorf("DI: противоречивый ControllerID модуля %s/%s", scs, name)
			}
			// These columns describe individual rows, not a stable module type or
			// controller ownership. SCS alone assigns the controller; varied row
			// metadata is omitted from the aggregate rather than falsely rejected.
			if parsed.Module.IOType != ioType {
				parsed.Module.IOType = ""
			}
			if parsed.Module.MarshallingCabinet != get("mashallingcabinet") {
				parsed.Module.MarshallingCabinet = ""
			}
			if parsed.Module.MainModule != main {
				parsed.Module.MainModule = ""
			}
			if parsed.Module.RedundantModule != redundant {
				parsed.Module.RedundantModule = ""
			}
		}
		if spare {
			continue
		}
		member := fmt.Sprintf("C%d", index+1)
		owner := strings.ToUpper(scs + ":" + tag + "." + member)
		if previous := state.owners[owner]; previous != "" {
			return fmt.Errorf("DI: получатель %s.%s назначен нескольким физическим каналам: %s и %s", tag, member, previous, position)
		}
		state.owners[owner] = position
		parsed.Channels[channel] = &assembly.Channel{Channel: Channel{Channel: channel, Tag: tag, Member: member, SourceRow: row.Number, Reserve: reserve(get("loop"))}, SourceLoop: get("loop"), Parts: 1}
	}
	if spare {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("DI, строка %d: SPARE на %s/%s%s, канал %d — модули сохранены, получатель DDR не создан.", row.Number, scs, main, redundantSuffix(redundant), channel))
	}
	return nil
}

// redundantSuffix добавляет имя резервного модуля только к сообщению о строке DI.
func redundantSuffix(module string) string {
	if module == "" {
		return ""
	}
	return "/" + module
}
