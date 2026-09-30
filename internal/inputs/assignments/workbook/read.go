// Чтение книги и последовательное делегирование строк конкретным адаптерам входного формата.
package workbook

import (
	"fmt"
	model "scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/inputs/assignments/assembly"
	"scheme-xml-generator/internal/inputs/assignments/fields"
	"scheme-xml-generator/internal/inputs/assignments/prepared"
	"scheme-xml-generator/internal/inputs/assignments/raw"
	xlsx "scheme-xml-generator/internal/inputs/xlsx"
	"strings"
)

type Plan = model.Plan
type Group = model.Group

// Parse читает безопасный XLSX-контейнер и передаёт ячейки обработке таблиц назначений.
func Parse(data []byte) (*Plan, error) {
	sheets, err := xlsx.ReadWorkbook(data)
	if err != nil {
		return nil, err
	}
	return ParseSheets(sheets)
}

// ParseSheets выбирает формат листов и накапливает общие назначения ПЛК через адаптеры.
// Ограничения числа строк/модулей защищают вход; название технологической области не участвует.
func ParseSheets(sheets []xlsx.Sheet) (*Plan, error) {
	plan := &Plan{Groups: []Group{}, Warnings: []string{}}
	modules := map[string]*assembly.Module{}
	aiOwners := map[string]string{}
	rawState := raw.NewState()
	controllerNames := map[string]string{}
	rows, tables := 0, 0
	for _, sheet := range sheets {
		rawState.Sheet = sheet.Name
		var columns map[string]string
		kind, headerRow := "", 0
		for _, row := range sheet.Rows {
			if row.Number > 30 {
				break
			}
			candidate, candidateKind, err := readHeader(row)
			if err != nil {
				return nil, fmt.Errorf("%s, строка %d: %w", sheet.Name, row.Number, err)
			}
			if candidateKind != "" {
				columns, kind, headerRow = candidate, candidateKind, row.Number
				break
			}
		}
		if columns == nil {
			continue
		}
		tables++
		for _, row := range sheet.Rows {
			if row.Number <= headerRow || fields.EmptyRow(row) {
				continue
			}
			rows++
			if rows > 50000 {
				return nil, fmt.Errorf("Назначения: допустимо не более 50000 строк назначений")
			}
			if isHeaderRow(row) {
				return nil, fmt.Errorf("%s, строка %d: несколько таблиц или повторный заголовок на одном листе; разместите таблицы на отдельных листах", sheet.Name, row.Number)
			}
			var err error
			if kind == rawIOKind {
				_, err = rawState.ParseRow(row, columns, plan)
			} else {
				err = prepared.ParseRow(row, columns, kind, modules, aiOwners, plan)
			}
			if err != nil {
				return nil, fmt.Errorf("%s, строка %d: %w", sheet.Name, row.Number, err)
			}
			scs := strings.TrimSpace(row.Cells[columns["scs"]])
			if fields.ControllerNamePattern.MatchString(scs) && controllerNames[strings.ToUpper(scs)] == "" {
				controllerNames[strings.ToUpper(scs)] = scs
			}
		}
	}
	if tables == 0 || len(modules) == 0 && (plan.Source == nil || len(plan.Source.Records) == 0) {
		return nil, fmt.Errorf("Назначения: не найдена таблица назначений с SCS AI, SCS DO или исходными DI/DO/AI (Tag No, SCS, I/O Type, Main_module, Channel)")
	}
	if len(modules) > 4096 {
		return nil, fmt.Errorf("Назначения: допустимо не более 4096 модулей")
	}
	if err := prepared.ValidateComplete(modules, controllerNames); err != nil {
		return nil, err
	}
	return finish(modules, controllerNames, plan)
}
