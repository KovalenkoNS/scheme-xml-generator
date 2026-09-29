// SCADA identifier rules validate the specific names and text supplied to generation.
package identifiers

import (
	"fmt"

	"unicode"
)

// ValidatePOUName Проверяет обязательное имя программы SCADA перед построением POU.
// Контролирует первый символ, длину и допустимый набор остальных символов.
func ValidatePOUName(value string) error {
	if value == "" {
		return fmt.Errorf("имя программного модуля POU не задано")
	}
	if len([]rune(value)) > 160 {
		return fmt.Errorf("имя программного модуля POU не должно быть длиннее 160 символов")
	}
	for index, r := range []rune(value) {
		if index == 0 {
			if unicode.IsLetter(r) || r == '_' {
				continue
			}
			return fmt.Errorf("имя программного модуля POU должно начинаться с буквы или подчёркивания")
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return fmt.Errorf("имя программного модуля POU содержит недопустимый символ %q", r)
	}
	return nil
}
