// Общая строковая таблица SST/EXTSST с корректными CONTINUE.
package xls

import (
	"encoding/binary"

	"unicode/utf16"
)

// appendSST записывает общую таблицу уникальных строк SST и индекс EXTSST книги.
// Разбивает длинные данные на CONTINUE с Unicode-флагом, сохраняя заголовки строк и surrogate-пары целиком.
func appendSST(dst []byte, texts []string, total uint32) []byte {
	chunk := binary.LittleEndian.AppendUint32(nil, total)
	chunk = binary.LittleEndian.AppendUint32(chunk, uint32(len(texts)))
	id := uint16(0x00FC)
	// Завершает текущий SST-фрагмент и переключает следующий на CONTINUE.
	// Обновляет выходной буфер, ID записи и очищает накопленное тело.
	flush := func() { dst = record(dst, id, chunk); id = 0x003C; chunk = nil }
	bucketSize := max(len(texts)/128+1, 8)
	ext := words(uint16(bucketSize))
	for i, text := range texts {
		units := utf16.Encode([]rune(text))
		// Keep at least one complete character with its string header.
		if maxRecord-len(chunk) < 5 {
			flush()
		}
		if i%bucketSize == 0 {
			ext = binary.LittleEndian.AppendUint32(ext, uint32(len(dst)+4+len(chunk)))
			ext = binary.LittleEndian.AppendUint16(ext, uint16(4+len(chunk)))
			ext = binary.LittleEndian.AppendUint16(ext, 0)
		}
		chunk = binary.LittleEndian.AppendUint16(chunk, uint16(len(units)))
		chunk = append(chunk, 1)
		for _, unit := range units {
			needed := 2
			// Readers commonly decode each record separately. Keep a UTF-16
			// surrogate pair together instead of splitting its two code units.
			if unit >= 0xD800 && unit <= 0xDBFF {
				needed = 4
			}
			if maxRecord-len(chunk) < needed {
				flush()
				chunk = append(chunk, 1)
			}
			chunk = binary.LittleEndian.AppendUint16(chunk, unit)
		}
	}
	flush()
	return record(dst, 0x00FF, ext)
}
