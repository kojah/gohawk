// Package ssaflowtest builds SSA packages from source strings for tests of
// the flow and fact machinery, so each test package does not repeat the parse
// and build steps.
package ssaflowtest

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// BuildPackage type-checks and builds source as one package at path, named
// by the last path element, with SSA sanity checks enabled. Failures end the
// test.
func BuildPackage(t *testing.T, path, source string) *ssa.Package {
	t.Helper()
	name := path[strings.LastIndex(path, "/")+1:]
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name+".go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: importer.Default()},
		fset,
		types.NewPackage(path, name),
		[]*ast.File{file},
		ssa.SanityCheckFunctions,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}
