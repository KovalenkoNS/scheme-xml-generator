// Потоковое чтение нестандартных C0 в экспорте библиотеки без изменения исходного файла.
package library

import (
	"fmt"

	"io"

	"sort"
	"strings"
)

// xmlControlFilter keeps vendor library exports readable without modifying
// the source file. Some SCADA exports contain isolated C0 bytes that XML 1.0
// forbids. Replacing only those bytes with a space preserves element framing
// and the surrounding human-readable parameter value.
type xmlControlFilter struct {
	reader io.Reader
	counts map[byte]int
}

// Read фильтрует поток экспортированной библиотеки перед XML-декодером.
// Заменяет только запрещённые XML 1.0 C0-байты пробелом в буфере и считает их, не изменяя файл.
func (f *xmlControlFilter) Read(buffer []byte) (int, error) {
	count, err := f.reader.Read(buffer)
	for index := 0; index < count; index++ {
		value := buffer[index]
		if value < 0x20 && value != '\t' && value != '\n' && value != '\r' {
			f.counts[value]++
			buffer[index] = ' '
		}
	}
	return count, err
}

// warnings подготавливает отчёт об управляющих байтах, встреченных при чтении библиотеки.
// Возвращает отсортированные коды/количество замен и явное указание, что исходный XML не изменён.
func (f *xmlControlFilter) warnings() []string {
	if len(f.counts) == 0 {
		return []string{}
	}
	codes := make([]int, 0, len(f.counts))
	total := 0
	for value, count := range f.counts {
		codes = append(codes, int(value))
		total += count
	}
	sort.Ints(codes)
	labels := make([]string, 0, len(codes))
	for _, code := range codes {
		labels = append(labels, fmt.Sprintf("U+%04X×%d", code, f.counts[byte(code)]))
	}
	return []string{fmt.Sprintf("При чтении заменены пробелами недопустимые XML 1.0 управляющие символы: %s (всего %d). Исходный файл не изменён.", strings.Join(labels, ", "), total)}
}
