// Проверки оформления BIFF/XLS: стили, геометрия, объединения и отказы при повреждённых профилях.
package xls

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

// Собирает тестовый профиль BIFF: шрифты, стили, числовой формат, геометрию и объединения для проверок оформления XLS.
func exampleFormatting() Formatting {
	fonts, xfs := defaultFormattingTables()
	bold := append([]byte(nil), fonts[0]...)
	binary.LittleEndian.PutUint16(bold[6:], 700)
	fonts = append(fonts, bold)
	header := append([]byte(nil), xfs[15]...)
	binary.LittleEndian.PutUint16(header, 1)
	header[6] = 0x1A                                       // horizontal center, wrap, vertical center
	binary.LittleEndian.PutUint32(header[10:], 0x1111)     // thin borders
	binary.LittleEndian.PutUint32(header[14:], 0x04000000) // solid fill
	binary.LittleEndian.PutUint16(header[18:], 8|(9<<7))
	xfs = append(xfs, header)
	number := append([]byte(nil), xfs[15]...)
	binary.LittleEndian.PutUint16(number[2:], 164)
	xfs = append(xfs, number)
	format := append(words(164, 4), 0)
	format = append(format, "0000"...)
	return Formatting{
		Fonts: fonts, XFs: xfs, Formats: [][]byte{format},
		Palette:   []byte{2, 0, 192, 192, 192, 0, 255, 255, 255, 0},
		CellXFs:   [][]uint16{{16, 16, 17}, {16, 16, 15}},
		DefaultXF: 15, DefaultRowHeight: 300, DefaultColumnWidth: 8,
		RowHeights: map[int]uint16{0: 600, 1: 225},
		Columns:    []ColumnFormat{{First: 0, Last: 1, Width: 20 * 256, XF: 15}, {First: 2, Last: 2, Width: 12 * 256, XF: 17}},
		Merges:     []Range{{FirstRow: 0, LastRow: 1, FirstColumn: 0, LastColumn: 1}},
	}
}

// Сверяет готовые BIFF-записи с профилем оформления: стили, палитра, размеры и значения объединённых ячеек
// сохраняются.
func TestWriteFormattingRetainsStylesGeometryAndMergedValues(t *testing.T) {
	formatting := exampleFormatting()
	rows := [][]Cell{{Text("Группа"), Text("Группа"), Number(12)}, {Text("Группа"), Text("Группа"), {}}}
	file, err := WriteWithFormatting("Styled", rows, formatting)
	if err != nil {
		t.Fatal(err)
	}
	stream := workbookStream(t, file)
	globals := readRecords(t, stream, 0)
	var fonts, formats, xfs [][]byte
	sheetOffset := 0
	paletteFound := false
	for _, rec := range globals {
		switch rec.id {
		case 0x0031:
			fonts = append(fonts, rec.body)
		case 0x041E:
			formats = append(formats, rec.body)
		case 0x00E0:
			xfs = append(xfs, rec.body)
		case 0x0092:
			paletteFound = bytes.Equal(rec.body, formatting.Palette)
		case 0x0085:
			sheetOffset = int(binary.LittleEndian.Uint32(rec.body))
		}
	}
	if !reflect.DeepEqual(fonts, formatting.Fonts) || !reflect.DeepEqual(formats, formatting.Formats) || !reflect.DeepEqual(xfs, formatting.XFs) || !paletteFound {
		t.Fatal("formatting tables were lost or altered")
	}
	texts, count := sharedStrings(t, globals)
	if count != 4 || !reflect.DeepEqual(texts, []string{"Группа"}) {
		t.Fatal("merged cell duplicates removed from SST")
	}
	var columns []ColumnFormat
	var heights []uint16
	labels, mergeCount := 0, 0
	for _, rec := range readRecords(t, stream, sheetOffset) {
		switch rec.id {
		case 0x0055:
			if binary.LittleEndian.Uint16(rec.body) != 8 {
				t.Fatal("default column width lost")
			}
		case 0x0225:
			if binary.LittleEndian.Uint16(rec.body[2:]) != 300 {
				t.Fatal("default row height lost")
			}
		case 0x007D:
			columns = append(columns, ColumnFormat{int(binary.LittleEndian.Uint16(rec.body)), int(binary.LittleEndian.Uint16(rec.body[2:])), binary.LittleEndian.Uint16(rec.body[4:]), binary.LittleEndian.Uint16(rec.body[6:])})
		case 0x0208:
			heights = append(heights, binary.LittleEndian.Uint16(rec.body[6:]))
			if binary.LittleEndian.Uint16(rec.body[12:])&0x40 == 0 {
				t.Fatal("custom row height flag missing")
			}
		case 0x00FD, 0x0203, 0x0201:
			r, c := binary.LittleEndian.Uint16(rec.body), binary.LittleEndian.Uint16(rec.body[2:])
			if binary.LittleEndian.Uint16(rec.body[4:]) != formatting.CellXFs[r][c] {
				t.Fatal("cell XF index lost")
			}
			if rec.id == 0x00FD {
				labels++
			}
		case 0x00E5:
			mergeCount += int(binary.LittleEndian.Uint16(rec.body))
			if !bytes.Equal(rec.body, words(1, 0, 1, 0, 1)) {
				t.Fatal("merged geometry changed")
			}
		}
	}
	if labels != 4 {
		t.Fatal("merged duplicates were removed from cell records")
	}
	if mergeCount != 1 || !reflect.DeepEqual(columns, formatting.Columns) || !reflect.DeepEqual(heights, []uint16{600, 225}) {
		t.Fatal("worksheet geometry was lost")
	}
	checkRowIndex(t, stream, sheetOffset, len(rows))
}

// Проверяет разделение 2055 объединений на MERGECELLS и применение DefaultXF к ячейкам при отсутствии индивидуальных
// стилей.
func TestMergeRecordSplittingAndDefaultCellStyle(t *testing.T) {
	rows := make([][]Cell, 2055)
	f := exampleFormatting()
	f.CellXFs, f.RowHeights, f.Columns = nil, nil, nil
	f.DefaultXF = 16
	f.Merges = nil
	for r := range rows {
		rows[r] = []Cell{Text("A"), Text("B")}
		f.Merges = append(f.Merges, Range{FirstRow: r, LastRow: r, FirstColumn: 0, LastColumn: 1})
	}
	file, err := WriteWithFormatting("Merges", rows, f)
	if err != nil {
		t.Fatal(err)
	}
	stream := workbookStream(t, file)
	for _, global := range readRecords(t, stream, 0) {
		if global.id != 0x0085 {
			continue
		}
		var counts []int
		for _, rec := range readRecords(t, stream, int(binary.LittleEndian.Uint32(global.body))) {
			if rec.id == 0x00E5 {
				counts = append(counts, int(binary.LittleEndian.Uint16(rec.body)))
			}
			if rec.id == 0x00FD && binary.LittleEndian.Uint16(rec.body[4:]) != 16 {
				t.Fatal("DefaultXF ignored")
			}
		}
		if !reflect.DeepEqual(counts, []int{1027, 1027, 1}) {
			t.Fatalf("MERGECELLS counts %v", counts)
		}
	}
}

// Проверяет, что XLS writer сохраняет явно заданный локализованный FORMAT, даже когда его ID относится к встроенным
// форматам.
func TestFormattingAllowsExplicitLocalizedBuiltinFormats(t *testing.T) {
	f := exampleFormatting()
	localized := append(words(5, 1), 1, 0xA4, 0)
	f.Formats = append(f.Formats, localized)
	file, err := WriteWithFormatting("Localized", [][]Cell{{Text("A"), Text("B"), Number(1)}, {Text("C"), Text("D"), {}}}, f)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range readRecords(t, workbookStream(t, file), 0) {
		if rec.id == 0x041E && bytes.Equal(rec.body, localized) {
			found = true
		}
	}
	if !found {
		t.Fatal("localized built-in FORMAT was removed")
	}
}

// Повреждает поля профиля оформления XLS по одному; writer должен отвергать неверные ссылки, записи и диапазоны
// объединений.
func TestFormattingRejectsMalformedRecordsAndRanges(t *testing.T) {
	rows := [][]Cell{{Text("A"), Text("B"), Number(1)}, {Text("C"), Text("D"), {}}}
	cases := map[string]func(*Formatting){
		"missing fonts":               func(f *Formatting) { f.Fonts = nil },
		"missing XFs":                 func(f *Formatting) { f.XFs = nil },
		"short font":                  func(f *Formatting) { f.Fonts[0] = []byte{0} },
		"font name mismatch":          func(f *Formatting) { f.Fonts[0][14] = 100 },
		"short XF":                    func(f *Formatting) { f.XFs[16] = []byte{0} },
		"font index out of range":     func(f *Formatting) { binary.LittleEndian.PutUint16(f.XFs[16], 40) },
		"reserved font index":         func(f *Formatting) { binary.LittleEndian.PutUint16(f.XFs[16], 4) },
		"invalid XF parent":           func(f *Formatting) { binary.LittleEndian.PutUint16(f.XFs[16][4:], 17<<4) },
		"invalid built-in XF":         func(f *Formatting) { f.XFs[0][4] &= ^byte(4) },
		"missing number format":       func(f *Formatting) { f.Formats = nil },
		"invalid number format":       func(f *Formatting) { f.Formats[0] = []byte{1} },
		"invalid number format index": func(f *Formatting) { binary.LittleEndian.PutUint16(f.Formats[0], 500) },
		"empty number format":         func(f *Formatting) { f.Formats[0] = append(words(164, 0), 0) },
		"duplicate number format":     func(f *Formatting) { f.Formats = append(f.Formats, f.Formats[0]) },
		"bad palette":                 func(f *Formatting) { f.Palette = f.Palette[:len(f.Palette)-1] },
		"default style XF":            func(f *Formatting) { f.DefaultXF = 1 },
		"cell style XF":               func(f *Formatting) { f.CellXFs[0][0] = 1 },
		"cell XF missing":             func(f *Formatting) { f.CellXFs[0][0] = 100 },
		"row formatting overflow":     func(f *Formatting) { f.CellXFs = append(f.CellXFs, []uint16{15}) },
		"column formatting overflow":  func(f *Formatting) { f.CellXFs[0] = append(f.CellXFs[0], 15) },
		"row height overflow":         func(f *Formatting) { f.RowHeights[0] = 9000 },
		"negative row":                func(f *Formatting) { f.RowHeights[-1] = 255 },
		"default height overflow":     func(f *Formatting) { f.DefaultRowHeight = 9000 },
		"default width overflow":      func(f *Formatting) { f.DefaultColumnWidth = 256 },
		"column width zero":           func(f *Formatting) { f.Columns[0].Width = 0 },
		"column range overflow":       func(f *Formatting) { f.Columns[0].Last = 256 },
		"column range inverted":       func(f *Formatting) { f.Columns[0].Last = -1 },
		"column style XF":             func(f *Formatting) { f.Columns[0].XF = 1 },
		"overlapping columns":         func(f *Formatting) { f.Columns[1].First = 1 },
		"merge row overflow":          func(f *Formatting) { f.Merges[0].LastRow = 2 },
		"merge column overflow":       func(f *Formatting) { f.Merges[0].LastColumn = 256 },
		"inverted merge":              func(f *Formatting) { f.Merges[0].LastRow = -1 },
		"single-cell merge":           func(f *Formatting) { f.Merges[0] = Range{} },
		"overlapping merges": func(f *Formatting) {
			f.Merges = append(f.Merges, Range{FirstRow: 0, LastRow: 0, FirstColumn: 1, LastColumn: 2})
		},
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			f := exampleFormatting()
			modify(&f)
			if _, err := WriteWithFormatting("Test", rows, f); err == nil {
				t.Fatal("accepted malformed formatting")
			}
		})
	}
}
