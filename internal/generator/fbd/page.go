// Single-template FBD page stage: Assembles one POU page from the completed graph and destination context.
package fbd

import (
	"scheme-xml-generator/internal/generator/xmlmodel"
	"scheme-xml-generator/internal/library"

	"strconv"
)

// buildDocument Assembles one POU page from the completed graph and destination context.
// Page dimensions include content bounds and configured margins; shared declarations are attached once.
func (b *singleBuild) buildDocument() error {
	b.pageWidth = max(b.generator.Config.Page.Width, max(library.Int(b.ref.Template.Width, 0), b.layout.MaxX)+b.dx+b.generator.Config.Page.MarginRight)
	b.pageHeight = max(b.generator.Config.Page.Height, max(library.Int(b.ref.Template.Height, 0), b.layout.MaxY)+b.dy+b.generator.Config.Page.MarginBottom)
	b.version = b.generator.Config.Common.Version
	if b.version == "" {
		b.version = b.ref.Library.Version
	}
	b.fontStyles = nil
	if len(b.fonts) > 0 {
		b.fontStyles = &xmlmodel.OutputFontStyles{Items: b.fonts}
	}
	b.document = xmlmodel.OutputDocument{
		Common: xmlmodel.OutputCommon{Version: b.version, Project: b.generator.Config.Common.Project, IsCut: "false", IsFFB: "false", ControllerType: b.generator.Config.Common.ControllerType, ControllerID: b.controllerID, ResourceID: b.resourceID},
		POUS: xmlmodel.OutputPOUS{Items: []xmlmodel.OutputPOU{{
			ID: strconv.FormatInt(b.ids.POUID, 10), Name: b.pouName, IsFBD: "1", GroupID: b.groupID, Enabled: "1", Number: b.pouNumber, Description: "",
			Params:   xmlmodel.OutputPOUParams{DParams: b.generator.Config.Page.DParams, Height: strconv.Itoa(b.pageHeight), Width: strconv.Itoa(b.pageWidth), TemplatePage: "0", Background: b.generator.Config.Page.Background, PrintWidth: "0", PrintHeight: "0", PrintPageA4: "8"},
			ISAGraf:  xmlmodel.OutputISAGraf{Blocks: xmlmodel.OutputBlocks{Items: b.blocks}, Gotos: xmlmodel.OutputGotos{}, Links: xmlmodel.OutputLinks{Items: b.links}},
			Graphics: xmlmodel.OutputGraphics{Items: b.graphics},
		}}},
		FontStyles: b.fontStyles, ISAObjects: xmlmodel.OutputISAObjects{Items: b.isaObjects}, ISACards: xmlmodel.OutputISACards{Items: b.cardRecords},
	}

	return nil
}
