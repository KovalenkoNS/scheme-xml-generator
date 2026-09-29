// SCADA XML lexical-contract checks independent of any particular FBD/ST/HMI renderer.
package xmlcodec

import (
	"bytes"
	"encoding/xml"
	"scheme-xml-generator/internal/generator/xmlmodel"
	"testing"
)

// TestSerializeSCADALexicalContractAndIVStates checks XML serialization preserves BOM, compact SCADA syntax and the
// distinction between absent and explicitly empty IV.
func TestSerializeSCADALexicalContractAndIVStates(t *testing.T) {
	empty := ""
	document := xmlmodel.OutputDocument{
		Common: xmlmodel.OutputCommon{Version: "29", Project: "", IsCut: "false", IsFFB: "false", ControllerType: "", ControllerID: "0", ResourceID: "0"},
		POUS: xmlmodel.OutputPOUS{Items: []xmlmodel.OutputPOU{{
			ID: "1", Name: "AD3_test", IsFBD: "1", GroupID: "0", Enabled: "1", Number: "0", Description: "",
			Params: xmlmodel.OutputPOUParams{DParams: "3", Height: "2000", Width: "2000", TemplatePage: "0", Background: "16777215", PrintWidth: "0", PrintHeight: "0", PrintPageA4: "8"},
			ISAGraf: xmlmodel.OutputISAGraf{
				Blocks: xmlmodel.OutputBlocks{Items: []xmlmodel.OutputBlock{
					{ObjectType: "37", Info: "ROOT", T11ID: "10", Graphics: xmlmodel.OutputBlockBounds{}, Params: xmlmodel.OutputBlockParams{Commented: "false", Initial: &empty}},
					{ObjectType: "34", Info: "FALSE", T11ID: "11", Graphics: xmlmodel.OutputBlockBounds{}, Params: xmlmodel.OutputBlockParams{Commented: "true", Initial: nil}},
				}},
				Gotos: xmlmodel.OutputGotos{}, Links: xmlmodel.OutputLinks{},
			},
			Graphics: xmlmodel.OutputGraphics{Items: []xmlmodel.OutputPrimitive{{SourceT11ID: "12", Params: "line1\nline2"}}},
		}}},
		ISAObjects: xmlmodel.OutputISAObjects{},
		ISACards:   xmlmodel.OutputISACards{},
	}

	data, err := SerializeSCADAValue(document)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := append(append([]byte{}, Utf8BOM...), []byte(xml.Header)...)
	if !bytes.HasPrefix(data, wantPrefix) {
		t.Fatalf("prefix=% X", data[:min(len(data), len(wantPrefix))])
	}
	for _, wanted := range [][]byte{
		[]byte(`<Common VER="29" Project="" isCut="false" isFFB="false" ControllerTypeName="" ControllerID="0" ResuorceID="0"/>`),
		[]byte(`<Gotos/><Links/>`),
		[]byte(`<IV/>`),
		[]byte("<PARAMS>line1\r\nline2</PARAMS>"),
	} {
		if !bytes.Contains(data, wanted) {
			t.Errorf("serialized output does not contain %q", wanted)
		}
	}
	if bytes.Count(data, []byte("<IV")) != 1 {
		t.Fatalf("IV nodes=%d, want exactly one empty IV and one absent IV", bytes.Count(data, []byte("<IV")))
	}
	if bytes.Contains(data, []byte("<FONTSTYLES")) || bytes.Contains(data, []byte("&#xA;")) || bytes.Contains(data, []byte("\n  <")) {
		t.Fatal("serializer emitted an optional empty section or non-SCADA whitespace form")
	}
	if expanded := ExpandedEmptyElement(data); expanded != "" {
		t.Fatalf("expanded empty element %s", expanded)
	}
	var decoded xmlmodel.OutputDocument
	if err := xml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("strict XML round-trip: %v", err)
	}
}
