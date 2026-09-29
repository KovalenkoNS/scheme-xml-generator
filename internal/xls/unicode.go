// Подсчёт UTF-16 длины для ограничений строк XLS.
package xls

// utf16Length считает размер строк XLS в единицах UTF-16 для ограничений BIFF8.
// Возвращает две единицы для символов вне BMP, не создавая промежуточный массив.
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
