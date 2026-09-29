// Проверка безопасного имени результата предметного HTTP-компонента.
package techobjectapi

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestTechObjectsOutputNamesAreSafe checks XLS output-name normalization prevents traversal and invalid file names
// at the technology-object HTTP boundary.
func TestTechObjectsOutputNamesAreSafe(t *testing.T) {
	name := techObjectsOutputName(`../../folder\objects.xlsx`, "PLC_TEST")
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, "_PLC_TEST.xls") {
		t.Fatalf("unsafe output name %q", name)
	}
}
