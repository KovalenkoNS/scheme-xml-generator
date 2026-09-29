// Преобразование строки подготовленного назначения в общую модель канала; ограничения выражений принадлежат этому адаптеру.
package prepared

import (
	"fmt"
	model "scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/domain/hardware"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strconv"
	"strings"
	"unicode"
)

type Plan = model.Plan
type Group = model.Group
type Module = model.Module
type Channel = model.Channel

// ParseRow проверяет адрес в выражении и накапливает назначение канала по фактическим данным строки.
// AI требует согласованные Measurement/Quality; аппаратные ID не выводятся из номера слота.
func ParseRow(row xlsx.Row, columns map[string]string, kind string, modules map[string]*assembly.Module, aiOwners map[string]string, plan *Plan) error {
	get := func(name string) string { return strings.TrimSpace(row.Cells[columns[name]]) }
	for name, column := range columns {
		if strings.ContainsFunc(row.Cells[column], func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	scs := get("scs")
	if !fields.ControllerNamePattern.MatchString(scs) {
		return fmt.Errorf("недопустимое имя SCS %q", scs)
	}
	name, prefix, _, err := fields.NormalizeModule(get("module"))
	if err != nil {
		return err
	}
	main, _, _, err := fields.NormalizeModule(get("mainmodule"))
	if err != nil {
		return fmt.Errorf("Main_module: %w", err)
	}
	redundant := get("redundantmodule")
	if redundant == "-" {
		redundant = ""
	}
	if redundant != "" {
		redundant, _, _, err = fields.NormalizeModule(redundant)
		if err != nil || redundant == main {
			return fmt.Errorf("недопустимый Redundant_module %q", get("redundantmodule"))
		}
	}
	if kind == "AI" && name != main && name != redundant {
		return fmt.Errorf("Module %s не совпадает с Main_module или Redundant_module", name)
	}
	// In the DO sample each output drives four distinct modules. Half of the
	// physical modules differ from both metadata columns; the assignment and
	// Module column, rather than that pair, define the actual destination.
	moduleType, objectType := "AI16H", "AD3_v2"
	if kind == "DO" {
		moduleType, objectType = "DO32P", "D32V"
	}
	capacity := hardware.ChannelCount(moduleType)
	channel, err := strconv.Atoi(get("channel"))
	if err != nil || channel < 0 || channel >= capacity {
		return fmt.Errorf("канал %q вне диапазона 0…%d", get("channel"), capacity-1)
	}
	template := get("шаблон")
	if kind == "AI" && (get("типобъекта") != "AD3_v2" || template != "AD3_v2") || kind == "DO" && template != "DO-1" && template != "D32V" && template != "простой" {
		return fmt.Errorf("неподдерживаемый тип объекта или шаблон %q для %s", template, kind)
	}
	ioType := get("iotype")
	if !strings.HasPrefix(strings.ToUpper(ioType), kind) {
		return fmt.Errorf("I/O Type %q не соответствует %s", ioType, kind)
	}
	loop, cabinet := get("loop"), get("mashallingcabinet")
	if loop == "" || cabinet == "" {
		return fmt.Errorf("не заполнены Loop или Mashalling_cabinet")
	}
	controllerID := get("controllerid")
	if controllerID != "" {
		id, err := strconv.ParseInt(controllerID, 10, 64)
		if err != nil || id < 0 {
			return fmt.Errorf("ControllerID должен быть неотрицательным целым числом")
		}
	}
	var tag, member, physical, physicalChannel string
	mask := 1
	if kind == "AI" {
		match := aiValue.FindStringSubmatch(get("scsai"))
		if match == nil {
			match = aiQuality.FindStringSubmatch(get("scsai"))
			mask = 2
		}
		if match == nil {
			return fmt.Errorf("SCS AI: ожидается ТЕГ.Xin := _IO_I*МОДУЛЬ*_AI16H_канал_VAL.Measurement; или ТЕГ.Xs := QUAL_STAT(_IO_I*МОДУЛЬ*_AI16H_канал_VAL.Quality);")
		}
		tag, physical, physicalChannel = match[1], match[2], match[3]
		if tag != get("марка") {
			return fmt.Errorf("марка не совпадает с тегом назначения SCS AI")
		}
	} else {
		match := doValue.FindStringSubmatch(get("scsdo"))
		if match == nil {
			return fmt.Errorf("SCS DO: ожидается _IO_Q*МОДУЛЬ*_DO32P_канал_VAL.Measurement := ТЕГ;")
		}
		physical, physicalChannel, tag, member = match[1], match[2], match[3], match[4]
		// The prepared expression is authoritative. Its RHS can be derived
		// from Loop and need not equal Марка or Марка + _DDVH. Raw IO retains
		// its separate Tag No naming rule.
	}
	physical, _, _, err = fields.NormalizeModule(physical)
	physicalNumber, channelErr := strconv.Atoi(physicalChannel)
	if err != nil || channelErr != nil || physical != name || physicalNumber != channel {
		return fmt.Errorf("Module/Channel не совпадают с физическим адресом в SCS %s", kind)
	}
	moduleKey := strings.ToUpper(scs + ":" + name)
	groupKey := scs + ":" + kind + ":" + prefix
	parsed := modules[moduleKey]
	if parsed == nil {
		parsed = &assembly.Module{
			PreparedAIPair: kind == "AI",
			Module:         Module{Name: name, Type: moduleType, ObjectType: objectType, Template: template, Capacity: capacity, MainModule: main, RedundantModule: redundant, IOType: ioType, MarshallingCabinet: cabinet, ControllerID: controllerID, SourceRow: row.Number},
			Group:          Group{Key: groupKey, ControllerName: scs, Kind: kind, Prefix: prefix, POUName: kind + "_" + prefix, Modules: []Module{}},
			Channels:       map[int]*assembly.Channel{},
		}
		modules[moduleKey] = parsed
	} else {
		m := &parsed.Module
		if parsed.Group.Kind != kind || m.Type != moduleType || m.ObjectType != objectType || m.Capacity != capacity || m.ControllerID != controllerID || kind == "AI" && (m.MainModule != main || m.RedundantModule != redundant || m.IOType != ioType || m.MarshallingCabinet != cabinet || m.Template != template) {
			return fmt.Errorf("противоречивые данные модуля %s (первая строка %d)", name, m.SourceRow)
		}
		if kind == "DO" {
			// Module/Channel and the physical address own the destination.
			// Row-level DO metadata can vary within one physical module; an
			// empty aggregate must not present the first row as universal.
			for _, field := range []struct {
				current *string
				value   string
			}{{&m.MainModule, main}, {&m.RedundantModule, redundant}, {&m.IOType, ioType}, {&m.MarshallingCabinet, cabinet}, {&m.Template, template}} {
				if *field.current != field.value {
					*field.current = ""
				}
			}
		}
	}
	channelTemplate := ""
	if kind == "DO" {
		channelTemplate = template
	}
	reserve := false
	switch strings.ToLower(loop) {
	case "резерв", "reserve", "spare":
		reserve = true
	}
	old := parsed.Channels[channel]
	if old != nil {
		if old.Channel.Tag != tag || old.Channel.Member != member || old.Channel.Template != channelTemplate || old.SourceLoop != loop || old.Channel.Reserve != reserve {
			return fmt.Errorf("конфликт назначений %s, канал %d (первая строка %d)", name, channel, old.Channel.SourceRow)
		}
		if kind == "DO" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s/%s, канал %d: повторное назначение строки %d совпадает со строкой %d и объединено", scs, name, channel, row.Number, old.Channel.SourceRow))
			return nil
		}
		if old.Parts&mask != 0 {
			return fmt.Errorf("повторное назначение AI для %s, канал %d", name, channel)
		}
		old.Parts |= mask
		return nil
	}
	if kind == "AI" {
		ownerKey := strings.ToUpper(scs + ":" + tag)
		target := fmt.Sprintf("%s/%d", name, channel)
		if previous := aiOwners[ownerKey]; previous != "" && previous != target {
			return fmt.Errorf("тег AI %s назначен нескольким физическим входам: %s и %s", tag, previous, target)
		}
		aiOwners[ownerKey] = target
	}
	parsed.Channels[channel] = &assembly.Channel{Channel: Channel{Channel: channel, Tag: tag, Member: member, Template: channelTemplate, SourceRow: row.Number, Reserve: reserve}, SourceLoop: loop, Parts: mask}
	return nil
}
