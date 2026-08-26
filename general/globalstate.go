package general

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

const globalStateDirective = "gohawk:globalstate "

func globalStateAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "globalstate",
		Doc:  "checks mutable package-level state",
		Run:  runGlobalState,
	}
}

func runGlobalState(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.VAR {
				continue
			}
			checkGlobalDeclaration(pass, file, generic)
		}
	}
	return nil, nil
}

func checkGlobalDeclaration(pass *analysis.Pass, file *ast.File, declaration *ast.GenDecl) {
	for _, specification := range declaration.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		locallyAllowed := globalStateAllowedAt(pass, file, value.Pos())
		for index, name := range value.Names {
			if name.Name == "_" {
				continue
			}
			object := pass.TypesInfo.Defs[name]
			if object == nil || !mutableGlobal(object.Type()) || locallyAllowed || allowedGlobal(pass, name.Name, object.Type(), value, index) {
				continue
			}
			pass.Reportf(name.Pos(), "mutable package state %s requires an immutable owner or //gohawk:globalstate <rationale>", name.Name)
		}
	}
}

func globalStateAllowedAt(pass *analysis.Pass, file *ast.File, position token.Pos) bool {
	line := pass.Fset.Position(position).Line
	for _, group := range file.Comments {
		if pass.Fset.Position(group.End()).Line != line-1 {
			continue
		}
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if strings.HasPrefix(text, globalStateDirective) && strings.TrimSpace(strings.TrimPrefix(text, globalStateDirective)) != "" {
				return true
			}
		}
	}
	return false
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
