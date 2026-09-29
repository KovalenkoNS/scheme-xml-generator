// SCADA identifier rules validate the specific names and text supplied to generation.
package identifiers

import (
	"fmt"

	"unicode"
)

// ValidateIdentifier Проверяет длину и символы имени объекта перед записью в XML.
// Возвращает предметную ошибку для библиотечных карточек и пользовательских тегов.
func ValidateIdentifier(value string) error {
	if len([]rune(value)) > 160 {
		return fmt.Errorf("имя объекта не должно быть длиннее 160 символов")
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-' {
			continue
		}
		return fmt.Errorf("имя объекта содержит недопустимый символ %q", r)
	}
	return nil
}
