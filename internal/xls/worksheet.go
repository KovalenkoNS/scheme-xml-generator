// Строки/ячейки/индексы и оформление одного листа BIFF8.
package xls

import (
	"encoding/binary"

	"math"
)

// worksheet сериализует значения, размеры и объединения одного листа BIFF8.
// Использует индексы SST и стили, строит ROW/ячейки/DBCELL и возвращает поток с конечным EOF.
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
