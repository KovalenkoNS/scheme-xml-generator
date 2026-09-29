// Single-template FBD single build stage: library-backed output preparation and assembly.
package fbd

import (
	"scheme-xml-generator/internal/generator/contracts"

	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"
)

type singleBuild struct {
	generator                                    Generator
	ref                                          *library.TemplateRef
	request                                      contracts.Request
	ids                                          contracts.IDRange
	preview                                      contracts.NamePreview
	pouName                                      string
	layout                                       library.LayoutBounds
	dx, dy                                       int
	controllerID, resourceID, groupID, pouNumber string
	warnings                                     []string
	cards                                        []*generatedCard
	cardMap                                      map[string]*generatedCard
	primitives                                   []library.Primitive
	blockCount, linkCount, graphicCount          int
	nextT11                                      int64
	idMap                                        map[string]string
	orderedBlocks                                []library.Primitive
	reorderedBlocks                              bool
	blocks                                       []xmlmodel.OutputBlock
	links                                        []xmlmodel.OutputLink
	graphics                                     []xmlmodel.OutputPrimitive
	fonts                                        []xmlmodel.OutputFontStyle
	fontWarnings                                 []string
	isaObjects                                   []xmlmodel.OutputISAObject
	cardRecords                                  []xmlmodel.OutputISACard
	pageWidth, pageHeight                        int
	version                                      string
	fontStyles                                   *xmlmodel.OutputFontStyles
	document                                     xmlmodel.OutputDocument
	data                                         []byte
}
