// Package globalstate implements the globalstate gohawk analyzer.
package globalstate

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
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
	allowlist := globalStateAllowlist{names: flagvalue.CommaSeparatedSet(config.allowNames), types: flagvalue.CommaSeparatedSet(config.allowTypes)}
	usage := globalStateUsage{files: pass.Files, parents: globalSyntaxParents(pass.Files), calleeParams: globalCalleeParameters(pass)}
	for _, file := range pass.Files {
		if !syntax.AnalyzeFile(pass, file) {
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
	// Type mutability selects candidates; ownership and observed usage decide
	// whether the value is effectively immutable. Explicit configuration is
	// checked before the evidence model so intentional framework globals remain
	// a stable repository policy rather than name-based analyzer exceptions.
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
			if object == nil || !mutableGlobal(object.Type()) || allowlist.names[name.Name] || allowlist.types[qualifiedTypeName(object.Type())] ||
				allowedGlobal(pass, name, object, declaration, value, index, usage) {
				continue
			}
			check.Reportf(
				pass,
				check.MutableGlobalState,
				name.Pos(),
				"mutable package state %s requires an immutable owner or //gohawk:ignore globalstate",
				name.Name,
			)
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

func allowedGlobal(
	pass *analysis.Pass,
	name *ast.Ident,
	object types.Object,
	declaration *ast.GenDecl,
	specification *ast.ValueSpec,
	index int,
	usage globalStateUsage,
) bool {
	value := object.Type()
	if strings.HasSuffix(name.Name, "Schema") || syntax.NamedType(value, "sync", "Once") || syntax.NamedType(value, "regexp", "Regexp") {
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
	if conventionalAnalyzerSingleton(pass, object, value, usage) || immutableRuntimeDescriptor(pass, object, value, usage) {
		return true
	}
	if conventionalFrameworkGlobal(value) {
		return true
	}
	return effectivelyImmutableComposite(pass, name, object, declaration, specification, index, usage)
}
