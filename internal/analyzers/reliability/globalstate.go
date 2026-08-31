package reliability

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

type globalStateUsage struct {
	files        []*ast.File
	parents      map[ast.Node]ast.Node
	calleeParams map[types.Object][]types.Object
}

func runGlobalState(pass *analysis.Pass, config globalStateConfig) (any, error) {
	allowlist := globalStateAllowlist{names: commaSeparatedSet(config.allowNames), types: commaSeparatedSet(config.allowTypes)}
	usage := globalStateUsage{files: pass.Files, parents: globalSyntaxParents(pass.Files), calleeParams: globalCalleeParameters(pass)}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.VAR {
				continue
			}
			checkGlobalDeclaration(pass, generic, allowlist, usage)
		}
	}
	return nil, nil
}

func checkGlobalDeclaration(pass *analysis.Pass, declaration *ast.GenDecl, allowlist globalStateAllowlist, usage globalStateUsage) {
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
			if object == nil || !mutableGlobal(object.Type()) || allowlist.names[name.Name] || allowlist.types[qualifiedTypeName(object.Type())] || allowedGlobal(pass, name, object, declaration, value, index, usage) {
				continue
			}
			reportf(pass, checkMutableGlobalState, name.Pos(), "mutable package state %s requires an immutable owner or //gohawk:ignore globalstate", name.Name)
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

func allowedGlobal(pass *analysis.Pass, name *ast.Ident, object types.Object, declaration *ast.GenDecl, specification *ast.ValueSpec, index int, usage globalStateUsage) bool {
	value := object.Type()
	if strings.HasSuffix(name.Name, "Schema") || analysisutil.NamedType(value, "sync", "Once") || analysisutil.NamedType(value, "regexp", "Regexp") {
		return true
	}
	if conventionalFrameworkBinding(pass, specification, index) || benchmarkResultSink(pass, name) {
		return true
	}
	if conventionalErrorSentinel(pass, name, object, specification, index, usage) {
		return true
	}
	if documentedTestSeam(name, object, declaration, specification) {
		return true
	}
	if conventionalFrameworkGlobal(value) {
		return true
	}
	return effectivelyImmutableComposite(pass, name, object, specification, index, usage)
}
