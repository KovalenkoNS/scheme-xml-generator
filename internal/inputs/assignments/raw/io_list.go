// Адаптер исходного IO-листа SCS читает сигналы, размещения и шкалы; не выбирает аппаратные типы, SCADA-имена или FBD-шаблоны.
package raw

import (
	"fmt"
	model "scheme-xml-generator/internal/domain/assignments"
	iorecords "scheme-xml-generator/internal/domain/io"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strconv"
	"strings"
	"unicode"
)

type Plan = model.Plan

type State struct {
	Sheet           string
	recordPositions map[string]iorecords.Record
}

// NewState создаёт индекс физических позиций одной книги для обнаружения конфликтов исходных сигналов.
func NewState() *State {
	return &State{recordPositions: map[string]iorecords.Record{}}
}

// Header распознаёт исходный SCS IO-лист и проверяет единственность его полей, отдельно от подготовленных выражений.
func Header(row xlsx.Row) (map[string]string, bool, error) {
	columns := map[string]string{}
	duplicates := []string{}
	for column, value := range row.Cells {
		key := fields.HeaderKey(value)
		switch key {
		case "tagno", "scs", "iotype", "mainmodule", "redundantmodule", "channel", "loop", "mainmodule2", "redundantmodule2", "mainchassis", "redundandchassis", "redundantchassis", "mashallingcabinet", "marshallingcabinet", "controllerid", "scsai", "scsdo", "typno", "service", "description", "min", "max", "unit", "min1", "max1", "unit1", "slot":
			if key == "marshallingcabinet" {
				key = "mashallingcabinet"
			}
			if key == "redundantchassis" {
				key = "redundandchassis"
			}
			if columns[key] != "" {
				duplicates = append(duplicates, value)
			}
			columns[key] = column
		}
	}
	// Tag No distinguishes the source IO inventory from prepared AI/DO maps.
	found := columns["tagno"] != "" && (columns["scs"] != "" || columns["mainmodule"] != "" || columns["iotype"] != "")
	if !found {
		return nil, false, nil
	}
	if columns["scsai"] != "" || columns["scsdo"] != "" {
		return nil, true, fmt.Errorf("исходная IO-карта (Tag No) и подготовленные SCS AI/SCS DO должны быть на отдельных листах")
	}
	if len(duplicates) != 0 {
		return nil, true, fmt.Errorf("повторный заголовок исходной IO-карты %q", duplicates[0])
	}
	for _, key := range []string{"tagno", "scs", "iotype", "mainmodule", "channel"} {
		if columns[key] == "" {
			return nil, true, fmt.Errorf("исходная IO-карта: не найден обязательный столбец %s", key)
		}
	}
	return columns, true, nil
}

// validateFields проверяет исходные ячейки и номер строки перед предметным разбором DI/DO.
func validateFields(row xlsx.Row, columns map[string]string) error {
	if row.Number < 1 {
		return fmt.Errorf("IO: номер строки источника должен быть положительным")
	}
	for name, column := range columns {
		if strings.ContainsFunc(row.Cells[column], func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	for _, name := range []string{"tagno", "scs", "iotype", "mainmodule", "redundantmodule", "channel", "mainmodule2", "redundantmodule2", "controllerid"} {
		if strings.ContainsFunc(row.Cells[columns[name]], unicode.IsControl) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	return nil
}

// empty распознаёт отсутствие значения по контракту исходного IO-листа.
func empty(value string) bool { return value == "" || value == "-" }

// reserve распознаёт явную отметку резерва, не создавая имя сигнала.
func reserve(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "spare", "reserve", "резерв":
		return true
	}
	return false
}

// ParseRow сохраняет сигнал, все явные размещения и обе шкалы исходной строки.
// Строки без физических адресов учитываются отдельно; неизвестные адресованные типы отклоняются с конкретной причиной.
func (state *State) ParseRow(row xlsx.Row, columns map[string]string, plan *Plan) (string, error) {
	if err := validateFields(row, columns); err != nil {
		return "", err
	}
	if plan.Source == nil {
		plan.Source = &iorecords.Source{Records: []iorecords.Record{}, Excluded: []iorecords.ExcludedRow{}}
	}
	get := func(key string) string { return strings.TrimSpace(row.Cells[columns[key]]) }
	location := iorecords.Location{Sheet: state.Sheet, Row: row.Number}
	ioType := get("iotype")
	kind := ""
	switch strings.ToUpper(strings.ReplaceAll(ioType, " ", "")) {
	case "DIR(I)-NAMUR", "DIR-VFC", "DIR(SCS1)":
		kind = "DI"
		if columns["redundantmodule"] == "" {
			return "", fmt.Errorf("исходная DI-карта: не найден обязательный столбец redundantmodule")
		}
	case "DOR-P", "DOR-VFC", "DOR(SCS3)":
		kind = "DO"
	case "AIR(I)-LP", "AIR-EP":
		kind = "AI"
	default:
		addressed := false
		for _, field := range []string{"mainmodule", "mainmodule2", "redundantmodule", "redundantmodule2", "channel"} {
			addressed = addressed || !empty(get(field))
		}
		if addressed || ioType != "" && ioType != "-" && !strings.EqualFold(ioType, "S") {
			return "", fmt.Errorf("I/O Type %q: правило чтения такого типа не определено; строка не пропущена", ioType)
		}
		reason := "тип IO не указан; физических адресов нет"
		if strings.EqualFold(ioType, "S") {
			reason = "тип S; физических адресов нет"
		}
		plan.Source.Excluded = append(plan.Source.Excluded, iorecords.ExcludedRow{Location: location, IOType: ioType, Reason: reason})
		return "", nil
	}
	controller := get("scs")
	if !fields.ControllerNamePattern.MatchString(controller) {
		return "", fmt.Errorf("SCS %q: не задано допустимое имя ПЛК; пустой Тег_CPU его не заменяет", controller)
	}
	channel, err := strconv.Atoi(get("channel"))
	if err != nil || channel < 0 || channel > 4095 {
		return "", fmt.Errorf("Channel %q: требуется целое неотрицательное число до4095", get("channel"))
	}
	tag := get("tagno")
	if empty(tag) {
		return "", fmt.Errorf("Tag No не заполнен для физического сигнала %s", kind)
	}
	record := iorecords.Record{
		Location: location, ControllerName: controller, Kind: kind,
		SignalName: tag, Loop: get("loop"), SignalType: get("typno"), IOType: ioType,
		Description: get("service"), Placements: []iorecords.Placement{}, Reserve: reserve(tag),
		ControllerID: get("controllerid"), MarshallingCabinet: get("mashallingcabinet"),
	}
	if record.ControllerID != "" {
		if value, err := strconv.ParseInt(record.ControllerID, 10, 64); err != nil || value < 0 {
			return "", fmt.Errorf("ControllerID %q: требуется неотрицательное целое число", record.ControllerID)
		}
	}
	if get("description") != "" {
		record.Description = get("description")
	}
	if kind == "AI" {
		for _, suffix := range []string{"", "1"} {
			record.Ranges = append(record.Ranges, iorecords.Range{Minimum: get("min" + suffix), Maximum: get("max" + suffix), Unit: get("unit" + suffix)})
		}
	}
	seen := map[string]string{}
	for _, field := range []string{"mainmodule", "mainmodule2", "redundantmodule", "redundantmodule2"} {
		value := get(field)
		if field != "mainmodule" && empty(value) {
			continue
		}
		name, rack, slot, err := fields.NormalizeModule(strings.ToUpper(value))
		if err != nil {
			return "", fmt.Errorf("%s %q: %w", field, value, err)
		}
		if previous := seen[name]; previous != "" {
			return "", fmt.Errorf("модуль %s повторён в полях %s и %s", name, previous, field)
		}
		seen[name] = field
		if field == "mainmodule" {
			if !empty(get("mainchassis")) && !strings.EqualFold(get("mainchassis"), rack) {
				return "", fmt.Errorf("Main_Chassis %q не совпадает с крейтом %s из Main_module", get("mainchassis"), rack)
			}
			if !empty(get("slot")) {
				declaredSlot, err := strconv.Atoi(get("slot"))
				if err != nil || declaredSlot != slot {
					return "", fmt.Errorf("Slot %q не совпадает со слотом%d из Main_module", get("slot"), slot)
				}
			}
		}
		position := fmt.Sprintf("%s/%s/%d", strings.ToUpper(controller), name, channel)
		if previous, exists := state.recordPositions[position]; exists && (previous.Kind != kind || !strings.EqualFold(previous.SignalName, tag)) {
			return "", fmt.Errorf("%s: физическая позиция назначена разным сигналам %q и %q; первая запись %s, строка%d", position, previous.SignalName, tag, previous.Sheet, previous.Row)
		}
		state.recordPositions[position] = record
		role := map[string]string{"mainmodule": "main", "mainmodule2": "main2", "redundantmodule": "redundant", "redundantmodule2": "redundant2"}[field]
		record.Placements = append(record.Placements, iorecords.Placement{Module: name, Rack: rack, Slot: slot, Channel: channel, Role: role})
	}
	plan.Source.Records = append(plan.Source.Records, record)
	return kind, nil
}
