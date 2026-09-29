// Независимый разбор готового OLE/BIFF проверяет значения, индексы, ограничения и целостность выпуска XLS.
package xls

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

// The test reader follows directory/FAT links rather than assuming where the
// writer puts streams. It also checks allocation ownership and chain lengths.
func workbookStream(t *testing.T, file []byte) []byte {
	t.Helper()
	if len(file)%512 != 0 || !bytes.Equal(file[:8], []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		t.Fatal("invalid compound-file header")
	}
	u32 := binary.LittleEndian.Uint32
	sectorCount := len(file)/512 - 1
	sector := func(id uint32) []byte {
		if uint64(id) >= uint64(sectorCount) {
			t.Fatalf("sector %d out of bounds", id)
		}
		return file[(int(id)+1)*512 : (int(id)+2)*512]
	}
	var fats []uint32
	for i := 76; i < 512; i += 4 {
		if id := u32(file[i:]); id != 0xFFFFFFFF {
			fats = append(fats, id)
		}
	}
	var difats []uint32
	next := u32(file[68:])
	for n := uint32(0); n < u32(file[72:]); n++ {
		difats = append(difats, next)
		data := sector(next)
		for i := 0; i < 508; i += 4 {
			if id := u32(data[i:]); id != 0xFFFFFFFF {
				fats = append(fats, id)
			}
		}
		next = u32(data[508:])
	}
	if next != 0xFFFFFFFE || len(fats) != int(u32(file[44:])) {
		t.Fatal("incorrect DIFAT chain")
	}
	var fat []uint32
	for _, id := range fats {
		data := sector(id)
		for i := 0; i < 512; i += 4 {
			fat = append(fat, u32(data[i:]))
		}
	}
	owned := make(map[uint32]bool)
	for _, group := range []struct {
		ids    []uint32
		marker uint32
	}{{fats, 0xFFFFFFFD}, {difats, 0xFFFFFFFC}} {
		for _, id := range group.ids {
			if owned[id] || fat[id] != group.marker {
				t.Fatal("invalid allocation-table ownership")
			}
			owned[id] = true
		}
	}
	readChain := func(id uint32) []byte {
		var result []byte
		for id != 0xFFFFFFFE {
			if owned[id] || len(result)/512 >= sectorCount {
				t.Fatal("cyclic or overlapping sector chain")
			}
			owned[id] = true
			result = append(result, sector(id)...)
			id = fat[id]
		}
		return result
	}
	directory := readChain(u32(file[48:]))
	if directory[66] != 5 || u32(directory[76:]) != 1 {
		t.Fatal("invalid root directory")
	}
	entry := directory[128:256]
	var name []uint16
	for i := 0; i < int(binary.LittleEndian.Uint16(entry[64:]))-2; i += 2 {
		name = append(name, binary.LittleEndian.Uint16(entry[i:]))
	}
	if string(utf16.Decode(name)) != "Workbook" || entry[66] != 2 {
		t.Fatal("missing Workbook stream")
	}
	size := binary.LittleEndian.Uint64(entry[120:])
	if size < 4096 {
		t.Fatal("regular stream below mini-stream cutoff")
	}
	stream := readChain(u32(entry[116:]))
	if (size+511)/512 != uint64(len(stream)/512) {
		t.Fatal("stream size and chain disagree")
	}
	if len(owned) != sectorCount {
		t.Fatal("unaccounted allocated sector")
	}
	return stream[:size]
}

type biffRecord struct {
	id     uint16
	offset int
	body   []byte
}

// Разбирает BIFF-записи готового Workbook; останавливает тест при повреждённой длине или отсутствии EOF.
func readRecords(t *testing.T, data []byte, offset int) []biffRecord {
	t.Helper()
	var records []biffRecord
	for offset < len(data) {
		if offset+4 > len(data) {
			t.Fatal("truncated BIFF header")
		}
		id, size := binary.LittleEndian.Uint16(data[offset:]), int(binary.LittleEndian.Uint16(data[offset+2:]))
		if size > 8224 || offset+4+size > len(data) {
			t.Fatalf("invalid BIFF record length %d at %d", size, offset)
		}
		records = append(records, biffRecord{id, offset, data[offset+4 : offset+4+size]})
		offset += 4 + size
		if id == 0x000A {
			return records
		}
	}
	t.Fatal("missing BIFF EOF")
	return nil
}

// Восстанавливает строки SST/CONTINUE из готового XLS, включая UTF-16; возвращает значения и число ссылок ячеек.
func sharedStrings(t *testing.T, records []biffRecord) ([]string, uint32) {
	t.Helper()
	var chunks [][]byte
	for _, rec := range records {
		if rec.id == 0x00FC {
			chunks = append(chunks, rec.body)
		} else if len(chunks) > 0 && rec.id == 0x003C {
			chunks = append(chunks, rec.body)
		}
	}
	if len(chunks) == 0 || len(chunks[0]) < 8 {
		t.Fatal("missing SST")
	}
	total, count := binary.LittleEndian.Uint32(chunks[0]), binary.LittleEndian.Uint32(chunks[0][4:])
	chunk, pos := 0, 8
	var result []string
	for n := uint32(0); n < count; n++ {
		if pos == len(chunks[chunk]) {
			chunk++
			pos = 0
		}
		if chunk >= len(chunks) || pos+3 > len(chunks[chunk]) {
			t.Fatal("split or missing string header")
		}
		length := int(binary.LittleEndian.Uint16(chunks[chunk][pos:]))
		wide := chunks[chunk][pos+2]
		pos += 3
		units := make([]uint16, 0, length)
		for len(units) < length {
			if pos == len(chunks[chunk]) {
				chunk++
				pos = 1
				if chunk >= len(chunks) || len(chunks[chunk]) == 0 {
					t.Fatal("truncated continuation")
				}
				wide = chunks[chunk][0]
			}
			if wide != 1 || pos+2 > len(chunks[chunk]) {
				t.Fatal("invalid Unicode continuation boundary")
			}
			unit := binary.LittleEndian.Uint16(chunks[chunk][pos:])
			if unit >= 0xD800 && unit <= 0xDBFF && pos+4 > len(chunks[chunk]) {
				t.Fatal("surrogate pair split across records")
			}
			units = append(units, unit)
			pos += 2
		}
		result = append(result, string(utf16.Decode(units)))
	}
	if chunk != len(chunks)-1 || pos != len(chunks[chunk]) {
		t.Fatal("unused bytes in SST")
	}
	return result, total
}

// Проверяет XLS writer на тексте, числах, пустых ячейках и Unicode; текст с '=' должен остаться строкой, без
// превращения в формулу.
func TestWritePreservesValuesAndTypes(t *testing.T) {
	rows := [][]Cell{{Text("Имя"), Text("00127"), Number(12.5), {}, Text("=1+1")}, {Text("ПЛК №1 🚀"), Text("00127"), Number(0), Text("")}}
	file, err := Write("Объекты ПЛК", rows)
	if err != nil {
		t.Fatal(err)
	}
	stream := workbookStream(t, file)
	globals := readRecords(t, stream, 0)
	texts, total := sharedStrings(t, globals)
	wantTexts := []string{"Имя", "00127", "=1+1", "ПЛК №1 🚀", ""}
	if !reflect.DeepEqual(texts, wantTexts) || total != 6 {
		t.Fatalf("SST = %q, count %d", texts, total)
	}
	sheetStart := 0
	for _, rec := range globals {
		if rec.id == 0x0085 {
			sheetStart = int(binary.LittleEndian.Uint32(rec.body))
			units := make([]uint16, rec.body[6])
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(rec.body[8+2*i:])
			}
			if string(utf16.Decode(units)) != "Объекты ПЛК" {
				t.Fatal("worksheet name corrupted")
			}
		}
	}
	sheet := readRecords(t, stream, sheetStart)
	cells := 0
	for _, rec := range sheet {
		if rec.id != 0x00FD && rec.id != 0x0203 && rec.id != 0x0201 {
			continue
		}
		r, c := int(binary.LittleEndian.Uint16(rec.body)), int(binary.LittleEndian.Uint16(rec.body[2:]))
		cell := rows[r][c]
		cells++
		switch cell.kind {
		case textCell:
			if rec.id != 0x00FD || texts[binary.LittleEndian.Uint32(rec.body[6:])] != cell.text {
				t.Fatalf("text changed at %d,%d", r, c)
			}
		case numberCell:
			if rec.id != 0x0203 || math.Float64frombits(binary.LittleEndian.Uint64(rec.body[6:])) != cell.number {
				t.Fatalf("number changed at %d,%d", r, c)
			}
		case blankCell:
			if rec.id != 0x0201 {
				t.Fatal("blank converted to data")
			}
		}
	}
	if cells != 9 {
		t.Fatalf("got %d cells", cells)
	}
}

// Проверяет BIFF SST на длинных, повторных и суррогатных строках; контролирует CONTINUE и индекс строк без потерь
// значений.
func TestSSTContinuationAndIndex(t *testing.T) {
	var rows [][]Cell
	var want []string
	// Exercise split character data, exact boundaries, empty text, repeated
	// values, surrogate pairs, and a maximum-length Excel string.
	for _, size := range []int{4105, 4106, 4107, 4110, 4111, 4112, 8224, 32767, 0, 1} {
		value := strings.Repeat("Я", size)
		want = append(want, value)
		rows = append(rows, []Cell{Text(value)})
	}
	value := strings.Repeat("🚀", 16383)
	want = append(want, value)
	rows = append(rows, []Cell{Text(value)})
	initialRows := len(rows)
	for i := 0; i < 2000; i++ {
		value := fmt.Sprintf("ПЛК-%04d_Текст 🚀", i)
		want = append(want, value)
		rows = append(rows, []Cell{Text(value), Text(value)})
	}
	file, err := Write("TEST", rows)
	if err != nil {
		t.Fatal(err)
	}
	stream := workbookStream(t, file)
	globals := readRecords(t, stream, 0)
	got, total := sharedStrings(t, globals)
	if !reflect.DeepEqual(got, want) || total != uint32(2*len(rows)-initialRows) {
		t.Fatal("SST continuation lost or duplicated text")
	}
	for _, rec := range globals {
		if rec.id == 0x00FF {
			step := int(binary.LittleEndian.Uint16(rec.body))
			for i, pos := 0, 2; pos < len(rec.body); i, pos = i+1, pos+8 {
				start := int(binary.LittleEndian.Uint32(rec.body[pos:]))
				offset := int(binary.LittleEndian.Uint16(rec.body[pos+4:]))
				id := binary.LittleEndian.Uint16(stream[start-offset:])
				if id != 0x00FC && id != 0x003C {
					t.Fatal("ExtSST points outside string record")
				}
				if int(binary.LittleEndian.Uint16(stream[start:])) != utf16Length(want[i*step]) {
					t.Fatal("ExtSST points to wrong string")
				}
			}
		}
		if rec.id == 0x0085 {
			checkRowIndex(t, stream, int(binary.LittleEndian.Uint32(rec.body)), len(rows))
		}
	}
}

// Сверяет INDEX/DBCELL готового BIFF с фактическими смещениями строк и ячеек; принимает поток, начало листа и число
// строк.
func checkRowIndex(t *testing.T, stream []byte, sheetStart, rows int) {
	t.Helper()
	records := readRecords(t, stream, sheetStart)
	byOffset := make(map[int]biffRecord)
	var index []byte
	for _, rec := range records {
		byOffset[rec.offset] = rec
		if rec.id == 0x020B {
			index = rec.body
		}
	}
	if len(index) != 16+4*((rows+31)/32) {
		t.Fatal("incorrect row-block index length")
	}
	if byOffset[int(binary.LittleEndian.Uint32(index[12:]))].id != 0x0055 {
		t.Fatal("Index points outside DefColWidth")
	}
	for pos := 16; pos < len(index); pos += 4 {
		db := byOffset[int(binary.LittleEndian.Uint32(index[pos:]))]
		if db.id != 0x00D7 {
			t.Fatal("Index points outside DBCell")
		}
		rowOffset := db.offset - int(binary.LittleEndian.Uint32(db.body))
		if byOffset[rowOffset].id != 0x0208 {
			t.Fatal("DBCell points outside Row")
		}
		cellOffset := rowOffset + 20
		for p := 4; p < len(db.body); p += 2 {
			cellOffset += int(binary.LittleEndian.Uint16(db.body[p:]))
			cell := byOffset[cellOffset]
			if cell.id != 0x00FD && cell.id != 0x0203 && cell.id != 0x0201 {
				t.Fatal("DBCell points outside cell record")
			}
			if binary.LittleEndian.Uint16(cell.body) != binary.LittleEndian.Uint16(byOffset[rowOffset].body)+uint16((p-4)/2) {
				t.Fatal("DBCell points to wrong row")
			}
		}
	}
}

// Проверяет OLE writer на потоках до 20 МБ: содержимое восстанавливается, а крупный файл содержит необходимую цепочку
// DIFAT.
func TestCompoundMultipleAllocationTables(t *testing.T) {
	for _, size := range []int{200, 4096, 65000, 8 << 20, 20 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i * 31)
			}
			file := compoundFile(data)
			got := workbookStream(t, file)
			if !bytes.Equal(got[:len(data)], data) {
				t.Fatal("compound stream data corrupted")
			}
			if size > 7<<20 && binary.LittleEndian.Uint32(file[72:]) == 0 {
				t.Fatal("large file has no DIFAT")
			}
		})
	}
}

// Проверяет XLS writer на пустых листах и допустимых предельных размерах; индекс строк должен ссылаться на реальные
// записи.
func TestWriteSheetLimitsAndEmptyRows(t *testing.T) {
	for _, rows := range [][][]Cell{nil, {{}}, {{Text("")}}, make([][]Cell, 65536), {make([]Cell, 256)}} {
		file, err := Write("Sheet", rows)
		if err != nil {
			t.Fatal(err)
		}
		stream := workbookStream(t, file)
		for _, rec := range readRecords(t, stream, 0) {
			if rec.id == 0x0085 {
				checkRowIndex(t, stream, int(binary.LittleEndian.Uint32(rec.body)), len(rows))
			}
		}
	}
}

// Подаёт XLS writer недопустимые имена, размеры, UTF-8 и нечисловые числа; ожидает отказ до создания некорректной
// книги.
func TestWriteRejectsInvalidInput(t *testing.T) {
	for _, name := range []string{"", strings.Repeat("x", 32), "bad/name", "bad\\name", "'name", "name'", "x\x00", "x\x03", "\xff"} {
		if _, err := Write(name, nil); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	for _, rows := range [][][]Cell{make([][]Cell, 65537), {make([]Cell, 257)}, {{Text(strings.Repeat("x", 32768))}}, {{Text(strings.Repeat("🚀", 16384))}}, {{Text("\xff")}}, {{Number(math.NaN())}}, {{Number(math.Inf(1))}}, {{Number(math.Inf(-1))}}} {
		if _, err := Write("Sheet", rows); err == nil {
			t.Error("accepted invalid cell data")
		}
	}
}
