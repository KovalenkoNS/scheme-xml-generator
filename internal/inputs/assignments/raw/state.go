// Состояние чтения исходной IO-карты: повторные позиции, получатели и пропущенные направления.
package raw

import (
	model "scheme-xml-generator/internal/domain/assignments"
)

type Plan = model.Plan
type Group = model.Group
type Module = model.Module
type Channel = model.Channel

type State struct {
	Ignored   int
	positions map[string]int
	owners    map[string]string
}

// NewState создаёт независимые индексы позиций и получателей для одной книги исходных IO.
func NewState() *State {
	return &State{positions: map[string]int{}, owners: map[string]string{}}
}
