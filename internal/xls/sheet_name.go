// Ограничения и UTF-16 представление имени листа BIFF8.
package xls

import (
	"fmt"

	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// checkedName проверяет имя листа перед записью заголовка BIFF8.
// Возвращает UTF-16 кодовые единицы имени до 31 единицы либо ошибку пустого/недопустимого имени.
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
