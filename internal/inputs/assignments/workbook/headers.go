// Выбор адаптера по реальным заголовкам таблицы, без технологических областей или названия проекта.
package workbook

import (
	"scheme-xml-generator/internal/inputs/assignments/fields"
	"scheme-xml-generator/internal/inputs/assignments/prepared"
	"scheme-xml-generator/internal/inputs/assignments/raw"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
)

const rawIOKind = "raw-IO"

// readHeader распознаёт один из поддержанных форматов по ячейкам шапки книги.
func readHeader(row xlsx.Row) (map[string]string, string, error) {
	columns, found, err := raw.Header(row)
	if err != nil || found {
		return columns, rawIOKind, err
	}
	return prepared.Header(row)
}

// isHeaderRow замечает второй заголовок на листе вместо ошибочного разбора его как назначения.
func isHeaderRow(row xlsx.Row) bool {
	keys := map[string]bool{}
	for _, value := range row.Cells {
		keys[fields.HeaderKey(value)] = true
	}
	return keys["scs"] && (keys["tagno"] || keys["scsai"] || keys["scsdo"])
}
