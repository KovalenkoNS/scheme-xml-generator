package xls

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// Formatting describes the appearance of one worksheet. Fonts, XFs, Formats
// and Palette are BIFF8 record payloads (without their four-byte headers). This
// permits faithful reuse of an existing workbook's styles without translating
// font, alignment, border, fill and number-format flags through another model.
// All indices use the original BIFF tables, including the unused font index 4.
// Fonts and XFs must be provided together, as complete tables; omitting both
// selects the built-in Arial/General styles. The writer validates record sizes,
// table references and ranges before writing any bytes.
type Formatting struct {
	Fonts   [][]byte
	XFs     [][]byte
	Formats [][]byte
	Palette []byte

	// CellXFs overrides the default for supplied cells, using zero-based row
	// and column indices. Missing entries use DefaultXF. Zero is a style XF,
	// not a cell XF, and is therefore not a valid explicit CellXFs entry.
	CellXFs [][]uint16
	// Zero selects cell XF 15, the mandatory BIFF8 default cell format.
	DefaultXF uint16
	// Row heights are in twips. Zero DefaultRowHeight selects 255 twips.
	RowHeights       map[int]uint16
	DefaultRowHeight uint16
	// The default width is in characters; zero selects 10 characters.
	DefaultColumnWidth uint16
	Columns            []ColumnFormat
	Merges             []Range
}

// ColumnFormat applies a width and a default cell format to an inclusive,
// zero-based column range. Width uses 1/256 of the Normal font's digit width.
// XF zero selects Formatting.DefaultXF. Ranges must not overlap.
type ColumnFormat struct {
	First, Last int
	Width       uint16
	XF          uint16
}

// Range is an inclusive, zero-based rectangle. Merging changes only display:
// the writer retains the values and styles of every cell, including duplicates
// hidden under the top-left cell. Ranges must not overlap.
type Range struct {
	FirstRow, LastRow       int
	FirstColumn, LastColumn int
}

func checkedFormatting(input Formatting, rows [][]Cell) (Formatting, error) {
	f := input
	if len(f.Fonts) == 0 && len(f.XFs) == 0 {
		f.Fonts, f.XFs = defaultFormattingTables()
	} else if len(f.Fonts) == 0 || len(f.XFs) == 0 {
		return f, fmt.Errorf("xls: Fonts and XFs must be supplied together")
	}
	if len(f.Fonts) > 1023 || len(f.XFs) < 16 || len(f.XFs) > 4094 {
		return f, fmt.Errorf("xls: invalid formatting-table size")
	}
	for i, font := range f.Fonts {
		if len(font) < 16 || font[14] == 0 || !validBIFFString(font[16:], int(font[14]), font[15]) {
			return f, fmt.Errorf("xls: invalid FONT record %d", i)
		}
	}
	formats := make(map[uint16]bool)
	for i, format := range f.Formats {
		if len(format) < 5 || len(format) > maxRecord || !validBIFFString(format[5:], int(binary.LittleEndian.Uint16(format[2:])), format[4]) {
			return f, fmt.Errorf("xls: invalid FORMAT record %d", i)
		}
		length := binary.LittleEndian.Uint16(format[2:])
		if length == 0 || length > 255 {
			return f, fmt.Errorf("xls: invalid FORMAT string length %d", length)
		}
		id := binary.LittleEndian.Uint16(format)
		// Native Excel files may explicitly define localized built-in
		// formats (for example currency formats 5 through 8).
		validID := (id >= 5 && id <= 8) || (id >= 23 && id <= 26) || (id >= 41 && id <= 44) || (id >= 63 && id <= 66) || (id >= 164 && id <= 392)
		if !validID || formats[id] {
			return f, fmt.Errorf("xls: invalid or duplicate FORMAT index %d", id)
		}
		formats[id] = true
	}
	if len(f.Palette) > 0 {
		if len(f.Palette) < 2 || binary.LittleEndian.Uint16(f.Palette) > 56 || len(f.Palette) != 2+4*int(binary.LittleEndian.Uint16(f.Palette)) {
			return f, fmt.Errorf("xls: invalid PALETTE record")
		}
	}
	for i, xf := range f.XFs {
		if len(xf) != 20 {
			return f, fmt.Errorf("xls: invalid XF record %d: expected 20 bytes", i)
		}
		font := int(binary.LittleEndian.Uint16(xf))
		if font == 4 {
			return f, fmt.Errorf("xls: XF %d references reserved font 4", i)
		}
		if font > 4 {
			font--
		}
		if font >= len(f.Fonts) {
			return f, fmt.Errorf("xls: XF %d references missing font", i)
		}
		format := binary.LittleEndian.Uint16(xf[2:])
		if format >= 164 && !formats[format] {
			return f, fmt.Errorf("xls: XF %d references missing FORMAT %d", i, format)
		}
		flags := binary.LittleEndian.Uint16(xf[4:])
		style := flags&4 != 0
		if (i < 15 && !style) || (i == 15 && style) {
			return f, fmt.Errorf("xls: invalid built-in XF %d", i)
		}
		parent := int(flags >> 4)
		if !style && (parent >= len(f.XFs) || len(f.XFs[parent]) != 20 || f.XFs[parent][4]&4 == 0) {
			return f, fmt.Errorf("xls: XF %d references missing parent style", i)
		}
	}
	if f.DefaultXF == 0 {
		f.DefaultXF = 15
	}
	validCellXF := func(index uint16) bool { return int(index) < len(f.XFs) && f.XFs[index][4]&4 == 0 }
	if !validCellXF(f.DefaultXF) {
		return f, fmt.Errorf("xls: invalid default cell XF %d", f.DefaultXF)
	}
	if len(f.CellXFs) > len(rows) {
		return f, fmt.Errorf("xls: cell formatting exceeds worksheet rows")
	}
	for r, xfs := range f.CellXFs {
		if len(xfs) > max(len(rows[r]), 1) {
			return f, fmt.Errorf("xls: cell formatting exceeds row %d", r+1)
		}
		for c, index := range xfs {
			if !validCellXF(index) {
				return f, fmt.Errorf("xls: invalid cell XF at row %d, column %d", r+1, c+1)
			}
		}
	}
	if f.DefaultRowHeight == 0 {
		f.DefaultRowHeight = 255
	}
	if f.DefaultRowHeight > 8179 {
		return f, fmt.Errorf("xls: invalid default row height")
	}
	for r, height := range f.RowHeights {
		if r < 0 || r >= len(rows) || height < 2 || height > 8192 {
			return f, fmt.Errorf("xls: invalid row height at row %d", r+1)
		}
	}
	if f.DefaultColumnWidth == 0 {
		f.DefaultColumnWidth = 10
	}
	if f.DefaultColumnWidth > 255 {
		return f, fmt.Errorf("xls: invalid default column width")
	}
	var usedColumns [MaxColumns]bool
	for _, column := range f.Columns {
		if column.First < 0 || column.Last < column.First || column.Last >= MaxColumns || column.Width == 0 || (column.XF != 0 && !validCellXF(column.XF)) {
			return f, fmt.Errorf("xls: invalid column formatting range")
		}
		for c := column.First; c <= column.Last; c++ {
			if usedColumns[c] {
				return f, fmt.Errorf("xls: overlapping column formatting ranges")
			}
			usedColumns[c] = true
		}
	}
	if len(f.Merges) > MaxRows {
		return f, fmt.Errorf("xls: too many merged ranges")
	}
	merges := append([]Range(nil), f.Merges...)
	for _, merged := range merges {
		if merged.FirstRow < 0 || merged.LastRow < merged.FirstRow || merged.LastRow >= len(rows) || merged.FirstColumn < 0 || merged.LastColumn < merged.FirstColumn || merged.LastColumn >= MaxColumns || (merged.FirstRow == merged.LastRow && merged.FirstColumn == merged.LastColumn) {
			return f, fmt.Errorf("xls: invalid merged range")
		}
	}
	sort.Slice(merges, func(i, j int) bool { return merges[i].FirstRow < merges[j].FirstRow })
	var active []Range
	for _, merged := range merges {
		retained := active[:0]
		for _, prior := range active {
			if prior.LastRow < merged.FirstRow {
				continue
			}
			if prior.FirstColumn <= merged.LastColumn && merged.FirstColumn <= prior.LastColumn {
				return f, fmt.Errorf("xls: overlapping merged ranges")
			}
			retained = append(retained, prior)
		}
		active = append(retained, merged)
	}
	return f, nil
}

func validBIFFString(data []byte, count int, flag byte) bool {
	if flag > 1 {
		return false
	}
	if flag == 1 {
		count *= 2
	}
	return len(data) == count
}

func defaultFormattingTables() ([][]byte, [][]byte) {
	font := words(200, 0, 0x7FFF, 400, 0)
	font = append(font, 0, 0, 0, 0, 5, 0)
	font = append(font, "Arial"...)
	xfs := make([][]byte, 16)
	for i := range xfs {
		xf := make([]byte, 20)
		if i < 15 {
			binary.LittleEndian.PutUint16(xf[4:], 0xFFF5)
		} else {
			binary.LittleEndian.PutUint16(xf[4:], 1)
		}
		xf[6] = 0x20
		binary.LittleEndian.PutUint16(xf[18:], 0x20C0)
		xfs[i] = xf
	}
	return [][]byte{font}, xfs
}

func (f Formatting) cellXF(row, column int) uint16 {
	if row < len(f.CellXFs) && column < len(f.CellXFs[row]) {
		return f.CellXFs[row][column]
	}
	return f.DefaultXF
}

func (f Formatting) storageBytes() uint64 {
	size := uint64(len(f.Palette) + len(f.Columns)*16 + len(f.Merges)*12)
	for _, records := range [][][]byte{f.Fonts, f.Formats, f.XFs} {
		for _, body := range records {
			size += uint64(len(body) + 4)
		}
	}
	return size
}
