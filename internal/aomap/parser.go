// Package aomap reads the tab-separated AO assignment maps exported from Excel.
package aomap

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"regexp"
	"scheme-xml-generator/internal/domain/analogoutput"
	"scheme-xml-generator/internal/domain/hardware"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type Plan = analogoutput.Plan
type Group = analogoutput.Group
type Module = analogoutput.Module
type Channel = analogoutput.Channel

var (
	modulePattern     = regexp.MustCompile(`^A([0-9]{1,6})[-_]([0-9]{1,6})$`)
	fcsPattern        = regexp.MustCompile(`^[A-Za-z0-9_]{1,100}$`)
	numberPattern     = `[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`
	assignmentPattern = regexp.MustCompile(`(?i)^_IO_QU\*(A[0-9]{1,6}[-_][0-9]{1,6})\*_([0-9]+)\.ValueDINT\s*:=\s*REAL_TO_DINT\(\s*([A-Za-z_][A-Za-z0-9_]{0,159})\.OUT\s*,\s*(` + numberPattern + `)\s*,\s*(` + numberPattern + `)\s*\)\s*;\s*$`)
)

type parsedModule struct {
	module Module
	prefix string
	seen   map[int]string
}

// Parse reads UTF-8, BOM-marked UTF-16, or Windows-1251 TSV. RowCount includes
// duplicate source rows; UniqueTagCount excludes reserves and is scoped by FCS.
// DuplicateCount counts repeated object calls, not repeated source rows.
// Generated reserves have SourceRow == 0. An invalid nonempty row always produces
// an error.
func Parse(data []byte) (*Plan, error) {
	if len(data) > 16*1024*1024 {
		return nil, fmt.Errorf("карта AO превышает 16 МиБ")
	}
	decoded, encodingWarning, err := decodeText(data)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(decoded))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать заголовок TSV: %w", err)
	}
	columns, err := readHeader(header)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Groups: []Group{}, Warnings: []string{}}
	if encodingWarning != "" {
		plan.Warnings = append(plan.Warnings, encodingWarning)
	}
	modules := map[string]*parsedModule{}
	groups := map[string]*Group{}
	for {
		fields, readErr := r.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("ошибка TSV: %w", readErr)
		}
		row, _ := r.FieldPos(0)
		if strings.TrimSpace(strings.Join(fields, "")) == "" {
			continue
		}
		if len(fields) != len(header) {
			return nil, rowError(row, "ожидалось %d столбцов, получено %d", len(header), len(fields))
		}
		plan.RowCount++
		if plan.RowCount > 50000 {
			return nil, rowError(row, "допустимо не более 50000 строк")
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
			if strings.ContainsFunc(fields[i], unicode.IsControl) {
				return nil, rowError(row, "управляющие символы внутри полей не поддерживаются")
			}
		}
		field := func(name string) string { return fields[columns[name]] }
		fcs := field("fcs")
		if !fcsPattern.MatchString(fcs) {
			return nil, rowError(row, "недопустимое имя FCS %q", fcs)
		}
		name, prefix, err := normalizeModule(field("module"))
		if err != nil {
			return nil, rowError(row, "%v", err)
		}
		main, mainPrefix, err := normalizeModule(field("main_module"))
		if err != nil {
			return nil, rowError(row, "Main_module: %v", err)
		}
		redundant := ""
		if field("redundant_module") != "" {
			var redundantPrefix string
			redundant, redundantPrefix, err = normalizeModule(field("redundant_module"))
			if err != nil || redundantPrefix != prefix {
				return nil, rowError(row, "Redundant_module должен относиться к группе %s", prefix)
			}
		}
		if mainPrefix != prefix || (name != main && name != redundant) || redundant == main {
			return nil, rowError(row, "Module %s не согласован с парой Main_module/Redundant_module в группе %s", name, prefix)
		}
		ioType := strings.ToUpper(field("i/o type"))
		if ioType != "AO" && ioType != "AOR" && ioType != "AOR(I)" {
			return nil, rowError(row, "тип ввода/вывода %q не поддерживается; ожидается AO, AOR или AOR(I)", ioType)
		}
		if (ioType == "AO" && redundant != "") || (ioType != "AO" && redundant == "") {
			return nil, rowError(row, "тип %s не согласован с Redundant_module", ioType)
		}
		if field("object_type") != "AN_v1" {
			return nil, rowError(row, "тип объекта %q не поддерживается; ожидается AN_v1", field("object_type"))
		}
		channel, err := strconv.Atoi(field("channel"))
		if err != nil || channel < 0 || channel >= hardware.AOC4HChannels {
			return nil, rowError(row, "канал %q вне диапазона 0–%d", field("channel"), hardware.AOC4HChannels-1)
		}
		match := assignmentPattern.FindStringSubmatch(field("dcs ao"))
		if match == nil {
			return nil, rowError(row, "DCS AO: ожидается _IO_QU*МОДУЛЬ*_канал.ValueDINT := REAL_TO_DINT(ТЕГ.OUT, min, max);")
		}
		statementModule, _, _ := normalizeModule(match[1])
		statementChannel, channelErr := strconv.Atoi(match[2])
		if statementModule != name || channelErr != nil || statementChannel != channel {
			return nil, rowError(row, "модуль/канал в DCS AO не совпадает с Module/Channel (%s, %d)", name, channel)
		}
		minimum, minErr := strconv.ParseFloat(match[4], 64)
		maximum, maxErr := strconv.ParseFloat(match[5], 64)
		if minErr != nil || maxErr != nil || math.IsInf(minimum, 0) || math.IsInf(maximum, 0) || minimum >= maximum {
			return nil, rowError(row, "REAL_TO_DINT: min и max должны быть конечными числами, min < max")
		}
		groupKey := fcs + ":" + prefix
		moduleKey := fcs + ":" + name
		metadata := Module{Name: name, MainModule: main, RedundantModule: redundant, IOType: ioType, ObjectType: "AN_v1", MarshallingCabinet: field("cabinet"), SourceRow: row}
		entry := modules[moduleKey]
		if entry == nil {
			metadata.Channels = make([]Channel, hardware.AOC4HChannels)
			for n := range metadata.Channels {
				metadata.Channels[n] = Channel{Channel: n, Tag: reserveTag(fcs, name, n), Reserve: true, Min: "0.0", Max: "100.0"}
			}
			entry = &parsedModule{module: metadata, prefix: prefix, seen: map[int]string{}}
			modules[moduleKey] = entry
		} else if entry.module.MainModule != main || entry.module.RedundantModule != redundant || entry.module.IOType != ioType || entry.module.MarshallingCabinet != metadata.MarshallingCabinet {
			return nil, rowError(row, "настройки модуля %s противоречат строке %d", name, entry.module.SourceRow)
		}
		fingerprint := strings.Join(fields, "\x00")
		if prior, ok := entry.seen[channel]; ok {
			if prior != fingerprint {
				return nil, rowError(row, "повторное назначение %s/%s, канал %d противоречит строке %d", fcs, name, channel, entry.module.Channels[channel].SourceRow)
			}
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("Строка %d: точный повтор строки %d (%s/%s, канал %d) пропущен.", row, entry.module.Channels[channel].SourceRow, fcs, name, channel))
			continue
		}
		entry.seen[channel] = fingerprint
		entry.module.Channels[channel] = Channel{Channel: channel, SourceRow: row, Tag: match[3], Reserve: strings.EqualFold(match[3], reserveTag(fcs, name, channel)), Min: match[4], Max: match[5]}
		if groups[groupKey] == nil {
			groups[groupKey] = &Group{Key: groupKey, ControllerName: fcs, Prefix: prefix, POUName: "AO_" + prefix, Modules: []Module{}}
		}
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("карта AO не содержит назначений каналов")
	}
	uniqueTags := map[string]struct{}{}
	for key, entry := range modules {
		fcs := strings.SplitN(key, ":", 2)[0]
		for _, partner := range []string{entry.module.MainModule, entry.module.RedundantModule} {
			if partner == "" {
				continue
			}
			if paired := modules[fcs+":"+partner]; paired != nil && (paired.module.MainModule != entry.module.MainModule || paired.module.RedundantModule != entry.module.RedundantModule || paired.module.IOType != entry.module.IOType) {
				return nil, rowError(entry.module.SourceRow, "пара модулей %s/%s противоречит строке %d", entry.module.MainModule, entry.module.RedundantModule, paired.module.SourceRow)
			}
		}
		group := groups[fcs+":"+entry.prefix]
		group.Modules = append(group.Modules, entry.module)
		for _, channel := range entry.module.Channels {
			plan.ChannelCount++
			if channel.Reserve {
				plan.ReserveCount++
			} else {
				uniqueTags[fcs+":"+strings.ToUpper(channel.Tag)] = struct{}{}
			}
		}
	}
	for _, group := range groups {
		sort.Slice(group.Modules, func(i, j int) bool { return moduleNumber(group.Modules[i].Name) < moduleNumber(group.Modules[j].Name) })
		plan.Groups = append(plan.Groups, *group)
	}
	sort.Slice(plan.Groups, func(i, j int) bool {
		if plan.Groups[i].ControllerName != plan.Groups[j].ControllerName {
			return plan.Groups[i].ControllerName < plan.Groups[j].ControllerName
		}
		iPrefix, _ := strconv.Atoi(strings.TrimPrefix(plan.Groups[i].Prefix, "A"))
		jPrefix, _ := strconv.Atoi(strings.TrimPrefix(plan.Groups[j].Prefix, "A"))
		return iPrefix < jPrefix
	})
	plan.ModuleCount = len(modules)
	plan.GroupCount = len(plan.Groups)
	controllerNames := map[string]bool{}
	for _, group := range plan.Groups {
		controllerNames[group.ControllerName] = true
	}
	plan.ControllerCount = len(controllerNames)
	plan.UniqueTagCount = len(uniqueTags)
	return ResolveDuplicates(plan), nil
}

// SplitByController передаёт разобранную AO-карту предметной модели для разделения по ПЛК.
// Возвращает независимые планы, сохраняя назначения исходного текстового формата.
func SplitByController(plan *Plan) []*Plan { return analogoutput.SplitByController(plan) }

// ResolveDuplicates delegates repeated-tag ownership to the format-independent AO model.
func ResolveDuplicates(plan *Plan) *Plan { return analogoutput.ResolveDuplicates(plan) }

// readHeader Проверяет столбцы экспортированной AO-таблицы и возвращает их позиции для чтения строк.
func readHeader(header []string) (map[string]int, error) {
	columns := map[string]int{}
	for i, name := range header {
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "mashalling_cabinet", "marshalling_cabinet":
			name = "cabinet"
		case "тип объекта", "object_type", "object type":
			name = "object_type"
		case "fcs", "module", "channel", "dcs ao", "main_module", "redundant_module", "i/o type":
		default:
			return nil, fmt.Errorf("строка 1: неизвестный столбец %q", header[i])
		}
		if _, exists := columns[name]; exists {
			return nil, fmt.Errorf("строка 1: столбец %q указан дважды", header[i])
		}
		columns[name] = i
	}
	for _, name := range []string{"fcs", "cabinet", "module", "channel", "dcs ao", "main_module", "redundant_module", "i/o type", "object_type"} {
		if _, exists := columns[name]; !exists {
			return nil, fmt.Errorf("строка 1: отсутствует обязательный столбец %q; файл должен быть разделён табуляцией", name)
		}
	}
	return columns, nil
}

// normalizeModule Приводит имя модуля из AO-таблицы к форме Axx_nn и извлекает группу; аппаратный ID не назначает.
func normalizeModule(value string) (name, prefix string, err error) {
	match := modulePattern.FindStringSubmatch(strings.ToUpper(value))
	if match == nil {
		return "", "", fmt.Errorf("недопустимый модуль %q; ожидается A11-00 или A11_00", value)
	}
	group, _ := strconv.Atoi(match[1])
	number, _ := strconv.Atoi(match[2])
	prefix = fmt.Sprintf("A%d", group)
	return fmt.Sprintf("%s_%02d", prefix, number), prefix, nil
}

// moduleNumber Извлекает номер нормализованного AO-модуля для упорядочения строк исходной карты.
func moduleNumber(name string) int {
	_, suffix, _ := strings.Cut(name, "_")
	number, _ := strconv.Atoi(suffix)
	return number
}

// reserveTag Создаёт имя свободного AO-канала из ПЛК, модуля и номера, сохраняя его место в карте.
func reserveTag(fcs, module string, channel int) string {
	return fmt.Sprintf("_%s_%s_%d", fcs, module, channel)
}

// rowError Добавляет номер исходной строки к ошибке AO-парсера для исправления таблицы пользователем.
func rowError(row int, format string, args ...any) error {
	return fmt.Errorf("строка %d: %s", row, fmt.Sprintf(format, args...))
}

// decodeText Декодирует байты AO-таблицы из UTF-8/UTF-16/Windows-1251, возвращая текст и предупреждение о кодировке.
func decodeText(data []byte) (string, string, error) {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
		if !utf8.Valid(data) {
			return "", "", fmt.Errorf("файл с UTF-8 BOM содержит недопустимые байты UTF-8")
		}
		return string(data), "", nil
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		var order binary.ByteOrder = binary.LittleEndian
		if data[0] == 0xfe {
			order = binary.BigEndian
		}
		data = data[2:]
		if len(data)%2 != 0 {
			return "", "", fmt.Errorf("некорректная длина UTF-16 файла")
		}
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = order.Uint16(data[i*2:])
		}
		for i := 0; i < len(units); i++ {
			if units[i] >= 0xd800 && units[i] <= 0xdbff {
				if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
					return "", "", fmt.Errorf("некорректная последовательность UTF-16")
				}
				i++
			} else if units[i] >= 0xdc00 && units[i] <= 0xdfff {
				return "", "", fmt.Errorf("некорректная последовательность UTF-16")
			}
		}
		return string(utf16.Decode(units)), "", nil
	}
	if utf8.Valid(data) {
		return string(data), "", nil
	}
	// Windows-1251's upper Cyrillic range is contiguous; only 0x80–0xbf
	// needs a lookup table. Byte 0x98 is undefined in that encoding.
	table := []rune("ЂЃ‚ѓ„…†‡€‰Љ‹ЊЌЋЏђ‘’“”•–—\ufffd™љ›њќћџ\u00a0ЎўЈ¤Ґ¦§Ё©Є«¬\u00ad®Ї°±Ііґµ¶·ё№є»јЅѕї")
	var result strings.Builder
	for _, b := range data {
		switch {
		case b < 0x80:
			result.WriteByte(b)
		case b == 0x98:
			return "", "", fmt.Errorf("кодировка файла не распознана; сохраните TSV как UTF-8 или UTF-16 с BOM")
		case b < 0xc0:
			result.WriteRune(table[int(b)-0x80])
		default:
			result.WriteRune('А' + rune(b-0xc0))
		}
	}
	return result.String(), "Файл прочитан как Windows-1251; рекомендуется UTF-8 или UTF-16 с BOM.", nil
}
