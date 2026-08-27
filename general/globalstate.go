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
	config := globalStateConfig{}
	analyzer := &analysis.Analyzer{
		Name: "globalstate",
		Doc:  "checks mutable package-level state",
	}
	analyzer.Flags.StringVar(&config.allowNames, "allow-names", "", "comma-separated package variable names to allow")
	analyzer.Flags.StringVar(&config.allowTypes, "allow-types", "", "comma-separated fully-qualified named types to allow")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runGlobalState(pass, config)
	}
	return analyzer
}

type globalStateConfig struct {
	allowNames string
	allowTypes string
}

type globalStateAllowlist struct {
	names map[string]bool
	types map[string]bool
}

func runGlobalState(pass *analysis.Pass, config globalStateConfig) (any, error) {
	allowlist := globalStateAllowlist{names: commaSeparatedSet(config.allowNames), types: commaSeparatedSet(config.allowTypes)}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.VAR {
				continue
			}
			checkGlobalDeclaration(pass, generic, allowlist)
		}
	}
	return nil, nil
}

func checkGlobalDeclaration(pass *analysis.Pass, declaration *ast.GenDecl, allowlist globalStateAllowlist) {
	for _, specification := range declaration.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			if name.Name == "_" {
				continue
			}
			object := pass.TypesInfo.Defs[name]
			if object == nil || !mutableGlobal(object.Type()) || allowlist.names[name.Name] || allowlist.types[qualifiedTypeName(object.Type())] || allowedGlobal(pass, name.Name, object.Type(), value, index) {
				continue
			}
			analysisutil.Reportf(pass, name.Pos(), "mutable package state %s requires an immutable owner or //gohawk:ignore globalstate", name.Name)
		}
	}
}

func qualifiedTypeName(value types.Type) string {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
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
