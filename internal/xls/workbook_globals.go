// Глобальные служебные записи книги BIFF8 и таблицы стилей.
package xls

import (
	"encoding/binary"
)

// workbookGlobals собирает глобальные записи книги BIFF8 до общей таблицы строк.
// Получает имя/оформление, возвращает поток и смещение BOUNDSHEET для последующей записи адреса листа.
func workbookGlobals(name []uint16, formatting Formatting) ([]byte, int) {
	var data []byte
	data = record(data, 0x0809, bof(0x0005))
	data = record(data, 0x00E1, words(0x04B0)) // InterfaceHdr: Unicode
	data = record(data, 0x00C1, words(0))      // MMS
	data = record(data, 0x00E2, nil)           // InterfaceEnd
	access := make([]byte, 112)
	copy(access, []byte{0, 0, 0}) // empty XLUnicodeString
	for i := 3; i < len(access); i++ {
		access[i] = ' '
	}
	data = record(data, 0x005C, access)      // WriteAccess
	data = record(data, 0x0042, words(1200)) // CodePage
	data = record(data, 0x0161, words(0))    // DSF
	data = record(data, 0x013D, words(1))    // RRTabId
	data = record(data, 0x003D, words(0, 0, 0x3FCF, 0x2A4E, 0x0038, 0, 0, 1, 600))
	for _, id := range []uint16{0x0040, 0x008D, 0x0022} {
		data = record(data, id, words(0))
	}
	data = record(data, 0x000E, words(1)) // CalcPrecision
	data = record(data, 0x01B7, words(0)) // RefreshAll
	data = record(data, 0x00DA, words(0)) // BookBool
	for _, font := range formatting.Fonts {
		data = record(data, 0x0031, font)
	}
	for _, format := range formatting.Formats {
		data = record(data, 0x041E, format)
	}
	for _, xf := range formatting.XFs {
		data = record(data, 0x00E0, xf)
	}
	data = record(data, 0x0293, []byte{0, 0x80, 0, 0xFF}) // Normal STYLE
	if len(formatting.Palette) > 0 {
		data = record(data, 0x0092, formatting.Palette)
	}
	data = record(data, 0x0160, words(0)) // UsesELFs
	boundOffset := len(data) + 4
	bound := make([]byte, 8, 8+2*len(name))
	bound[6], bound[7] = byte(len(name)), 1
	for _, u := range name {
		bound = binary.LittleEndian.AppendUint16(bound, u)
	}
	data = record(data, 0x0085, bound)
	data = record(data, 0x008C, words(1, 1)) // Country
	return data, boundOffset
}
