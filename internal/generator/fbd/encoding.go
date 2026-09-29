// Single-template FBD encoding stage: Serializes the completed document using the SCADA lexical codec.
package fbd

import (
	"fmt"

	"scheme-xml-generator/internal/generator/xmlcodec"
)

// encodeAndValidate Serializes the completed document using the SCADA lexical codec.
// The resulting bytes must match primitive, link, card and font expectations before return.
func (b *singleBuild) encodeAndValidate() error {
	var err error
	b.data, err = xmlcodec.SerializeSCADAValue(b.document)
	if err != nil {
		return fmt.Errorf("сериализовать SCADA XML: %w", err)
	}
	if err := validateGeneratedXML(b.data, len(b.primitives), b.blocks, b.links, b.graphics, len(b.fonts), false); err != nil {
		return err
	}

	return nil
}
