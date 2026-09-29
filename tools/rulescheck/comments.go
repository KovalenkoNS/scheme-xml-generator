// Comment checks cover every owned Go source, test and support helper; meaning is reviewed separately.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// ownedGo selects authored production and test sources; retired text and runtime artifacts are outside this inventory.
func ownedGo(relative string) bool {
	if !strings.HasSuffix(relative, ".go") {
		return false
	}
	for _, owner := range []string{"internal/", "cmd/", "tools/", "web/", "tests/"} {
		if strings.HasPrefix(relative, owner) {
			return true
		}
	}
	return false
}

// inspectGoComments parses one owned source and returns concrete file/function omissions for the rules gate.
func inspectGoComments(path, relative string) (*ast.File, []string) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
	if err != nil {
		return nil, []string{"CODE-002: cannot inspect " + relative + ": " + err.Error()}
	}
	return file, auditGoComments(relative, set, file)
}

// auditGoComments checks a parsed file's responsibility header and every named function/method comment.
// It detects omissions only; descriptions still require a review against the function's actual component and effect.
func auditGoComments(relative string, set *token.FileSet, file *ast.File) []string {
	var failures []string
	if file.Doc == nil || strings.TrimSpace(file.Doc.Text()) == "" {
		failures = append(failures, "CODE-002: missing file responsibility: "+relative)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && (function.Doc == nil || strings.TrimSpace(function.Doc.Text()) == "") {
			failures = append(failures, fmt.Sprintf("CODE-002: %s:%d: missing function comment for %s", relative, set.Position(function.Pos()).Line, function.Name.Name))
		}
	}
	return failures
}
