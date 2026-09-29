// Direct-core regressions close the removed physical-IO format independently of the HTTP guard.
package generator_test

import (
	"scheme-xml-generator/internal/config"
	"scheme-xml-generator/internal/generator"
	"strings"
	"testing"
)

// TestCoreRejectsRetiredIOBeforeLibraryRendering checks AI/AO/DI/DO cannot reactivate fixed schemes through Go APIs.
// A valid resolved library signal is present so rejection is specifically caused by the retired IO section.
func TestCoreRejectsRetiredIOBeforeLibraryRendering(t *testing.T) {
	for _, kind := range []string{"AI", "AO", "DI", "DO"} {
		t.Run(kind, func(t *testing.T) {
			input := resolved(dioTemplate(), false)
			input[0].Request.IO = &generator.IORequest{Type: kind}
			if _, err := generator.NormalizePOURequests([]generator.POURequest{input[0].Request}); err == nil || !strings.Contains(err.Error(), "io.modules") {
				t.Fatalf("normalization accepted retired IO: %v", err)
			}
			if _, err := generator.RequirementsForDocument(input); err == nil || !strings.Contains(err.Error(), "io.modules") {
				t.Fatalf("allocation planning accepted retired IO: %v", err)
			}
			result, err := (generator.Generator{Config: config.Default()}).GenerateDocument(generator.Request{}, input, generator.IDRange{T11Start: 100, CardStart: 200, POUID: 300})
			if err == nil || !strings.Contains(err.Error(), "io.modules") || len(result.XML) != 0 {
				t.Fatalf("renderer accepted retired IO: %v", err)
			}
		})
	}
}
