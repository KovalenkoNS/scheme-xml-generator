// Исторические проверки назначений соединяют реальные адаптеры и профиль; производственный input остаётся независимым от генерации.
package assignments

import (
	"scheme-xml-generator/internal/generator/moduleprofile"
	"scheme-xml-generator/internal/iomap"
)

// parseAssignmentPlan передаёт исходную книгу адаптеру и применяет существующий профиль для проверок SCADA-назначений.
func parseAssignmentPlan(data []byte) (*Plan, error) {
	plan, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if err := moduleprofile.ApplySourceAssignments(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// parseAssignmentSheets сохраняет исторические сценарии ячеек через адаптер и профиль без копирования их алгоритмов.
func parseAssignmentSheets(sheets []iomap.Sheet) (*Plan, error) {
	plan, err := ParseSheets(sheets)
	if err != nil {
		return nil, err
	}
	if err := moduleprofile.ApplySourceAssignments(plan); err != nil {
		return nil, err
	}
	return plan, nil
}
