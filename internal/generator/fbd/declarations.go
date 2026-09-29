// Single-template FBD declarations stage: Builds font/type tables and emitted card records for the document.
package fbd

import (
	"scheme-xml-generator/internal/generator/xmlmodel"
)

// buildDeclarations Builds font/type tables and emitted card records for the document.
// Library metadata is resolved before page assembly and any missing type aborts generation.
func (b *singleBuild) buildDeclarations() error {
	var err error
	b.fonts, b.fontWarnings = buildFonts(b.ref)
	b.warnings = append(b.warnings, b.fontWarnings...)
	b.isaObjects, err = buildISAObjects(b.ref)
	if err != nil {
		return err
	}
	b.cardRecords = make([]xmlmodel.OutputISACard, 0, len(b.cards))
	for _, card := range b.cards {
		size := cleanNumber(card.Source.Size, "0")
		b.cardRecords = append(b.cardRecords, xmlmodel.OutputISACard{ID: card.ID, Info: card.Info, IsRetain: "1", Name: card.Name, Size: size, ClusterPath: b.request.ClusterPath})
	}

	return nil
}
