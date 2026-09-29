// Single-template FBD single stage: library-backed output preparation and assembly.
package fbd

import (
	"scheme-xml-generator/internal/generator/contracts"

	"scheme-xml-generator/internal/library"
)

// Generate orchestrates one library-template export through isolated preparation and rendering stages.
// Each stage owns one result fragment; the public API returns bytes only after XML validation.
func (g Generator) Generate(ref *library.TemplateRef, request contracts.Request, ids contracts.IDRange) (contracts.Result, error) {
	b := &singleBuild{generator: g, ref: ref, request: request, ids: ids}
	for _, step := range []func() error{b.prepareRequest, b.preparePlacement, b.prepareIDs, b.prepareDestination, b.prepareWarnings, b.prepareCards, b.classifyPrimitives, b.orderBlocks, b.buildBlocks, b.buildLinks, b.buildGraphics, b.buildDeclarations, b.buildDocument, b.encodeAndValidate} {
		if err := step(); err != nil {
			return contracts.Result{}, err
		}
	}
	return b.result()
}
