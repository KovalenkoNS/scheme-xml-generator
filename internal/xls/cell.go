// Буквальные текстовые, числовые и пустые ячейки XLS, без формул.
package xls

type cellKind uint8

const (
	blankCell cellKind = iota
	textCell
	numberCell
)

// Cell is a literal spreadsheet value. Its zero value is an empty cell.
type Cell struct {
	kind   cellKind
	text   string
	number float64
}

// Text создаёт текстовую ячейку для XLS-сериализатора технологических объектов.
// Сохраняет ведущие нули и начальный знак = как буквальный текст, не формулу.
func Text(s string) Cell { return Cell{kind: textCell, text: s} }

// Number создаёт числовую ячейку для XLS-сериализатора.
// Сохраняет float64; окончательная проверка при Write отклоняет NaN и бесконечность.
func Number(n float64) Cell { return Cell{kind: numberCell, number: n} }
