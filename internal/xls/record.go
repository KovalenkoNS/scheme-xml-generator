// Кодирование заголовков и коротких полей записей BIFF8.
package xls

import (
	"encoding/binary"
)

// record добавляет одну запись BIFF8 при сборке потока Workbook.
// Принимает ID и тело, записывает little-endian заголовок длины и возвращает расширенный буфер.
func record(dst []byte, id uint16, body []byte) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, id)
	dst = binary.LittleEndian.AppendUint16(dst, uint16(len(body)))
	return append(dst, body...)
}

// words кодирует короткие поля служебных записей BIFF8.
// Возвращает последовательность uint16 в little-endian для переданных значений.
func words(values ...uint16) []byte {
	data := make([]byte, 0, 2*len(values))
	for _, value := range values {
		data = binary.LittleEndian.AppendUint16(data, value)
	}
	return data
}

// bof готовит тело BOF-записи начала книги или листа BIFF8.
// Подставляет заданный тип потока и постоянные поля версии экспортируемого формата.
func bof(kind uint16) []byte {
	data := words(0x0600, kind, 0x0DBB, 0x07CC)
	data = binary.LittleEndian.AppendUint32(data, 0x00000009)
	return binary.LittleEndian.AppendUint32(data, 0x00000006)
}
