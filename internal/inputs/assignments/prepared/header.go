// Распознавание подготовленных таблиц со столбцами SCS AI/SCS DO; это адаптер входного формата.
package prepared

import (
	"fmt"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
)

// Header проверяет только заголовки поддержанного формата подготовленных назначений.
// Возвращает карту столбцов и направление IO, не выбирает шаблон генерации.
func Header(row xlsx.Row) (map[string]string, string, error) {
	columns := map[string]string{}
	for column, value := range row.Cells {
		key := fields.HeaderKey(value)
		switch key {
		case "loop", "scs", "mashallingcabinet", "marshallingcabinet", "module", "channel", "scsai", "scsdo", "mainmodule", "redundantmodule", "iotype", "типобъекта", "шаблон", "controllerid", "марка":
			if key == "marshallingcabinet" {
				key = "mashallingcabinet"
			}
			if columns[key] != "" {
				return nil, "", fmt.Errorf("повторный заголовок %q", value)
			}
			columns[key] = column
		}
	}
	kind := ""
	if columns["scsai"] != "" {
		kind = "AI"
	}
	if columns["scsdo"] != "" {
		if kind != "" {
			return nil, "", fmt.Errorf("столбцы SCS AI и SCS DO должны быть на отдельных листах")
		}
		kind = "DO"
	}
	if kind == "" {
		if columns["scs"] != "" && (columns["module"] != "" || columns["channel"] != "" || columns["марка"] != "") {
			return nil, "", fmt.Errorf("в таблице назначений отсутствует заголовок SCS AI или SCS DO")
		}
		return nil, "", nil
	}
	required := []string{"loop", "scs", "mashallingcabinet", "module", "channel", "mainmodule", "redundantmodule", "iotype", "шаблон", "марка"}
	if kind == "AI" {
		required = append(required, "типобъекта")
	}
	for _, key := range required {
		if columns[key] == "" {
			return nil, "", fmt.Errorf("не найден обязательный столбец %s", key)
		}
	}
	return columns, kind, nil
}
