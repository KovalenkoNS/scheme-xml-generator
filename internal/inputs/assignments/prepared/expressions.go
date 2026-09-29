// Грамматика существующего формата Measurement/Quality в столбцах подготовленной Excel-карты.
package prepared

import (
	"regexp"
	"scheme-xml-generator/internal/inputs/assignments/fields"
)

var (
	aiValue   = regexp.MustCompile(`^(` + fields.IdentifierText + `)\.Xin\s*:=\s*_IO_I\*(` + fields.PhysicalModuleText + `)\*_AI16H_([0-9]{1,2})_VAL\.Measurement\s*;\s*$`)
	aiQuality = regexp.MustCompile(`^(` + fields.IdentifierText + `)\.Xs\s*:=\s*QUAL_STAT\(\s*_IO_I\*(` + fields.PhysicalModuleText + `)\*_AI16H_([0-9]{1,2})_VAL\.Quality\s*\)\s*;\s*$`)
	doValue   = regexp.MustCompile(`^_IO_Q\*(` + fields.PhysicalModuleText + `)\*_DO32P_([0-9]{1,2})_VAL\.Measurement\s*:=\s*(` + fields.IdentifierText + `)(?:\.(` + fields.IdentifierText + `))?\s*;\s*$`)
)
