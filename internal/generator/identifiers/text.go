// SCADA identifier rules validate the specific names and text supplied to generation.
package identifiers

import (
	"fmt"
)

// ValidateText Проверяет длину и управляющие символы текста, включаемого в XML.
// Название поля добавляется в ошибку, чтобы указать настройку пользователю.
func ValidateText(value, field string) error {
	if len([]rune(value)) > 500 {
		return fmt.Errorf("%s не должно быть длиннее 500 символов", field)
	}
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 {
			continue
		}
		return fmt.Errorf("%s содержит недопустимый для XML 1.0 управляющий символ U+%04X", field, r)
	}
	return nil
}
