// Проверка значений и сборка полноценной однотабличной книги XLS.
package xls

import (
	"encoding/binary"
	"fmt"
	"math"

	"unicode/utf8"
)

// Write формирует однотабличный XLS из буквальных значений с базовым оформлением.
// Передаёт имя листа и строки в WriteWithFormatting, возвращает байты OLE/BIFF8 либо ошибку ограничений.
func Write(sheetName string, rows [][]Cell) ([]byte, error) {
	return WriteWithFormatting(sheetName, rows, Formatting{})
}

// WriteWithFormatting проверяет и сериализует значения и оформление листа в OLE/BIFF8.
// Возвращает XLS-байты с общей таблицей строк; отвергает неверные данные, ссылки стилей и превышение размера.
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
