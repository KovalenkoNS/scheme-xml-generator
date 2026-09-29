// Справочник подтверждённых имён конкретного IO-источника; не участвует в выборе XML-шаблонов.
package iomap

import (
	_ "embed"
	"encoding/json"
)

// The commissioning exports contain corrected engineering names which cannot
// always be inferred from Loop (e.g. renamed area or removed alarm suffix).
// A match includes the complete naming inputs AND physical location: changing
// the IO row cannot accidentally reuse a stale signal from this calibration.
//
//go:embed reference_tags.json
var referenceJSON []byte

var referenceTags = func() map[string]string {
	var values map[string]string
	if err := json.Unmarshal(referenceJSON, &values); err != nil {
		panic(err)
	}
	return values
}()

// referenceKey Строит точный ключ строки и размещения для справочника инженерных имён, сохраняя исходный FCS.
func referenceKey(row sourceRow, redundant bool) string {
	if row.OriginalFCS != "" {
		row.FCS = row.OriginalFCS
	}
	data, _ := json.Marshal([]any{row.FCS, row.Cabinet, row.Type, row.Main, row.Redundant, row.Channel, row.Loop, row.LoopNo, row.TagNo, row.Typno, row.Alarms, redundant})
	return string(data)
}

// referenceTag Возвращает подтверждённый тег только при полном совпадении исходных полей и физического размещения.
func referenceTag(row sourceRow, redundant bool) string {
	return referenceTags[referenceKey(row, redundant)]
}
