// Single-template FBD result stage: library-backed output preparation and assembly.
package fbd

import (
	xmlartifact "scheme-xml-generator/internal/generator/artifact"
)

// result exposes the validated XML and exact transport/geometry summary.
// It returns independent result values while retaining source template identity in the signal report.
func (b *singleBuild) result() (xmlartifact.Result, error) {
	return xmlartifact.Result{
		XML: b.data, BaseName: b.preview.BaseName, Warnings: uniqueStrings(b.warnings),
		Summary: xmlartifact.Summary{
			Blocks: len(b.blocks), Links: len(b.links), Graphics: len(b.graphics), Cards: len(b.cards),
			T11First: b.ids.T11Start, T11Last: b.nextT11 - 1, CardFirst: b.ids.CardStart, CardLast: b.ids.CardStart + int64(len(b.cards)) - 1, POUID: b.ids.POUID, POUName: b.pouName, POUGroupID: b.groupID, POUNumber: b.pouNumber,
			POUCount: 1, SignalCount: 1,
			POUs: []xmlartifact.POUSummary{{
				POUID: b.ids.POUID, POUName: b.pouName, POUGroupID: b.groupID, POUNumber: b.pouNumber,
				Blocks: len(b.blocks), Links: len(b.links), Graphics: len(b.graphics), Cards: len(b.cards),
				T11First: b.ids.T11Start, T11Last: b.nextT11 - 1, CardFirst: b.ids.CardStart, CardLast: b.ids.CardStart + int64(len(b.cards)) - 1,
				Signals: []xmlartifact.SignalSummary{{
					TemplateKey: b.ref.Key, BaseName: b.preview.BaseName,
					Blocks: len(b.blocks), Links: len(b.links), Graphics: len(b.graphics), Cards: len(b.cards),
					T11First: b.ids.T11Start, T11Last: b.nextT11 - 1, CardFirst: b.ids.CardStart, CardLast: b.ids.CardStart + int64(len(b.cards)) - 1,
					OffsetX: b.dx, OffsetY: b.dy,
				}},
			}},
		},
	}, nil
}
