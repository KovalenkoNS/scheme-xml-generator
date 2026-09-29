// Проверки совместимости HTTP-имён после отделения ПЛК от заголовков исходных таблиц.
package contracts_test

import (
	"encoding/json"
	"scheme-xml-generator/internal/domain/analogoutput"
	"scheme-xml-generator/internal/domain/inventory"
	"scheme-xml-generator/internal/httpapi/output"
	"scheme-xml-generator/internal/techobjects"
	"testing"
)

// TestControllerNamesRetainExistingJSONContracts проверяет прежние ключи preview и выходных HTTP DTO.
// Нейтральные Go-поля должны передавать те же значения без новых ControllerName/SourceController ключей.
func TestControllerNamesRetainExistingJSONContracts(t *testing.T) {
	cases := []struct {
		name  string
		value any
		key   string
		want  any
	}{
		{"AO count", analogoutput.Plan{ControllerCount: 2}, "fcsCount", float64(2)},
		{"AO controller", analogoutput.Group{ControllerName: "PLC_A"}, "fcs", "PLC_A"},
		{"IO source", inventory.Controller{SourceController: "PLC_B"}, "sourceFcs", "PLC_B"},
		{"objects preview", techobjects.ControllerPreview{SourceController: "PLC_C"}, "sourceFcs", "PLC_C"},
		{"XML result", output.GeneratedControllerFile{ControllerName: "PLC_D"}, "fcs", "PLC_D"},
		{"XLS result", output.TechObjectsGeneratedFile{ControllerName: "PLC_E"}, "fcs", "PLC_E"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if fields[test.key] != test.want {
				t.Fatalf("legacy JSON key %q = %#v, want %#v", test.key, fields[test.key], test.want)
			}
			for _, internalName := range []string{"ControllerName", "ControllerCount", "SourceController", "controllerName", "controllerCount", "sourceController"} {
				if _, exists := fields[internalName]; exists {
					t.Fatalf("internal name %q leaked into existing HTTP contract", internalName)
				}
			}
		})
	}
}
