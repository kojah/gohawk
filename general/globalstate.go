package general

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func globalStateAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "globalstate",
		Doc:  "checks mutable package-level state",
		Run:  runGlobalState,
	}
}

func runGlobalState(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if analysisutil.GeneratedFile(file) {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.VAR {
				continue
			}
			checkGlobalDeclaration(pass, generic)
		}
	}
	return nil, nil
}

func checkGlobalDeclaration(pass *analysis.Pass, declaration *ast.GenDecl) {
	for _, specification := range declaration.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			object := pass.TypesInfo.Defs[name]
			if object == nil || !mutableGlobal(object.Type()) || allowedGlobal(pass, name.Name, object.Type(), value, index) {
				continue
			}
			pass.Reportf(name.Pos(), "mutable package state %s requires a contextual allowlist or immutable owner", name.Name)
		}
	}
}

func mutableGlobal(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Map, *types.Slice, *types.Pointer, *types.Interface, *types.Signature, *types.Chan:
		return true
	default:
		return false
	}
}

func allowedGlobal(pass *analysis.Pass, name string, value types.Type, specification *ast.ValueSpec, index int) bool {
	if strings.HasSuffix(name, "Schema") || analysisutil.NamedType(value, "sync", "Once") || analysisutil.NamedType(value, "regexp", "Regexp") {
		return true
	}
	if index < len(specification.Values) {
		call, ok := specification.Values[index].(*ast.CallExpr)
		return ok && analysisutil.IsPackageCall(pass, call, analysisutil.FunctionSymbol{Package: "errors", Name: "New"})
	}
	return false
}
