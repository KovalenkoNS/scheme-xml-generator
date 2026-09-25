// Package skzmap reads PLC850 SKZ assignment workbooks without inferring
// numeric physical module IDs or filling channels absent from the source.
package skzmap

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"scheme-xml-generator/internal/iomap"
)

type Plan struct {
	Groups   []Group  `json:"groups"`
	Warnings []string `json:"warnings"`
}

type Group struct {
	Key     string   `json:"key"`
	SCS     string   `json:"scs"`
	Kind    string   `json:"kind"`
	Prefix  string   `json:"prefix"`
	POUName string   `json:"pouName"`
	Modules []Module `json:"modules"`
}

type Module struct {
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	ObjectType         string    `json:"objectType"`
	Template           string    `json:"template"`
	Capacity           int       `json:"capacity"`
	MainModule         string    `json:"mainModule"`
	RedundantModule    string    `json:"redundantModule"`
	IOType             string    `json:"ioType"`
	MarshallingCabinet string    `json:"marshallingCabinet"`
	ControllerID       string    `json:"controllerId,omitempty"`
	SourceRow          int       `json:"sourceRow"`
	Channels           []Channel `json:"channels"`
}

type Channel struct {
	Channel   int    `json:"channel"`
	Tag       string `json:"tag"`
	Member    string `json:"member,omitempty"`
	SourceRow int    `json:"sourceRow"`
	Reserve   bool   `json:"reserve"`
}

const identifierText = `[A-Za-z_][A-Za-z0-9_]{0,159}`
const physicalModuleText = `A[0-9]{1,6}[-_][0-9]{1,4}`

var (
	scsPattern    = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)
	modulePattern = regexp.MustCompile(`^A([0-9]{1,6})[-_]([0-9]{1,4})$`)
	aiValue       = regexp.MustCompile(`^(` + identifierText + `)\.Xin\s*:=\s*_IO_I\*(` + physicalModuleText + `)\*_AI16H_([0-9]{1,2})_VAL\.Measurement\s*;\s*$`)
	aiQuality     = regexp.MustCompile(`^(` + identifierText + `)\.Xs\s*:=\s*QUAL_STAT\(\s*_IO_I\*(` + physicalModuleText + `)\*_AI16H_([0-9]{1,2})_VAL\.Quality\s*\)\s*;\s*$`)
	doValue       = regexp.MustCompile(`^_IO_Q\*(` + physicalModuleText + `)\*_DO32P_([0-9]{1,2})_VAL\.Measurement\s*:=\s*(` + identifierText + `)(?:\.(` + identifierText + `))?\s*;\s*$`)
)

type parsedChannel struct {
	channel Channel
	loop    string
	mask    int
}

type parsedModule struct {
	module   Module
	group    Group
	channels map[int]*parsedChannel
}

func Parse(data []byte) (*Plan, error) {
	sheets, err := iomap.ReadWorkbook(data)
	if err != nil {
		return nil, err
	}
	return ParseSheets(sheets)
}

func headerKey(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(text))
}

func readHeader(row iomap.Row) (map[string]string, string, error) {
	columns := map[string]string{}
	for column, value := range row.Cells {
		key := headerKey(value)
		switch key {
		case "loop", "scs", "mashallingcabinet", "marshallingcabinet", "module", "channel", "scsai", "scsdo", "mainmodule", "redundantmodule", "iotype", "типобъекта", "шаблон", "controllerid", "марка":
			if key == "marshallingcabinet" {
				key = "mashallingcabinet"
			}
			if columns[key] != "" {
				return nil, "", fmt.Errorf("повторный заголовок %q", value)
			}
			columns[key] = column
		}
	}
	kind := ""
	if columns["scsai"] != "" {
		kind = "AI"
	}
	if columns["scsdo"] != "" {
		if kind != "" {
			return nil, "", fmt.Errorf("столбцы SCS AI и SCS DO должны быть на отдельных листах")
		}
		kind = "DO"
	}
	if kind == "" {
		if columns["scs"] != "" && (columns["module"] != "" || columns["channel"] != "" || columns["марка"] != "") {
			return nil, "", fmt.Errorf("в таблице СКЗ отсутствует заголовок SCS AI или SCS DO")
		}
		return nil, "", nil
	}
	required := []string{"loop", "scs", "mashallingcabinet", "module", "channel", "mainmodule", "redundantmodule", "iotype", "шаблон", "марка"}
	if kind == "AI" {
		required = append(required, "типобъекта")
	}
	for _, key := range required {
		if columns[key] == "" {
			return nil, "", fmt.Errorf("не найден обязательный столбец %s", key)
		}
	}
	return columns, kind, nil
}

func normalizeModule(value string) (string, string, int, error) {
	parts := modulePattern.FindStringSubmatch(value)
	if parts == nil {
		return "", "", 0, fmt.Errorf("неверное имя модуля %q: ожидается A1-00", value)
	}
	rack, _ := strconv.Atoi(parts[1])
	slot, _ := strconv.Atoi(parts[2])
	if rack < 1 || slot > 4095 {
		return "", "", 0, fmt.Errorf("неверный крейт или номер модуля %q: допустимы номера 0…4095", value)
	}
	prefix := fmt.Sprintf("A%d", rack)
	return fmt.Sprintf("%s-%02d", prefix, slot), prefix, slot, nil
}

func emptyRow(row iomap.Row) bool {
	for _, value := range row.Cells {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func ParseSheets(sheets []iomap.Sheet) (*Plan, error) {
	plan := &Plan{Groups: []Group{}, Warnings: []string{}}
	modules := map[string]*parsedModule{}
	aiOwners := map[string]string{}
	rows, tables := 0, 0
	for _, sheet := range sheets {
		var columns map[string]string
		kind, headerRow := "", 0
		for _, row := range sheet.Rows {
			if row.Number > 30 {
				break
			}
			candidate, candidateKind, err := readHeader(row)
			if err != nil {
				return nil, fmt.Errorf("%s, строка %d: %w", sheet.Name, row.Number, err)
			}
			if candidateKind != "" {
				columns, kind, headerRow = candidate, candidateKind, row.Number
				break
			}
		}
		if columns == nil {
			continue
		}
		tables++
		for _, row := range sheet.Rows {
			if row.Number <= headerRow || emptyRow(row) {
				continue
			}
			rows++
			if rows > 50000 {
				return nil, fmt.Errorf("СКЗ: допустимо не более 50000 строк назначений")
			}
			err := parseRow(row, columns, kind, modules, aiOwners, plan)
			if err != nil {
				return nil, fmt.Errorf("%s, строка %d: %w", sheet.Name, row.Number, err)
			}
		}
	}
	if tables == 0 || len(modules) == 0 {
		return nil, fmt.Errorf("СКЗ: не найдена таблица назначений с SCS AI или SCS DO")
	}
	if len(modules) > 4096 {
		return nil, fmt.Errorf("СКЗ: допустимо не более 4096 модулей")
	}
	groupIndices := map[string]int{}
	for _, parsed := range modules {
		module := parsed.module
		module.Channels = []Channel{}
		for _, channel := range parsed.channels {
			if parsed.group.Kind == "AI" && channel.mask != 3 {
				return nil, fmt.Errorf("СКЗ, строка %d: для %s/%s, канал %d нужны оба назначения Xin и Xs", channel.channel.SourceRow, parsed.group.SCS, module.Name, channel.channel.Channel)
			}
			module.Channels = append(module.Channels, channel.channel)
		}
		sort.Slice(module.Channels, func(i, j int) bool { return module.Channels[i].Channel < module.Channels[j].Channel })
		gi, exists := groupIndices[parsed.group.Key]
		if !exists {
			gi = len(plan.Groups)
			groupIndices[parsed.group.Key] = gi
			plan.Groups = append(plan.Groups, parsed.group)
		}
		plan.Groups[gi].Modules = append(plan.Groups[gi].Modules, module)
	}
	sort.Slice(plan.Groups, func(i, j int) bool {
		a, b := plan.Groups[i], plan.Groups[j]
		if a.SCS != b.SCS {
			return a.SCS < b.SCS
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
			_, _, slotA, _ := normalizeModule(plan.Groups[i].Modules[a].Name)
			_, _, slotB, _ := normalizeModule(plan.Groups[i].Modules[b].Name)
			return slotA < slotB
		})
	}
	return plan, nil
}

func parseRow(row iomap.Row, columns map[string]string, kind string, modules map[string]*parsedModule, aiOwners map[string]string, plan *Plan) error {
	get := func(name string) string { return strings.TrimSpace(row.Cells[columns[name]]) }
	for name, column := range columns {
		if strings.ContainsFunc(row.Cells[column], func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	scs := get("scs")
	if !scsPattern.MatchString(scs) {
		return fmt.Errorf("недопустимое имя SCS %q", scs)
	}
	name, prefix, _, err := normalizeModule(get("module"))
	if err != nil {
		return err
	}
	main, _, _, err := normalizeModule(get("mainmodule"))
	if err != nil {
		return fmt.Errorf("Main_module: %w", err)
	}
	redundant := get("redundantmodule")
	if redundant == "-" {
		redundant = ""
	}
	if redundant != "" {
		redundant, _, _, err = normalizeModule(redundant)
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
	moduleType, objectType, capacity := "AI16H", "AD3_v2", 16
	if kind == "DO" {
		moduleType, objectType, capacity = "DO32P", "D32V", 32
	}
	channel, err := strconv.Atoi(get("channel"))
	if err != nil || channel < 0 || channel >= capacity {
		return fmt.Errorf("канал %q вне диапазона 0…%d", get("channel"), capacity-1)
	}
	template := get("шаблон")
	if kind == "AI" && (get("типобъекта") != "AD3_v2" || template != "AD3_v2") || kind == "DO" && template != "DO-1" && template != "D32V" {
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
		if mark := get("марка"); mark == "" || tag != mark && tag != mark+"_DDVH" {
			return fmt.Errorf("марка не совпадает с исходной переменной SCS DO")
		}
	}
	physical, _, _, err = normalizeModule(physical)
	physicalNumber, channelErr := strconv.Atoi(physicalChannel)
	if err != nil || channelErr != nil || physical != name || physicalNumber != channel {
		return fmt.Errorf("Module/Channel не совпадают с физическим адресом в SCS %s", kind)
	}
	moduleKey := scs + ":" + name
	groupKey := scs + ":" + kind + ":" + prefix
	parsed := modules[moduleKey]
	if parsed == nil {
		parsed = &parsedModule{
			module:   Module{Name: name, Type: moduleType, ObjectType: objectType, Template: template, Capacity: capacity, MainModule: main, RedundantModule: redundant, IOType: ioType, MarshallingCabinet: cabinet, ControllerID: controllerID, SourceRow: row.Number},
			group:    Group{Key: groupKey, SCS: scs, Kind: kind, Prefix: prefix, POUName: kind + "_" + prefix, Modules: []Module{}},
			channels: map[int]*parsedChannel{},
		}
		modules[moduleKey] = parsed
	} else if m := parsed.module; parsed.group.Kind != kind || m.MainModule != main || m.RedundantModule != redundant || m.IOType != ioType || m.MarshallingCabinet != cabinet || m.ControllerID != controllerID || m.Template != template {
		return fmt.Errorf("противоречивые данные модуля %s (первая строка %d)", name, m.SourceRow)
	}
	reserve := false
	switch strings.ToLower(loop) {
	case "резерв", "reserve", "spare":
		reserve = true
	}
	old := parsed.channels[channel]
	if old != nil {
		if old.channel.Tag != tag || old.channel.Member != member || old.loop != loop || old.channel.Reserve != reserve {
			return fmt.Errorf("конфликт назначений %s, канал %d (первая строка %d)", name, channel, old.channel.SourceRow)
		}
		if kind == "DO" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s/%s, канал %d: повторное назначение строки %d совпадает со строкой %d и объединено", scs, name, channel, row.Number, old.channel.SourceRow))
			return nil
		}
		if old.mask&mask != 0 {
			return fmt.Errorf("повторное назначение AI для %s, канал %d", name, channel)
		}
		old.mask |= mask
		return nil
	}
	if kind == "AI" {
		ownerKey := scs + ":" + strings.ToUpper(tag)
		target := fmt.Sprintf("%s/%d", name, channel)
		if previous := aiOwners[ownerKey]; previous != "" && previous != target {
			return fmt.Errorf("тег AI %s назначен нескольким физическим входам: %s и %s", tag, previous, target)
		}
		aiOwners[ownerKey] = target
	}
	parsed.channels[channel] = &parsedChannel{channel: Channel{Channel: channel, Tag: tag, Member: member, SourceRow: row.Number, Reserve: reserve}, loop: loop, mask: mask}
	return nil
}
