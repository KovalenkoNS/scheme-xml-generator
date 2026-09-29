// Package aomap builds AO table inputs for ST and HMI tests through the real input parser.
package aomap

import (
	"fmt"
	"strings"
	"testing"

	input "scheme-xml-generator/internal/aomap"
)

// Header is the source table schema accepted by the AO assignment adapter.
const Header = "FCS\tMashalling_cabinet\tModule\tChannel\tDCS AO\tMain_module\tRedundant_module\tI/O Type\tТип объекта\n"

// Row creates one AO source assignment; production parsing still validates its fields.
func Row(controllerName, module string, channel int, tag string) string {
	return fmt.Sprintf("%s\tCAB-1\t%s\t%d\t_IO_QU*%s*_%d.ValueDINT := REAL_TO_DINT(%s.OUT, 0.0, 100.0);\t%s\t\tAO\tAN_v1\n", controllerName, module, channel, module, channel, tag, module)
}

// Plan passes a small controller table through the real AO parser for generator tests.
func Plan(t testing.TB, groups int) *input.Plan {
	t.Helper()
	var source strings.Builder
	source.WriteString(Header)
	for i := range groups {
		source.WriteString(Row("FCS1", fmt.Sprintf("A%d-00", 11+i), 2, fmt.Sprintf("_TAG%d", i)))
	}
	plan, err := input.Parse([]byte(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
