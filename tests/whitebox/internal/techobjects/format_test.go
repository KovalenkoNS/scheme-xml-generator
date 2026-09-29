// Проверка поставляемого BIFF-профиля оформления таблицы технологических объектов.
package techobjects

import (
	"encoding/binary"
	"reflect"
	"testing"

	"scheme-xml-generator/internal/xls"
)

// Проверяет поставляемый профиль оформления XLS техобъектов: таблицы стилей, размеры и объединения согласованы с
// заголовком.
func TestNativeFormattingProfile(t *testing.T) {
	format, err := nativeFormatting()
	if err != nil {
		t.Fatal(err)
	}
	wantMerges := []xls.Range{
		{FirstRow: 0, LastRow: 1, FirstColumn: 0, LastColumn: 1},
		{FirstRow: 0, LastRow: 1, FirstColumn: 2, LastColumn: 19},
		{FirstRow: 0, LastRow: 1, FirstColumn: 20, LastColumn: 20},
		{FirstRow: 0, LastRow: 1, FirstColumn: 21, LastColumn: 21},
		{FirstRow: 0, LastRow: 1, FirstColumn: 22, LastColumn: 22},
		{FirstRow: 0, LastRow: 1, FirstColumn: 23, LastColumn: 99},
	}
	if !reflect.DeepEqual(format.Merges, wantMerges) {
		t.Fatalf("native merged heading bands changed: %+v", format.Merges)
	}
	if format.DefaultColumnWidth != 8 || format.DefaultRowHeight != 255 || format.DefaultXF != 65 || !reflect.DeepEqual(format.RowHeights, map[int]uint16{0: 255, 1: 255, 2: 600, 3: 225}) {
		t.Fatalf("native dimensions/default styles changed: %+v", format)
	}
	if len(format.CellXFs) != 4 || len(format.XFs) != 67 {
		t.Fatal("missing native cell styles")
	}
	for row, want := range []uint16{66, 66, 63, 64} {
		if len(format.CellXFs[row]) != 100 {
			t.Fatal("missing styles on blank header cells")
		}
		for column, xf := range format.CellXFs[row] {
			if xf != want {
				t.Fatalf("header cell %d/%d: XF=%d, want %d", row, column, xf, want)
			}
		}
	}
	// Native field codes use Arial 8; other headings and data use Arial 10.
	if len(format.Fonts) < 2 || binary.LittleEndian.Uint16(format.Fonts[0]) != 200 || binary.LittleEndian.Uint16(format.Fonts[1]) != 160 || binary.LittleEndian.Uint16(format.XFs[64]) != 1 {
		t.Fatal("native header fonts changed")
	}
	for _, i := range []int{63, 64, 66} {
		xf := format.XFs[i]
		if len(xf) != 20 || xf[6]&7 != 2 || (xf[6]>>4)&7 != 1 || binary.LittleEndian.Uint16(xf[10:]) != 0x2222 {
			t.Fatalf("header XF %d lost center alignment or medium borders", i)
		}
	}
	if format.XFs[63][6]&8 == 0 || format.XFs[64][6]&8 != 0 {
		t.Fatal("captions must wrap and field codes must stay on one line")
	}
	if binary.LittleEndian.Uint32(format.XFs[64][14:])>>26 != 1 || binary.LittleEndian.Uint16(format.XFs[64][18:])&0x7f != 22 {
		t.Fatal("field-code row lost its solid gray fill")
	}
	if binary.LittleEndian.Uint16(format.XFs[65][10:]) != 0x1111 {
		t.Fatal("object rows lost thin borders")
	}
}
