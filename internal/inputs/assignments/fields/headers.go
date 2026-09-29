// Нормализация заголовков и пустых строк входных таблиц; не содержит правил генерации.
package fields

import (
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strings"
	"unicode"
)

// HeaderKey сопоставляет русские/английские заголовки без пробелов и регистра.
func HeaderKey(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(text))
}

// EmptyRow определяет отсутствие входных значений перед разбором строки.
func EmptyRow(row xlsx.Row) bool {
	for _, value := range row.Cells {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
