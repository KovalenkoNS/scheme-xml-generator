// Dependency and identity checks keep source adapters outside the domain and sibling renderers.
package main

import (
	"go/ast"
	"strconv"
	"strings"
)

// auditImports prevents subject generators from reaching through sibling implementations.
// FBD/ST/HMI use owned requests/results and models; HTTP composes their public APIs directly.
func auditImports(relative string, file *ast.File) []string {
	var failures []string
	for _, declaration := range file.Imports {
		name, _ := strconv.Unquote(declaration.Path.Value)
		if name == "scheme-xml-generator/internal/generator" || strings.HasPrefix(name, "scheme-xml-generator/internal/generator/contracts") {
			failures = append(failures, "CODE-003: aggregate generator API or mixed contracts dependency: "+relative+": "+name)
		}
		for _, subject := range []string{"fbd", "st", "hmi"} {
			if strings.HasPrefix(relative, "internal/httpapi/"+subject+"/") {
				for _, sibling := range []string{"fbd", "st", "hmi"} {
					if sibling != subject && name == "scheme-xml-generator/internal/generator/"+sibling {
						failures = append(failures, "CODE-003: HTTP subject depends on sibling generator: "+relative+": "+name)
					}
				}
			}
		}
		if strings.HasPrefix(relative, "internal/httpapi/workspace/") && strings.HasPrefix(name, "scheme-xml-generator/internal/generator/") {
			failures = append(failures, "CODE-003: workspace settings depend on generation behavior: "+relative+": "+name)
		}
		if strings.HasPrefix(relative, "internal/domain/") && strings.HasPrefix(name, "scheme-xml-generator/internal/") && !strings.HasPrefix(name, "scheme-xml-generator/internal/domain/") {
			failures = append(failures, "CODE-003: domain model depends on an adapter or renderer: "+relative+": "+name)
		}
		if strings.HasPrefix(relative, "internal/generator/") && (strings.HasPrefix(name, "scheme-xml-generator/internal/inputs/") || name == "scheme-xml-generator/internal/aomap" || name == "scheme-xml-generator/internal/iomap") {
			failures = append(failures, "CODE-003: renderer depends on a source parser instead of the shared model: "+relative+": "+name)
		}
	}
	if strings.HasPrefix(relative, "internal/generator/") || strings.HasPrefix(relative, "internal/domain/") {
		ast.Inspect(file, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok {
				if strings.Contains(strings.ToLower(id.Name), "skz") || id.Name == "FCS" || id.Name == "FCSCount" || id.Name == "SourceFCS" || id.Name == "AoSTFCSPattern" {
					failures = append(failures, "CODE-003: source-specific identity in general model/generator: "+relative+": "+id.Name)
				}
			}
			return true
		})
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 4 || parts[0] != "internal" || parts[1] != "generator" {
		return failures
	}
	owner := parts[2]
	for _, declaration := range file.Imports {
		name, _ := strconv.Unquote(declaration.Path.Value)
		if name == "scheme-xml-generator/internal/generator" {
			failures = append(failures, "CODE-001: subject imports compatibility facade: "+relative)
		}
		for _, subject := range []string{"fbd", "st", "hmi"} {
			if owner != subject && name == "scheme-xml-generator/internal/generator/"+subject {
				failures = append(failures, "CODE-001: "+owner+" imports sibling renderer "+subject+": "+relative)
			}
		}
	}
	return failures
}
