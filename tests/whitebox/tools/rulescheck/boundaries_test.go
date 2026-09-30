// Architecture acceptance rejects the exact abstractions previously rejected by the owner.
package main

import (
	"go/parser"
	"go/token"
	"testing"
)

// TestAbstractionBoundaries checks dependency direction and source-specific identities, not file counts.
func TestAbstractionBoundaries(t *testing.T) {
	cases := []struct {
		name, path, source string
		rejected           bool
	}{
		{"renderer imports source parser", "internal/generator/st/assignments.go", `package st; import "scheme-xml-generator/internal/inputs/assignments"`, true},
		{"domain imports HTTP", "internal/domain/assignments/model.go", `package assignments; import "scheme-xml-generator/internal/httpapi/transport"`, true},
		{"source name becomes domain type", "internal/domain/assignments/model.go", `package assignments; type SKZPlan struct {}`, true},
		{"table header becomes domain field", "internal/domain/analogoutput/model.go", `package analogoutput; type Group struct { FCS string }`, true},
		{"legacy JSON alias stays a wire field", "internal/domain/analogoutput/model.go", "package analogoutput; type Group struct { ControllerName string `json:\"fcs\"` }", false},
		{"renderer uses general model", "internal/generator/st/assignments.go", `package st; import "scheme-xml-generator/internal/domain/assignments"`, false},
		{"URL alias stays at transport boundary", "internal/httpapi/io/modulemapping/routes.go", `package modulemapping; const alias = "/api/skz/preview"`, false},
		{"HTTP imports aggregate facade", "internal/httpapi/hmi/ao.go", `package hmi; import "scheme-xml-generator/internal/generator"`, true},
		{"HTTP imports sibling generator", "internal/httpapi/hmi/ao.go", `package hmi; import "scheme-xml-generator/internal/generator/fbd"`, true},
		{"HTTP imports own generator", "internal/httpapi/hmi/ao.go", `package hmi; import "scheme-xml-generator/internal/generator/hmi"`, false},
		{"workspace imports renderer", "internal/httpapi/workspace/service.go", `package workspace; import "scheme-xml-generator/internal/generator/fbd"`, true},
		{"workspace imports settings", "internal/httpapi/workspace/service.go", `package workspace; import "scheme-xml-generator/internal/config"`, false},
		{"mixed contracts restored", "internal/generator/fbd/document.go", `package fbd; import "scheme-xml-generator/internal/generator/contracts"`, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), item.path, item.source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			failures := auditImports(item.path, file)
			if (len(failures) > 0) != item.rejected {
				t.Fatalf("rejected=%v, findings=%v", item.rejected, failures)
			}
		})
	}
}
