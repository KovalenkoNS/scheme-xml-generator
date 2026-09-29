// Адаптер исходной таблицы физического IO: столбцы, размещения и правила инженерных имён.
package iomap

import (
	"fmt"
	"regexp"
	"scheme-xml-generator/internal/domain/hardware"
	"sort"
	"strconv"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var plcIdentifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var modulePattern = regexp.MustCompile(`^(A[0-9]+)[_-]([0-9]{1,2})$`)
var loopPattern = regexp.MustCompile(`^([0-9]+)-([A-Z]+)-([A-Za-z0-9]+)$`)

// Parse reads the IO inventory. Derived AI/AO spreadsheets are reference data,
// never the source of physical module inventory or channel placement.
func Parse(data []byte) (*Plan, error) {
	sheets, err := ReadWorkbook(data)
	if err != nil {
		return nil, err
	}
	return ParseSheets(sheets)
}

// ParseInventory reads physical placement and occupied/free channels without
// resolving technological signal names. Creating modules and spare objects
// does not depend on the naming rules used by panel XML generation.
func ParseInventory(data []byte) (*Plan, error) {
	sheets, err := ReadWorkbook(data)
	if err != nil {
		return nil, err
	}
	return parseSheets(sheets, true)
}

// headerKey Нормализует первую строку заголовка IO-столбца для поиска известных полей таблицы.
func headerKey(value string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(value), "\n")
	line = strings.ToLower(line)
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, line)
}

// discoverHeaders Сопоставляет столбцы исходного IO-листа с полями адаптера и отмечает неоднозначные дубликаты.
func discoverHeaders(row Row) map[string]string {
	result := map[string]string{}
	for column, value := range row.Cells {
		key := headerKey(value)
		logical := ""
		switch {
		case key == "fcs":
			logical = "fcs"
		case key == "controlcabinet":
			logical = "cabinet"
		case key == "moduletype":
			logical = "type"
		case strings.HasPrefix(key, "mainmodule"):
			logical = "main"
		case strings.HasPrefix(key, "redundandmodule") || strings.HasPrefix(key, "redundantmodule"):
			logical = "redundant"
		case key == "channel":
			logical = "channel"
		case key == "loop":
			logical = "loop"
		case key == "loopno":
			logical = "loopNo"
		case key == "tagno":
			logical = "tagNo"
		case key == "typno":
			logical = "typno"
		case key == "ll" || key == "l" || key == "h" || key == "hh":
			logical = key
		case key == "mainchassis":
			logical = "rack"
		case key == "slot":
			logical = "slot"
		case key == "scadatag" || key == "scadamain":
			logical = "explicit"
		case key == "scadareservetag" || key == "scadareserve":
			logical = "explicitReserve"
		case key == "service":
			logical = "service"
		case key == "description" || key == "signaldescription" || strings.EqualFold(strings.TrimSpace(value), "Назначение"):
			logical = "description"
		}
		if logical != "" {
			if previous := result[logical]; previous != "" {
				result["ambiguous"] = previous + "/" + column
			}
			result[logical] = column
		}
	}
	return result
}

// completeHeaders Проверяет наличие обязательных столбцов физической IO-карты перед выбором листа.
func completeHeaders(headers map[string]string) bool {
	for _, key := range []string{"fcs", "cabinet", "type", "main", "redundant", "channel", "loop", "tagNo"} {
		if headers[key] == "" {
			return false
		}
	}
	return true
}

type sourceRow struct {
	Number                                                          int
	FCS, Cabinet, Type, Main, Redundant, Loop, LoopNo, TagNo, Typno string
	OriginalFCS                                                     string
	Alarms                                                          [4]string
	Channel                                                         int
	Explicit, ExplicitReserve                                       string
}

// empty Распознаёт принятые в IO-таблице обозначения незаполненного значения.
func empty(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "-" || value == "—"
}

// normalizeName Преобразует табличные имена оборудования в идентификаторы с подчёркиванием.
func normalizeName(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "-", "_")
}

// ParseSheets Преобразует прочитанные листы в инвентарь ПЛК с именами сигналов для диагностического генератора.
func ParseSheets(sheets []Sheet) (*Plan, error) {
	return parseSheets(sheets, false)
}

// parseSheets Выбирает единственную IO-таблицу, проверяет размещения и собирает инвентарь; inventoryOnly пропускает вывод технологических имён.
func parseSheets(sheets []Sheet, inventoryOnly bool) (*Plan, error) {
	var source *Sheet
	var header map[string]string
	var headerNumber int
	for i := range sheets {
		for _, row := range sheets[i].Rows {
			if row.Number > 30 {
				break
			}
			candidate := discoverHeaders(row)
			if !completeHeaders(candidate) {
				continue
			}
			if source != nil {
				return nil, fmt.Errorf("IO: найдено несколько таблиц с разметкой; оставьте один лист IO в загружаемой книге")
			}
			source, header, headerNumber = &sheets[i], candidate, row.Number
			break
		}
	}
	if source == nil {
		return nil, fmt.Errorf("IO: не найдены столбцы FCS, Control cabinet, ModuleType, Main module, Redundand module, Channel, Loop и Tag No; нужен исходный IO, а не перекладка AI/AO")
	}
	if header["ambiguous"] != "" {
		return nil, fmt.Errorf("IO: неоднозначные повторные заголовки %s", header["ambiguous"])
	}
	owners, err := rackOwners(*source, header, headerNumber)
	if err != nil {
		return nil, err
	}
	plan := &Plan{SheetName: source.Name, Controllers: []Controller{}, Warnings: []string{}}
	byController := map[string]int{}
	moduleIndices := map[string]map[string]int{}
	occupied := map[string]map[int]Channel{}
	profileCount, inferredCount, skipped := 0, 0, 0
	corrections := map[string][]int{}
	for _, row := range source.Rows {
		if row.Number <= headerNumber {
			continue
		}
		get := func(key string) string { return strings.TrimSpace(row.Cells[header[key]]) }
		description := get("description")
		if empty(description) || strings.EqualFold(description, "Н/Д") || strings.EqualFold(description, "N/A") {
			description = get("service")
		}
		if empty(get("main")) && empty(get("type")) && empty(get("cabinet")) {
			skipped++
			continue
		}
		sr := sourceRow{Number: row.Number, FCS: get("fcs"), Cabinet: normalizeName(get("cabinet")), Type: strings.ToUpper(get("type")), Main: get("main"), Redundant: get("redundant"), Loop: get("loop"), LoopNo: get("loopNo"), TagNo: get("tagNo"), Typno: get("typno"), Explicit: get("explicit"), ExplicitReserve: get("explicitReserve"), Alarms: [4]string{get("ll"), get("l"), get("h"), get("hh")}}
		capacity := moduleCapacity(sr.Type)
		if capacity == 0 {
			return nil, fmt.Errorf("%s, строка %d: неподдерживаемый ModuleType %q (поддерживаются AI16H, AOC4H, DI32, DO32P)", source.Name, row.Number, sr.Type)
		}
		if empty(sr.FCS) || !plcIdentifier.MatchString(sr.FCS) || !plcIdentifier.MatchString(sr.Cabinet) || len(sr.Cabinet) > 120 {
			return nil, fmt.Errorf("IO, строка %d: неверные FCS или Control cabinet", row.Number)
		}
		channel, err := strconv.Atoi(get("channel"))
		if err != nil || channel < 0 || channel >= capacity {
			return nil, fmt.Errorf("IO, строка %d: канал должен быть целым числом 0…%d", row.Number, capacity-1)
		}
		sr.Channel = channel
		mainName, mainRack, mainSlot, err := parseModule(sr.Main)
		if err != nil {
			return nil, fmt.Errorf("IO, строка %d: %w", row.Number, err)
		}
		sr.Main = mainName
		if owner := owners[sr.Cabinet+":"+mainRack]; owner != "" && owner != sr.FCS {
			key := sr.Cabinet + " / " + mainRack + ": " + sr.FCS + " → " + owner
			corrections[key] = append(corrections[key], row.Number)
			sr.OriginalFCS = sr.FCS
			sr.FCS = owner
		}
		if !empty(get("rack")) && get("rack") != mainRack {
			return nil, fmt.Errorf("IO, строка %d: MainChassis не совпадает с Main module", row.Number)
		}
		if !empty(get("slot")) {
			slot, err := strconv.Atoi(get("slot"))
			if err != nil || slot != mainSlot {
				return nil, fmt.Errorf("IO, строка %d: Slot не совпадает с Main module", row.Number)
			}
		}
		if !empty(sr.Redundant) {
			sr.Redundant, _, _, err = parseModule(sr.Redundant)
			if err != nil || sr.Redundant == sr.Main {
				return nil, fmt.Errorf("IO, строка %d: неверный резервный модуль %q", row.Number, sr.Redundant)
			}
		} else {
			sr.Redundant = ""
		}
		if sr.Redundant != "" {
			_, rack, _, _ := parseModule(sr.Redundant)
			if owner := owners[sr.Cabinet+":"+rack]; owner != "" && owner != sr.FCS {
				return nil, fmt.Errorf("IO, строка %d: основной и резервный крейты отнесены к разным FCS", row.Number)
			}
		}
		key := sr.FCS + ":" + sr.Cabinet
		ci, ok := byController[key]
		if !ok {
			if len(plan.Controllers) >= 128 {
				return nil, fmt.Errorf("IO: не более 128 ПЛК")
			}
			ci = len(plan.Controllers)
			byController[key] = ci
			moduleIndices[key] = map[string]int{}
			plan.Controllers = append(plan.Controllers, Controller{Key: key, SourceController: sr.FCS, Name: defaultPLCName(sr.FCS, sr.Cabinet), Cabinet: sr.Cabinet, Modules: []Module{}})
		}
		controller := &plan.Controllers[ci]
		for placement, name := range []string{sr.Main, sr.Redundant} {
			if name == "" {
				continue
			}
			_, rack, slot, _ := parseModule(name)
			mi, ok := moduleIndices[key][name]
			if !ok {
				if plan.ModuleCount >= 4096 {
					return nil, fmt.Errorf("IO: не более 4096 модулей")
				}
				mi = len(controller.Modules)
				moduleIndices[key][name] = mi
				controller.Modules = append(controller.Modules, Module{Name: name, Rack: rack, Slot: slot, Type: sr.Type, Capacity: capacity, Channels: make([]Channel, capacity)})
				plan.ModuleCount++
			} else if controller.Modules[mi].Type != sr.Type {
				return nil, fmt.Errorf("IO, строка %d: модуль %s/%s указан с разными типами", row.Number, sr.FCS, name)
			}
			var tag string
			var fromProfile bool
			hasExplicit := sr.Explicit != "" || placement == 1 && sr.ExplicitReserve != ""
			reserve := empty(sr.TagNo) && !hasExplicit
			if reserve {
				tag = reserveTag(controller.Name, name, channel)
			} else if !inventoryOnly {
				tag, fromProfile, err = signalTag(sr, placement == 1)
				if err != nil {
					return nil, fmt.Errorf("IO, строка %d: %w", row.Number, err)
				}
				if fromProfile {
					profileCount++
				} else if (sr.Type == "AI16H" || sr.Type == "AOC4H") && !hasExplicit {
					inferredCount++
				}
			}
			objectType := map[string]string{"AI16H": "AD3_v2", "AOC4H": "AN_v1"}[sr.Type]
			peer := sr.Redundant
			if placement == 1 {
				peer = sr.Main
			}
			ch := Channel{Channel: channel, Tag: tag, ObjectType: objectType, Reserve: reserve, Redundant: placement == 1, SourceRow: row.Number, SourceTag: sr.TagNo, Description: description, PeerModule: peer}
			switch {
			case reserve:
				ch.BindingSource = "reserve"
			case inventoryOnly:
				ch.BindingSource = "inventory"
			case hasExplicit:
				ch.BindingSource = "explicit"
			case fromProfile:
				ch.BindingSource = "reference"
			default:
				ch.BindingSource = "io-rule"
			}
			address := key + ":" + name
			if occupied[address] == nil {
				occupied[address] = map[int]Channel{}
			}
			if old, exists := occupied[address][channel]; exists {
				return nil, fmt.Errorf("IO: строки %d и %d повторно занимают %s/%s, канал %d", old.SourceRow, row.Number, sr.FCS, name, channel)
			}
			occupied[address][channel] = ch
			controller.Modules[mi].Channels[channel] = ch
		}
		plan.RowCount++
	}
	if len(plan.Controllers) == 0 {
		return nil, fmt.Errorf("IO: не найдено физических модулей")
	}
	for ci := range plan.Controllers {
		controller := &plan.Controllers[ci]
		sort.Slice(controller.Modules, func(i, j int) bool {
			a, b := controller.Modules[i], controller.Modules[j]
			if a.Rack != b.Rack {
				return rackNumber(a.Rack) < rackNumber(b.Rack)
			}
			return a.Slot < b.Slot
		})
		racks := map[string]bool{}
		for mi := range controller.Modules {
			module := &controller.Modules[mi]
			racks[module.Rack] = true
			for c := range module.Channels {
				if module.Channels[c].SourceRow == 0 {
					module.Channels[c] = Channel{Channel: c, Tag: reserveTag(controller.Name, module.Name, c), ObjectType: map[string]string{"AI16H": "AD3_v2", "AOC4H": "AN_v1"}[module.Type], Reserve: true, BindingSource: "reserve"}
				}
			}
			plan.SignalCount += module.Capacity
		}
		rackNames := make([]string, 0, len(racks))
		for name := range racks {
			rackNames = append(rackNames, name)
		}
		sort.Slice(rackNames, func(i, j int) bool { return rackNumber(rackNames[i]) < rackNumber(rackNames[j]) })
		panelCounts := map[string]int{}
		for _, name := range rackNames {
			panel := "front"
			if rackNumber(name)%10 >= 2 {
				panel = "back"
			}
			controller.Racks = append(controller.Racks, Rack{Name: name, Panel: panel, Order: panelCounts[panel]})
			panelCounts[panel]++
		}
	}
	sort.Slice(plan.Controllers, func(i, j int) bool { return plan.Controllers[i].Key < plan.Controllers[j].Key })
	if skipped > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Строки без назначения ПЛК/модуля не создают оборудование: %d.", skipped))
	}
	correctionKeys := make([]string, 0, len(corrections))
	for key := range corrections {
		correctionKeys = append(correctionKeys, key)
	}
	sort.Strings(correctionKeys)
	for _, key := range correctionKeys {
		rows := corrections[key]
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Исправлена принадлежность по физическому шкафу и крейту (%s): %d строк, первая — %d. Исходный Excel не изменён.", key, len(rows), rows[0]))
	}
	if !inventoryOnly {
		plan.Warnings = append(plan.Warnings, "Размещение крейтов: A…0/A…1 — передняя панель, A…2 и далее — задняя. CPU715 в слотах 0/1 первого крейта — правило образца, отсутствующее в сигнальной карте IO.")
	}
	if profileCount > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Для %d аналоговых размещений сохранены уточнённые имена из проверенной перекладки AI/AO; профиль применяется только при совпадении исходных полей IO.", profileCount))
	}
	if inferredCount > 0 {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Для %d аналоговых размещений имя вычислено по правилам IO. Проверьте теги перед импортом; столбцы SCADA Tag / SCADA Reserve Tag позволяют явно задать имена.", inferredCount))
	}
	return plan, nil
}

// moduleCapacity Возвращает число каналов поддержанного типа модуля IO-формата; неизвестный тип получает ноль.
func moduleCapacity(kind string) int {
	module, _ := hardware.Lookup(kind)
	return module.Channels
}

// parseModule Разбирает табличное размещение на имя модуля, крейт и слот 00–15; это не аппаратный ModuleID.
func parseModule(value string) (name, rack string, slot int, err error) {
	m := modulePattern.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return "", "", 0, fmt.Errorf("неверное имя модуля %q, ожидается A11-00", value)
	}
	slot, _ = strconv.Atoi(m[2])
	if slot > 15 {
		return "", "", 0, fmt.Errorf("слот %s вне диапазона 00…15", m[2])
	}
	return fmt.Sprintf("%s_%02d", m[1], slot), m[1], slot, nil
}

// rackNumber Возвращает числовой порядок крейта Axx для раскладки инвентаря.
func rackNumber(value string) int { n, _ := strconv.Atoi(strings.TrimPrefix(value, "A")); return n }

// reserveTag Формирует имя свободного физического канала из принадлежности ПЛК и размещения.
func reserveTag(fcs, module string, ch int) string { return fmt.Sprintf("_%s_%s_%d", fcs, module, ch) }

// defaultPLCName Применяет подтверждённые имена ПЛК конкретного исходного IO-формата; это адаптация таблицы, не правило генерации.
func defaultPLCName(fcs, cabinet string) string {
	// Project-specific names confirmed against both AI.xlsx and AO.xlsx.
	known := map[string]string{"FCS5:3000_D_SC_B05": "3000_D_SC_B05_1", "FCS6:3000_D_SC_B06": "3000_D_SC_B06_1", "FCS7:3000_D_SC_B07": "3000_D_SC_B07_1", "FCS8:3000_D_SC_B07": "3000_D_SC_B07_2"}
	if name := known[fcs+":"+cabinet]; name != "" {
		return name
	}
	if strings.Contains(fcs, "_SC_") {
		return fcs
	}
	return cabinet
}

// signalTag Разрешает имя сигнала из явного поля, точного справочника или правил IO-формата; неоднозначность возвращает ошибкой.
func signalTag(row sourceRow, redundant bool) (string, bool, error) {
	explicit := row.Explicit
	if redundant && row.ExplicitReserve != "" {
		explicit = row.ExplicitReserve
	}
	if explicit != "" {
		if !identifier.MatchString(explicit) {
			return "", false, fmt.Errorf("неверный SCADA Tag %q", explicit)
		}
		if redundant && row.Type == "AI16H" && row.ExplicitReserve == "" {
			return "", false, fmt.Errorf("для резервного AI укажите SCADA Reserve Tag")
		}
		return explicit, false, nil
	}
	if tag := referenceTag(row, redundant); tag != "" {
		return tag, true, nil
	}
	base := row.TagNo
	if row.Type == "AI16H" {
		base = latinLookalikes(row.Loop)
		m := loopPattern.FindStringSubmatch(base)
		if m == nil || strings.HasPrefix(row.Typno, "PID") || m != nil && strings.Contains(m[2], "C") {
			m = loopPattern.FindStringSubmatch(latinLookalikes(row.LoopNo))
			if m == nil {
				return "", false, fmt.Errorf("не удаётся вывести имя AI из Loop / Loop No; добавьте SCADA Tag")
			}
			base = m[1] + "-" + m[2] + "I-" + m[3]
		} else {
			hasAlarm := false
			for _, value := range row.Alarms {
				if !empty(value) {
					hasAlarm = true
				}
			}
			if !hasAlarm {
				m[2] = strings.TrimSuffix(m[2], "A")
			}
			base = m[1] + "-" + m[2] + "-" + m[3]
		}
		if row.Redundant != "" {
			if redundant {
				base += "_reserve"
			} else {
				base += "_main"
			}
		}
	} else if row.Type == "AOC4H" {
		m := loopPattern.FindStringSubmatch(latinLookalikes(base))
		if m == nil {
			return "", false, fmt.Errorf("неверный Tag No для AO; добавьте SCADA Tag")
		}
		code := map[string]string{"TY": "TV", "PY": "PV", "FY": "FV", "LY": "LV", "HY": "HIC", "XI": "XIC", "ESI": "ES", "WI": "WV", "SY": "SY"}[m[2]]
		if code == "" {
			return "", false, fmt.Errorf("нет правила имени AO для %q; добавьте SCADA Tag", m[2])
		}
		base = m[1] + "-" + code + "-" + m[3]
	}
	tag := "_" + normalizeName(latinLookalikes(base))
	if !identifier.MatchString(tag) {
		return "", false, fmt.Errorf("неверное имя сигнала %q; добавьте SCADA Tag", tag)
	}
	return tag, false, nil
}

// latinLookalikes Заменяет кириллические двойники в обозначениях исходной таблицы перед проверкой имени сигнала.
func latinLookalikes(value string) string {
	return strings.NewReplacer("С", "C", "А", "A", "В", "B", "Е", "E", "К", "K", "М", "M", "Н", "H", "О", "O", "Р", "P", "Т", "T", "Х", "X").Replace(strings.TrimSpace(value))
}

// IO occasionally carries a copied FCS label from another cabinet. A physical
// cabinet/rack has one owner. User confirmed taking that physical inventory as
// authoritative; a tie is rejected rather than inventing another controller.
func rackOwners(sheet Sheet, h map[string]string, headerNumber int) (map[string]string, error) {
	counts := map[string]map[string]int{}
	for _, row := range sheet.Rows {
		if row.Number <= headerNumber {
			continue
		}
		cabinet := normalizeName(row.Cells[h["cabinet"]])
		fcs := strings.TrimSpace(row.Cells[h["fcs"]])
		if !plcIdentifier.MatchString(cabinet) || !plcIdentifier.MatchString(fcs) {
			continue
		}
		for _, column := range []string{"main", "redundant"} {
			_, rack, _, err := parseModule(row.Cells[h[column]])
			if err != nil {
				continue
			}
			key := cabinet + ":" + rack
			if counts[key] == nil {
				counts[key] = map[string]int{}
			}
			counts[key][fcs]++
		}
	}
	owners := map[string]string{}
	for key, candidates := range counts {
		max, total := 0, 0
		for fcs, count := range candidates {
			total += count
			if count > max {
				max = count
				owners[key] = fcs
			}
		}
		if max*2 <= total {
			return nil, fmt.Errorf("IO: крейт %s неоднозначно относится к нескольким FCS; исправьте столбец FCS", key)
		}
	}
	return owners, nil
}
