// Package ioimport собирает сценарий чтения IO: адаптер формата и подтверждённый профиль назначений имеют отдельных владельцев.
package ioimport

import (
	"scheme-xml-generator/internal/domain/assignments"
	"scheme-xml-generator/internal/generator/moduleprofile"
	input "scheme-xml-generator/internal/inputs/assignments"
)

// Read передаёт XLSX адаптеру, затем применяет профиль к прочитанным сигналам для preview и ST одного контракта.
func Read(data []byte) (*assignments.Plan, error) {
	plan, err := input.Parse(data)
	if err != nil {
		return nil, err
	}
	if err := moduleprofile.ApplySourceAssignments(plan); err != nil {
		return nil, err
	}
	return plan, nil
}
