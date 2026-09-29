// Проверка строк и управляющих символов исходного IO-листа до создания назначений.
package raw

import (
	"fmt"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strings"
	"unicode"
)

// validateFields проверяет исходные ячейки и номер строки перед предметным разбором DI/DO.
func validateFields(row xlsx.Row, columns map[string]string) error {
	if row.Number < 1 {
		return fmt.Errorf("IO: номер строки источника должен быть положительным")
	}
	for name, column := range columns {
		if strings.ContainsFunc(row.Cells[column], func(r rune) bool { return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' }) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	for _, name := range []string{"tagno", "scs", "iotype", "mainmodule", "redundantmodule", "channel", "mainmodule2", "redundantmodule2", "controllerid"} {
		if strings.ContainsFunc(row.Cells[columns[name]], unicode.IsControl) {
			return fmt.Errorf("недопустимые управляющие символы в поле %s", name)
		}
	}
	return nil
}
