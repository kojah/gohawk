package architecture

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestAnalyzerSummaryBoundaries checks API use, not which evidence model an
// analyzer chooses. Completion proofs and context-keyed summaries remain valid
// alternatives to FunctionSummaries. Layering and traversal have their own
// tests; unknown propagation and cache correctness need behavioral tests.
func TestAnalyzerSummaryBoundaries(t *testing.T) {
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
	if count := packages.PrintErrors(loaded); count != 0 {
		t.Fatalf("load summary consumers: %d errors", count)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			path, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				reason := summaryBoundaryViolation(pkg.TypesInfo, node)
				if reason == "" {
					return true
				}
				t.Errorf("%s:%d: %s", path, pkg.Fset.Position(node.Pos()).Line, reason)
				return true
			})
		}
	}
}

const summaryNilBudget = "summary query passes nil budget; share a bounded SearchBudget with nested computation and binding"

func summaryBoundaryViolation(info *types.Info, node ast.Node) string {
	switch node := node.(type) {
	case *ast.SelectorExpr:
		selection := info.Selections[node]
		if !summaryInfrastructureSelection(selection) {
			return ""
		}
		if selection.Kind() == types.FieldVal {
			return "summary consumer accesses infrastructure state; use the summary query API, not cache or guard fields"
		}
		switch selection.Obj().Name() {
		case "Answer", "Enter", "Entered", "Leave", "Cut":
			return "analyzer manages summary caching or recursion directly; use FunctionSummaries or CallGraphMemo.Summarize"
		}
	case *ast.CallExpr:
		selector, ok := ast.Unparen(node.Fun).(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		selection := info.Selections[selector]
		if !summaryInfrastructureSelection(selection) {
			return ""
		}
		index := summaryBudgetIndex(selection)
		if index >= 0 && index < len(node.Args) && info.Types[ast.Unparen(node.Args[index])].IsNil() {
			return summaryNilBudget
		}
	}
	return ""
}

func summaryInfrastructureType(candidate types.Type) bool {
	candidate = types.Unalias(candidate)
	if pointer, ok := candidate.(*types.Pointer); ok {
		candidate = types.Unalias(pointer.Elem())
	}
	named, ok := candidate.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != internalImportPrefix+"ssaflow" {
		return false
	}
	return named.Obj().Name() == "FunctionSummaries" || named.Obj().Name() == "CallGraphMemo"
}

func summaryInfrastructureSelection(selection *types.Selection) bool {
	if selection == nil {
		return false
	}
	if method, ok := selection.Obj().(*types.Func); ok {
		signature, ok := method.Type().(*types.Signature)
		return ok && signature.Recv() != nil && summaryInfrastructureType(signature.Recv().Type())
	}
	return summaryInfrastructureType(selection.Recv())
}

func summaryBudgetIndex(selection *types.Selection) int {
	index := -1
	switch selection.Obj().Name() {
	case "Function", "AtCall":
		index = 1
	case "Summarize":
		index = 2
	}
	if index >= 0 && selection.Kind() == types.MethodExpr {
		index++ // An explicit receiver is the first argument of a method expression.
	}
	return index
}
