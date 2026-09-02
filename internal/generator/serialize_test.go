package generator

import (
	"bytes"
	"encoding/xml"
	"testing"
)

func TestSerializeSCADALexicalContractAndIVStates(t *testing.T) {
	empty := ""
	document := outputDocument{
		Common: outputCommon{Version: "29", Project: "", IsCut: "false", IsFFB: "false", ControllerType: "", ControllerID: "0", ResourceID: "0"},
		POUS: outputPOUS{Items: []outputPOU{{
			ID: "1", Name: "AD3_test", IsFBD: "1", GroupID: "0", Enabled: "1", Number: "0", Description: "",
			Params: outputPOUParams{DParams: "3", Height: "2000", Width: "2000", TemplatePage: "0", Background: "16777215", PrintWidth: "0", PrintHeight: "0", PrintPageA4: "8"},
			ISAGraf: outputISAGraf{
				Blocks: outputBlocks{Items: []outputBlock{
					{ObjectType: "37", Info: "ROOT", T11ID: "10", Graphics: outputBlockBounds{}, Params: outputBlockParams{Commented: "false", Initial: &empty}},
					{ObjectType: "34", Info: "FALSE", T11ID: "11", Graphics: outputBlockBounds{}, Params: outputBlockParams{Commented: "true", Initial: nil}},
				}},
				Gotos: outputGotos{}, Links: outputLinks{},
			},
			Graphics: outputGraphics{Items: []outputPrimitive{{SourceT11ID: "12", Params: "line1\nline2"}}},
		}}},
		ISAObjects: outputISAObjects{},
		ISACards:   outputISACards{},
	}

	data, err := serializeSCADA(document)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := append(append([]byte{}, utf8BOM...), []byte(xml.Header)...)
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
	if expanded := expandedEmptyElement(data); expanded != "" {
		t.Fatalf("expanded empty element %s", expanded)
	}
	var decoded outputDocument
	if err := xml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("strict XML round-trip: %v", err)
	}
}
