// Раскладка значений объектов по 100 столбцам импортной XLS-таблицы.
package techobjects

import (
	"scheme-xml-generator/internal/xls"
)

// textRow переносит значения технологического объекта в строку экспортного XLS-формата.
// Возвращает 100 ячеек, сохраняя пустые служебные столбцы; непустые значения записывает как литеральный текст.
func textRow(values []string) []xls.Cell {
	// Preserve the native export's 100 columns, including the unused final
	// heading band and styled blank cells. Its 23 populated columns stay fixed.
	row := make([]xls.Cell, 100)
	for i, value := range values {
		if value != "" {
			row[i] = xls.Text(value)
		}
	}
	return row
}
