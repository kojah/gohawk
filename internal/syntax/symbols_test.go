package syntax

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestIsCallToUsesResolvedIdentity(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "symbols.go", `
package symboltest

import tm "time"

type timer struct{}

func (timer) AfterFunc() {}

func calls(local timer) {
	tm.AfterFunc(0, func() {})
	local.AfterFunc()
	_ = len([]int{})
}
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("example.com/symboltest", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{TypesInfo: info}
	calls := sourceCalls(file)

	if !IsCallTo(pass, calls[0], PackageFunction("time", "AfterFunc")) {
		t.Error("aliased time.AfterFunc call did not match its package function")
	}
	if IsCallTo(pass, calls[1], PackageFunction("time", "AfterFunc")) {
		t.Error("same-named local method matched time.AfterFunc")
	}
	if !IsCallTo(pass, calls[1], PackageMethod(MethodSymbol{PackagePath: pkg.Path(), Receiver: "timer", Name: "AfterFunc"})) {
		t.Error("local method did not match its receiver-qualified identity")
	}
	if !IsCallTo(pass, calls[2], Builtin("len")) {
		t.Error("len call did not match its builtin identity")
	}
}

func TestSymbolMatchesPackageVariable(t *testing.T) {
	osPackage, err := importer.Default().Import("os")
	if err != nil {
		t.Fatal(err)
	}
	args := osPackage.Scope().Lookup("Args")
	if !PackageVariable("os", "Args").MatchesObject(args) {
		t.Error("os.Args did not match its package variable identity")
	}
	if PackageVariable("os", "Args").MatchesObject(osPackage.Scope().Lookup("Getenv")) {
		t.Error("os.Getenv matched a package variable identity")
	}
}

func sourceCalls(file *ast.File) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}
