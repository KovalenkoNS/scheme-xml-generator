// Rules-gate regressions ensure authored tests and helpers cannot silently escape CODE-002.
package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestOwnedGoIncludesTestsAndSupport checks actual inventory selectors include both test layouts and helper code.
func TestOwnedGoIncludesTestsAndSupport(t *testing.T) {
	for _, path := range []string{"tests/support/aomap/factory.go", "tests/whitebox/internal/generator/fbd/document_test.go", "tests/generator/st/planning_test.go", "internal/domain/hardware/modules.go"} {
		if !ownedGo(path) {
			t.Errorf("owned source excluded: %s", path)
		}
	}
	for _, path := range []string{"tests/retired/fixed-fbd/io_test.go.txt", ".artifacts/generated.go", "libraries/example.xml"} {
		if ownedGo(path) {
			t.Errorf("non-source included: %s", path)
		}
	}
}

// TestCommentOmissionsFailForTestsAndSupport parses deliberately incomplete helper/test files and checks exact gate failures.
func TestCommentOmissionsFailForTestsAndSupport(t *testing.T) {
	for _, path := range []string{"tests/support/fixture.go", "tests/whitebox/internal/generator/fbd/example_test.go"} {
		for _, item := range []struct {
			name, source, missing string
			want                  int
		}{
			{"both absent", "package fixture; func Build() {}", "", 2},
			{"function absent", "// Fixture values used by the renderer test.\npackage fixture\nfunc Build() {}", "missing function", 1},
			{"header absent", "package fixture\n// Build supplies one channel to the renderer test.\nfunc Build() {}", "missing file", 1},
			{"documented", "// Fixture values used by the renderer test.\npackage fixture\n// Build supplies one channel to the renderer test.\nfunc Build() {}", "", 0},
		} {
			t.Run(path+"/"+item.name, func(t *testing.T) {
				set := token.NewFileSet()
				file, err := parser.ParseFile(set, path, item.source, parser.ParseComments)
				if err != nil {
					t.Fatal(err)
				}
				failures := auditGoComments(path, set, file)
				if len(failures) != item.want || item.missing != "" && !strings.Contains(strings.Join(failures, "\n"), item.missing) {
					t.Fatalf("findings=%v, want %d containing %q", failures, item.want, item.missing)
				}
			})
		}
	}
}
