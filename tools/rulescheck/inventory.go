// Source inventory checks physical ownership and test placement before invoking focused Go audits.
package main

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// audit compares physical source ownership with the documented generator architecture.
// It inspects versionable files directly, so merely adding a document or empty folder
// cannot satisfy the subject-package and separate-test requirements.
func audit(root string) ([]string, int) {
	var failures []string
	count := 0
	for _, relative := range []string{
		"docs/SOURCE_LAYOUT.md", ".specify/memory/rules.md", "tests/whitebox_test.go", "tests/whitebox/go.mod",
		"internal/generator/fbd", "internal/generator/st", "internal/generator/hmi", "internal/generator/fbd/request",
		"internal/generator/identity", "internal/generator/artifact", "internal/generator/program", "internal/generator/st/assignment",
		"internal/generator/xmlmodel", "internal/generator/xmlcodec", "internal/generator/allocation", "internal/domain/controller", "internal/generator/planning",
		"internal/generator/addressing", "internal/generator/identifiers", "internal/generator/modules", "internal/generator/moduleprofile", "internal/generator/exportprofile",
		"internal/domain/assignments", "internal/domain/analogoutput", "internal/domain/inventory", "internal/domain/hardware",
		"internal/inputs/assignments/raw", "internal/inputs/assignments/prepared", "internal/inputs/xlsx",
		"internal/httpapi/catalog", "internal/httpapi/workspace", "internal/httpapi/fbd", "internal/httpapi/st", "internal/httpapi/hmi",
		"internal/httpapi/io/modulemapping", "internal/httpapi/techobjects", "internal/httpapi/output", "internal/httpapi/transport", "internal/httpapi/limits",
		"web/shared", "web/shell", "web/sources", "web/library", "web/preview", "web/generation/fbd", "web/generation/st", "web/generation/hmi",
	} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			failures = append(failures, "CODE-001: missing "+relative)
			continue
		}
		if info.IsDir() {
			entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(relative)))
			if len(entries) == 0 {
				failures = append(failures, "CODE-001: empty subject directory "+relative)
			}
		}
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".artifacts", ".agents", ".specify", "node_modules", "libraries", "output", "data", "logs":
				return filepath.SkipDir
			}
			if entry.Name() == "testdata" && !strings.HasPrefix(relative, "tests/") {
				failures = append(failures, "CODE-001-T: fixture directory outside tests: "+relative)
			}
			return nil
		}
		isTest := strings.HasSuffix(relative, "_test.go") || strings.HasSuffix(relative, ".test.cjs") || strings.HasSuffix(relative, "_test.cjs")
		if isTest && !strings.HasPrefix(relative, "tests/") {
			failures = append(failures, "CODE-001-T: test outside tests: "+relative)
		}
		if !ownedGo(relative) && !strings.HasPrefix(relative, "web/") {
			return nil
		}
		if filepath.Dir(relative) == "web" {
			switch entry.Name() {
			case "app.js", "workspace.js", "temporary.js", "skz.js", "techobjects.js", "styles.css":
				failures = append(failures, "CODE-001: retired mixed UI source at web root: "+relative)
			}
		}
		if !ownedGo(relative) {
			return nil
		}
		count++
		if filepath.ToSlash(filepath.Dir(relative)) == "internal/generator" || strings.HasPrefix(relative, "internal/generator/contracts/") {
			failures = append(failures, "CODE-001: aggregate generator API or mixed contracts returned: "+relative)
		}
		file, comments := inspectGoComments(path, relative)
		failures = append(failures, comments...)
		if file == nil {
			return nil
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && strings.HasPrefix(relative, "internal/appserver/") && function.Name.Name != "New" && function.Name.Name != "Handler" && function.Name.Name != "Shutdown" {
				failures = append(failures, "CODE-001: subject behavior returned to appserver facade: "+relative+": "+function.Name.Name)
			}
		}
		failures = append(failures, auditImports(relative, file)...)
		return nil
	})
	if err != nil {
		failures = append(failures, "inventory: "+err.Error())
	}
	return failures, count
}
