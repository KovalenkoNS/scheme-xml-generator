// Single-template FBD reservation stage: Applies explicit transport ID overrides for the single document.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/contracts"
)

// prepareIDs Applies explicit transport ID overrides for the single document.
// Positive signed32 bounds are checked before graph objects are created.
func (b *singleBuild) prepareIDs() error {
	if b.request.T11Start != nil {
		b.ids.T11Start = *b.request.T11Start
	}
	if b.request.CardStart != nil {
		b.ids.CardStart = *b.request.CardStart
	}
	if b.request.POUID != nil {
		b.ids.POUID = *b.request.POUID
	}
	if b.ids.T11Start < 1 || b.ids.CardStart < 1 || b.ids.POUID < 1 {
		return fmt.Errorf("начальные ID должны быть положительными")
	}
	if b.ids.POUID > contracts.MaxTransportID {
		return fmt.Errorf("POU ID должен помещаться в signed 32-bit")
	}
	return nil
}
