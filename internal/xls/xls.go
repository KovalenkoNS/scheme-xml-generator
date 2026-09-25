// Package xls writes a single-sheet Excel 97-2003 binary workbook (BIFF8).
// It deliberately supports only literal text, finite numbers and blank cells;
// text is never interpreted as a formula or converted to a number.
//
// The file structures follow Microsoft's MS-XLS and MS-CFB specifications:
// https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-xls/
// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cfb/
package xls

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	MaxRows    = 65536
	MaxColumns = 256
	maxRecord  = 8224
	// Bound intermediate allocations as well as the final download size.
	maxWorkbookBytes = 256 << 20
)

type cellKind uint8

const (
	blankCell cellKind = iota
	textCell
	numberCell
)

// Cell is a literal spreadsheet value. Its zero value is an empty cell.
type Cell struct {
	kind   cellKind
	text   string
	number float64
}

// Text preserves s exactly, including leading zeroes and an initial '='.
func Text(s string) Cell { return Cell{kind: textCell, text: s} }

// Number creates a numeric cell. Write rejects NaN and infinities.
func Number(n float64) Cell { return Cell{kind: numberCell, number: n} }

// Write returns a real OLE/BIFF8 .xls file, with one worksheet. No installed
// spreadsheet application is needed. Names are limited to 31 UTF-16 code units,
// cells to 32767 UTF-16 code units, and sheets to 65536 rows by 256 columns.
func Write(sheetName string, rows [][]Cell) ([]byte, error) {
	return WriteWithFormatting(sheetName, rows, Formatting{})
}

// WriteWithFormatting writes literal values together with validated BIFF8
// appearance settings. Merged cells retain all supplied values and styles.
func WriteWithFormatting(sheetName string, rows [][]Cell, formatting Formatting) ([]byte, error) {
	name, err := checkedName(sheetName)
	if err != nil {
		return nil, err
	}
	if len(rows) > MaxRows {
		return nil, fmt.Errorf("xls: %d rows exceeds limit %d", len(rows), MaxRows)
	}
	formatting, err = checkedFormatting(formatting, rows)
	if err != nil {
		return nil, err
	}
	indices := make(map[string]uint32)
	var texts []string
	var textCount uint32
	columns := 0
	// The allowance includes all fixed records, row/index records and SST
	// continuation headers, and is intentionally an upper bound.
	estimated := uint64(8192+len(rows)*36) + formatting.storageBytes()
	if estimated > maxWorkbookBytes {
		return nil, fmt.Errorf("xls: workbook exceeds the 256 MiB size limit")
	}
	for r, row := range rows {
		if len(row) > MaxColumns {
			return nil, fmt.Errorf("xls: row %d has more than %d columns", r+1, MaxColumns)
		}
		columns = max(columns, len(row))
		for c, cell := range row {
			estimated += 18
			switch cell.kind {
			case blankCell:
			case numberCell:
				if math.IsNaN(cell.number) || math.IsInf(cell.number, 0) {
					return nil, fmt.Errorf("xls: non-finite number at row %d, column %d", r+1, c+1)
				}
			case textCell:
				textCount++
				if _, exists := indices[cell.text]; !exists {
					if !utf8.ValidString(cell.text) {
						return nil, fmt.Errorf("xls: invalid UTF-8 at row %d, column %d", r+1, c+1)
					}
					units := utf16Length(cell.text)
					if units > 32767 {
						return nil, fmt.Errorf("xls: text exceeds 32767 characters at row %d, column %d", r+1, c+1)
					}
					estimated += uint64(3 + units*2 + (units*2/maxRecord+1)*5)
					indices[cell.text] = uint32(len(texts))
					texts = append(texts, cell.text)
				}
			default:
				return nil, fmt.Errorf("xls: invalid cell kind at row %d, column %d", r+1, c+1)
			}
			if estimated > maxWorkbookBytes {
				return nil, fmt.Errorf("xls: workbook exceeds the 256 MiB size limit")
			}
		}
	}
	// Empty rows are represented by a formatted blank at column A.
	if len(rows) > 0 {
		columns = max(columns, 1)
	}
	for _, merged := range formatting.Merges {
		columns = max(columns, merged.LastColumn+1)
	}
	globals, boundOffset := workbookGlobals(name, formatting)
	globals = appendSST(globals, texts, textCount)
	globals = record(globals, 0x000A, nil)
	binary.LittleEndian.PutUint32(globals[boundOffset:], uint32(len(globals)))
	sheet := worksheet(rows, indices, columns, len(globals), formatting)
	return compoundFile(append(globals, sheet...)), nil
}

func checkedName(name string) ([]uint16, error) {
	if !utf8.ValidString(name) || name == "" || utf16Length(name) > 31 || strings.ContainsAny(name, "[]:*?/\\\x00\r\n") || strings.HasPrefix(name, "'") || strings.HasSuffix(name, "'") {
		return nil, fmt.Errorf("xls: invalid worksheet name %q", name)
	}
	for _, r := range name {
		if r < 0x20 {
			return nil, fmt.Errorf("xls: invalid worksheet name %q", name)
		}
	}
	return utf16.Encode([]rune(name)), nil
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}

func record(dst []byte, id uint16, body []byte) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, id)
	dst = binary.LittleEndian.AppendUint16(dst, uint16(len(body)))
	return append(dst, body...)
}

func words(values ...uint16) []byte {
	data := make([]byte, 0, 2*len(values))
	for _, value := range values {
		data = binary.LittleEndian.AppendUint16(data, value)
	}
	return data
}

func bof(kind uint16) []byte {
	data := words(0x0600, kind, 0x0DBB, 0x07CC)
	data = binary.LittleEndian.AppendUint32(data, 0x00000009)
	return binary.LittleEndian.AppendUint32(data, 0x00000006)
}

func workbookGlobals(name []uint16, formatting Formatting) ([]byte, int) {
	var data []byte
	data = record(data, 0x0809, bof(0x0005))
	data = record(data, 0x00E1, words(0x04B0)) // InterfaceHdr: Unicode
	data = record(data, 0x00C1, words(0))      // MMS
	data = record(data, 0x00E2, nil)           // InterfaceEnd
	access := make([]byte, 112)
	copy(access, []byte{0, 0, 0}) // empty XLUnicodeString
	for i := 3; i < len(access); i++ {
		access[i] = ' '
	}
	data = record(data, 0x005C, access)      // WriteAccess
	data = record(data, 0x0042, words(1200)) // CodePage
	data = record(data, 0x0161, words(0))    // DSF
	data = record(data, 0x013D, words(1))    // RRTabId
	data = record(data, 0x003D, words(0, 0, 0x3FCF, 0x2A4E, 0x0038, 0, 0, 1, 600))
	for _, id := range []uint16{0x0040, 0x008D, 0x0022} {
		data = record(data, id, words(0))
	}
	data = record(data, 0x000E, words(1)) // CalcPrecision
	data = record(data, 0x01B7, words(0)) // RefreshAll
	data = record(data, 0x00DA, words(0)) // BookBool
	for _, font := range formatting.Fonts {
		data = record(data, 0x0031, font)
	}
	for _, format := range formatting.Formats {
		data = record(data, 0x041E, format)
	}
	for _, xf := range formatting.XFs {
		data = record(data, 0x00E0, xf)
	}
	data = record(data, 0x0293, []byte{0, 0x80, 0, 0xFF}) // Normal STYLE
	if len(formatting.Palette) > 0 {
		data = record(data, 0x0092, formatting.Palette)
	}
	data = record(data, 0x0160, words(0)) // UsesELFs
	boundOffset := len(data) + 4
	bound := make([]byte, 8, 8+2*len(name))
	bound[6], bound[7] = byte(len(name)), 1
	for _, u := range name {
		bound = binary.LittleEndian.AppendUint16(bound, u)
	}
	data = record(data, 0x0085, bound)
	data = record(data, 0x008C, words(1, 1)) // Country
	return data, boundOffset
}

// String headers always stay in one record. A CONTINUE that splits character
// data starts with its own Unicode flag; a CONTINUE between strings does not.
func appendSST(dst []byte, texts []string, total uint32) []byte {
	chunk := binary.LittleEndian.AppendUint32(nil, total)
	chunk = binary.LittleEndian.AppendUint32(chunk, uint32(len(texts)))
	id := uint16(0x00FC)
	flush := func() { dst = record(dst, id, chunk); id = 0x003C; chunk = nil }
	bucketSize := max(len(texts)/128+1, 8)
	ext := words(uint16(bucketSize))
	for i, text := range texts {
		units := utf16.Encode([]rune(text))
		// Keep at least one complete character with its string header.
		if maxRecord-len(chunk) < 5 {
			flush()
		}
		if i%bucketSize == 0 {
			ext = binary.LittleEndian.AppendUint32(ext, uint32(len(dst)+4+len(chunk)))
			ext = binary.LittleEndian.AppendUint16(ext, uint16(4+len(chunk)))
			ext = binary.LittleEndian.AppendUint16(ext, 0)
		}
		chunk = binary.LittleEndian.AppendUint16(chunk, uint16(len(units)))
		chunk = append(chunk, 1)
		for _, unit := range units {
			needed := 2
			// Readers commonly decode each record separately. Keep a UTF-16
			// surrogate pair together instead of splitting its two code units.
			if unit >= 0xD800 && unit <= 0xDBFF {
				needed = 4
			}
			if maxRecord-len(chunk) < needed {
				flush()
				chunk = append(chunk, 1)
			}
			chunk = binary.LittleEndian.AppendUint16(chunk, unit)
		}
	}
	flush()
	return record(dst, 0x00FF, ext)
}

func worksheet(rows [][]Cell, indices map[string]uint32, columns, base int, formatting Formatting) []byte {
	data := record(nil, 0x0809, bof(0x0010))
	blockCount := (len(rows) + 31) / 32
	index := make([]byte, 16+4*blockCount)
	binary.LittleEndian.PutUint32(index[8:], uint32(len(rows)))
	indexOffset := len(data) + 4
	data = record(data, 0x020B, index)
	for _, setting := range [][2]uint16{{0x000D, 1}, {0x000C, 100}, {0x000F, 1}, {0x0011, 0}} {
		data = record(data, setting[0], words(setting[1]))
	}
	data = record(data, 0x0010, binary.LittleEndian.AppendUint64(nil, math.Float64bits(0.001)))
	for _, setting := range [][2]uint16{{0x005F, 1}, {0x002A, 0}, {0x002B, 0}, {0x0082, 1}} {
		data = record(data, setting[0], words(setting[1]))
	}
	data = record(data, 0x0080, words(0, 0, 0, 0)) // Guts
	data = record(data, 0x0225, words(0, formatting.DefaultRowHeight))
	data = record(data, 0x0081, words(0x04C1)) // WsBool
	data = record(data, 0x0014, words(0))      // Header
	data = record(data, 0x0015, words(0))      // Footer
	data = record(data, 0x0083, words(0))
	data = record(data, 0x0084, words(0))
	setup := words(9, 100, 1, 1, 1, 0x0082, 300, 300)
	setup = binary.LittleEndian.AppendUint64(setup, math.Float64bits(0.3))
	setup = binary.LittleEndian.AppendUint64(setup, math.Float64bits(0.3))
	setup = append(setup, 1, 0)
	data = record(data, 0x00A1, setup)
	binary.LittleEndian.PutUint32(data[indexOffset+12:], uint32(base+len(data)))
	data = record(data, 0x0055, words(formatting.DefaultColumnWidth)) // DefColWidth
	for _, column := range formatting.Columns {
		xf := column.XF
		if xf == 0 {
			xf = formatting.DefaultXF
		}
		data = record(data, 0x007D, words(uint16(column.First), uint16(column.Last), column.Width, xf, 2, 0))
	}
	dimensions := make([]byte, 14)
	binary.LittleEndian.PutUint32(dimensions[4:], uint32(len(rows)))
	binary.LittleEndian.PutUint16(dimensions[10:], uint16(columns))
	data = record(data, 0x0200, dimensions)
	for block := 0; block < blockCount; block++ {
		first, end := block*32, min((block+1)*32, len(rows))
		rowStart := len(data)
		for r := first; r < end; r++ {
			height, flags := formatting.DefaultRowHeight, uint16(0x0100)
			if custom, exists := formatting.RowHeights[r]; exists {
				height = custom
				flags |= 0x40
			}
			row := words(uint16(r), 0, uint16(max(len(rows[r]), 1)), height, 0, 0, flags, formatting.DefaultXF)
			data = record(data, 0x0208, row)
		}
		db := make([]byte, 4, 4+2*(end-first))
		previous := rowStart + 20
		for r := first; r < end; r++ {
			db = binary.LittleEndian.AppendUint16(db, uint16(len(data)-previous))
			previous = len(data)
			row := rows[r]
			if len(row) == 0 {
				row = []Cell{{}}
			}
			for c, cell := range row {
				body := words(uint16(r), uint16(c), formatting.cellXF(r, c))
				switch cell.kind {
				case textCell:
					body = binary.LittleEndian.AppendUint32(body, indices[cell.text])
					data = record(data, 0x00FD, body)
				case numberCell:
					body = binary.LittleEndian.AppendUint64(body, math.Float64bits(cell.number))
					data = record(data, 0x0203, body)
				default:
					data = record(data, 0x0201, body)
				}
			}
		}
		binary.LittleEndian.PutUint32(db, uint32(len(data)-rowStart))
		binary.LittleEndian.PutUint32(data[indexOffset+16+4*block:], uint32(base+len(data)))
		data = record(data, 0x00D7, db)
	}
	data = record(data, 0x023E, words(0x06B6, 0, 0, 64, 0, 0, 0, 0, 0))
	// A MERGECELLS payload contains at most 1027 eight-byte ranges.
	for first := 0; first < len(formatting.Merges); first += 1027 {
		end := min(first+1027, len(formatting.Merges))
		body := words(uint16(end - first))
		for _, merged := range formatting.Merges[first:end] {
			body = append(body, words(uint16(merged.FirstRow), uint16(merged.LastRow), uint16(merged.FirstColumn), uint16(merged.LastColumn))...)
		}
		data = record(data, 0x00E5, body)
	}
	return record(data, 0x000A, nil)
}
