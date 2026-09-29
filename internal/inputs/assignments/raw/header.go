// Распознавание исходного IO-листа по Tag No и размещению Main/Redundant_module.
package raw

import (
	"fmt"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
)

// Header проверяет столбцы исходной карты DI/DO, отделяя её от подготовленных выражений.
func Header(row xlsx.Row) (map[string]string, bool, error) {
	columns := map[string]string{}
	duplicates := []string{}
	for column, value := range row.Cells {
		key := fields.HeaderKey(value)
		switch key {
		case "tagno", "scs", "iotype", "mainmodule", "redundantmodule", "channel", "loop", "mainmodule2", "redundantmodule2", "mainchassis", "redundandchassis", "redundantchassis", "mashallingcabinet", "marshallingcabinet", "controllerid", "scsai", "scsdo":
			if key == "marshallingcabinet" {
				key = "mashallingcabinet"
			}
			if key == "redundantchassis" {
				key = "redundandchassis"
			}
			if columns[key] != "" {
				duplicates = append(duplicates, value)
			}
			columns[key] = column
		}
	}
	// Tag No distinguishes the source IO inventory from prepared AI/DO maps.
	found := columns["tagno"] != "" && (columns["scs"] != "" || columns["mainmodule"] != "" || columns["iotype"] != "")
	if !found {
		return nil, false, nil
	}
	if columns["scsai"] != "" || columns["scsdo"] != "" {
		return nil, true, fmt.Errorf("исходная IO-карта (Tag No) и подготовленные SCS AI/SCS DO должны быть на отдельных листах")
	}
	if len(duplicates) != 0 {
		return nil, true, fmt.Errorf("повторный заголовок исходной IO-карты %q", duplicates[0])
	}
	for _, key := range []string{"tagno", "scs", "iotype", "mainmodule", "channel"} {
		if columns[key] == "" {
			return nil, true, fmt.Errorf("исходная IO-карта: не найден обязательный столбец %s", key)
		}
	}
	return columns, true, nil
}
