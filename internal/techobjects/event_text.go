// Формат записи события SCADA в строке технологического XLS.
package techobjects

import (
	"fmt"

	"strings"
)

// eventText готовит одну запись отекстовки события для таблицы технологических объектов SCADA.
// Нормализует разделители в подписи и возвращает строку с заданной битовой маской без лишних событий.
func eventText(mask int32, label string) string {
	// Colons and newlines are delimiters of the SCADA event table, not escaped
	// text. Keep source descriptions readable without creating extra entries.
	label = strings.Join(strings.Fields(strings.ReplaceAll(label, ":", " — ")), " ")
	return fmt.Sprintf("n%d:%s:не вывод::не вывод", mask, label)
}
