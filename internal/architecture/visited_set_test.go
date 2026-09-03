package architecture

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// A recursive walk over the call graph needs a cycle guard, and the usual one
// marks a function on entry and un-marks it on the way out. That guard is
// scoped to the current path, so on its own it makes the walk enumerate call
// paths rather than the call graph: a helper reachable by N paths is re-walked
// N times, which is exponential in a mutually recursive package and once
// stopped gohawk from terminating on a single 89k-line package.
//
// Pairing the guard with a memo fixes the cost, but an answer the guard cut
// short holds only for the path that produced it and must not be retained.
// Those two rules travel together, and getting either wrong fails silently:
// forget the memo and the walk is exponential, forget the cut and the memo
// changes which proofs succeed. ssaflow.CallGraphMemo owns both, so this test
// requires the guard to go through it rather than be rebuilt by hand.
//
// The rule says nothing about a visited set that only ever marks. A monotonic
// set is already sound on its own.

// callGraphMemoImplementation is the one file allowed to un-mark a call-graph
// visited set, because it is the shared guard every other walk delegates to.
const callGraphMemoImplementation = "internal/ssaflow/call_graph_memo.go"

func TestCallGraphGuardsGoThroughTheSharedMemo(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load internal packages: %d errors", errors)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			if index >= len(pkg.CompiledGoFiles) {
				continue
			}
			repositoryPath, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok || repositoryPath == callGraphMemoImplementation {
				continue
			}
			reportHandRolledGuards(t, pkg, file, repositoryPath)
		}
	}
}

func reportHandRolledGuards(t *testing.T, pkg *packages.Package, file *ast.File, repositoryPath string) {
	t.Helper()
	ast.Inspect(file, func(node ast.Node) bool {
		if !unmarksCallGraphVisitedSet(pkg.TypesInfo, node) {
			return true
		}
		line := pkg.Fset.Position(node.Pos()).Line
		t.Errorf("%s:%d un-marks a call-graph visited set by hand; use ssaflow.CallGraphMemo, which keeps the "+
			"path guard and the memo together and withholds an answer the guard cut short",
			repositoryPath, line)
		return true
	})
}

// unmarksCallGraphVisitedSet reports whether the node deletes from a map from
// a function to a bool, which is the visited set a call-graph walk keeps. A map
// keyed by an SSA value is a value walk, which the traversal rules govern
// separately, and a map of anything else, such as a set of held locks, is not a
// visited set at all.
func unmarksCallGraphVisitedSet(info *types.Info, node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}
	if name, ok := call.Fun.(*ast.Ident); !ok || name.Name != "delete" {
		return false
	}
	maps, ok := info.TypeOf(call.Args[0]).(*types.Map)
	if !ok {
		return false
	}
	if basic, ok := maps.Elem().(*types.Basic); !ok || basic.Kind() != types.Bool {
		return false
	}
	pointer, ok := maps.Key().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := pointer.Elem().(*types.Named)
	return ok && named.Obj().Name() == "Function" &&
		named.Obj().Pkg() != nil && named.Obj().Pkg().Name() == "ssa"
}
