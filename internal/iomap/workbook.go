// Workbook access used by the IO-list adapter; decoding belongs to inputs/xlsx.
package iomap

import "scheme-xml-generator/internal/inputs/xlsx"

type Sheet = xlsx.Sheet
type Row = xlsx.Row

const MaxWorkbookBytes = xlsx.MaxWorkbookBytes

// ReadWorkbook returns cached Excel values for IO parsing without interpreting controller semantics.
func ReadWorkbook(data []byte) ([]Sheet, error) { return xlsx.ReadWorkbook(data) }
