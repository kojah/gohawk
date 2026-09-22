package architecture

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Summary components may be typed in analyzer code, but their prerequisites,
// construction, and fact-backed lookup enter through the summary broker.
// Checking resolved objects catches import aliases, dot imports, and assigning
// constructors to function variables rather than only direct selector calls.
func TestAnalyzersUseSummaryBroker(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers") {
		production[source.absolutePath] = source.repositoryPath
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  inventory.root,
	}, "./internal/analyzers/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load summary consumers: %d errors", errors)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			path, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if summaryBrokerNodeForbidden(pkg.TypesInfo, node) {
					t.Errorf("%s:%d bypasses the summary broker; obtain selected knowledge through internal/summaries",
						path, pkg.Fset.Position(node.Pos()).Line)
				}
				return true
			})
		}
	}
}

func summaryBrokerNodeForbidden(info *types.Info, node ast.Node) bool {
	switch node := node.(type) {
	case *ast.Ident:
		return summaryBrokerObjectForbidden(info.Uses[node])
	case *ast.CompositeLit:
		return summaryBrokerConstructionForbidden(info.TypeOf(node))
	case *ast.TypeAssertExpr:
		return summaryBrokerConstructionForbidden(info.TypeOf(node.Type))
	case *ast.CallExpr:
		identifier, ok := node.Fun.(*ast.Ident)
		if !ok {
			return false
		}
		builtin, ok := info.Uses[identifier].(*types.Builtin)
		return ok && builtin.Name() == "new" && summaryBrokerConstructionForbidden(info.TypeOf(node))
	default:
		return false
	}
}

func summaryDomainPackage(path string) bool {
	return path == internalImportPrefix+"passes/lifecyclefacts" || path == internalImportPrefix+"passes/concurrencyfacts" ||
		path == internalImportPrefix+"passes/resultfacts"
}

func summaryBrokerObjectForbidden(object types.Object) bool {
	if object == nil || object.Pkg() == nil || !summaryDomainPackage(object.Pkg().Path()) {
		return false
	}
	if variable, ok := object.(*types.Var); ok {
		return !variable.IsField() && variable.Name() == "Analyzer"
	}
	function, ok := object.(*types.Func)
	if !ok {
		return false
	}
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() != nil {
		return false
	}
	// These helpers inspect type/value structure without obtaining summaries
	// or reading pass-owned knowledge. New exceptions require explicit review.
	path := function.Pkg().Path()
	if path == internalImportPrefix+"passes/lifecyclefacts" && function.Name() == "ResourceCleanup" {
		return false
	}
	if path == internalImportPrefix+"passes/concurrencyfacts" && (function.Name() == "FreshMutex" || function.Name() == "MutexPointer") {
		return false
	}
	return true
}

func summaryBrokerConstructionForbidden(value types.Type) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || !summaryDomainPackage(named.Obj().Pkg().Path()) {
		return false
	}
	switch named.Obj().Name() {
	case "Engine", "LifecycleEvidence", "Summaries", "Fact":
		return true
	default:
		return false
	}
}
