package architecture

import (
	"go/ast"
	"go/types"
	"path"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestAnalyzersUseSharedTraversal keeps value-provenance mechanics in
// lifecycle. An analyzer must not fan out over phi edges itself, and must not
// thread its own visited set through a recursive walk: both belong to
// ssaflow.ReachingWalk, cfg.WalkStates, and the phi helpers, so the
// cycle guard and the edge bounds check exist once.
func TestAnalyzersUseSharedTraversal(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analysis/analyzers") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/analysis/analyzers/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load analyzer packages: %d errors", errors)
	}
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			if index >= len(pkg.CompiledGoFiles) {
				continue
			}
			repositoryPath, ok := production[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.SelectorExpr:
					if phiEdgesField(pkg.TypesInfo, typed) {
						position := pkg.Fset.Position(typed.Sel.Pos())
						t.Errorf("%s:%d ranges over phi edges directly; use ssaflow.ReachingWalk or ssaflow.PhiIncoming", repositoryPath, position.Line)
					}
				case *ast.Ident:
					if object, ok := pkg.TypesInfo.Defs[typed]; ok && visitedValueSet(object) {
						position := pkg.Fset.Position(typed.Pos())
						t.Errorf("%s:%d declares a visited set of SSA values; let ssaflow.ReachingWalk own the cycle guard", repositoryPath, position.Line)
					}
				}
				return true
			})
		}
	}
}

// phiEdgesField reports whether the selector reads the Edges field of
// *ssa.Phi.
func phiEdgesField(info *types.Info, selector *ast.SelectorExpr) bool {
	selection, ok := info.Selections[selector]
	if !ok || selection.Kind() != types.FieldVal || selection.Obj().Name() != "Edges" {
		return false
	}
	return ssaTypeNamed(selection.Recv(), "Phi")
}

// visitedValueSet reports whether object is a variable whose name marks it
// as a visited set and whose type is a map keyed by ssa.Value or *ssa.Phi.
func visitedValueSet(object types.Object) bool {
	variable, ok := object.(*types.Var)
	if !ok {
		return false
	}
	name := strings.ToLower(variable.Name())
	if !strings.Contains(name, "seen") && !strings.Contains(name, "visited") {
		return false
	}
	mapped, ok := variable.Type().Underlying().(*types.Map)
	if !ok {
		return false
	}
	return ssaTypeNamed(mapped.Key(), "Value") || ssaTypeNamed(mapped.Key(), "Phi")
}

func ssaTypeNamed(candidate types.Type, name string) bool {
	if pointer, ok := candidate.(*types.Pointer); ok {
		candidate = pointer.Elem()
	}
	named, ok := candidate.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "golang.org/x/tools/go/ssa" && named.Obj().Name() == name
}

// A transparent form says that one SSA wrapper may be looked through. Which
// wrappers are sound to follow depends on what is being proved: an ownership
// proof may follow a type assertion, because the assertion selects the same
// object, while a proof about which methods a value has must stop there.
//
// Every caller therefore names the forms it wants. Passing a mask assembled
// elsewhere would make a later form apply to proofs nobody reviewed for it,
// and the widening would be silent: no test fails when a proof quietly starts
// following one more wrapper. This test keeps the choice at the call site.

// transparentFormParameters are the functions whose forms argument selects
// which wrappers a proof may look through.
var transparentFormParameters = map[string]int{
	"UnwrapTransparentValue": 1,
	"NewReachingWalk":        0,
}

func TestTransparentFormsAreNamedAtTheCallSite(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	sources := inventory.productionGoFiles(t, "internal")
	// A named mask declared beside the proof, such as contextForms, is the
	// preferred shape: it is explicit and can be documented once. It is
	// declared per package, not per file, so collect the whole package first.
	declared := map[string]map[string]bool{}
	for _, source := range sources {
		directory := path.Dir(source.repositoryPath)
		if declared[directory] == nil {
			declared[directory] = map[string]bool{}
		}
		for name := range transparentFormDeclarations(source.file) {
			declared[directory][name] = true
		}
	}
	for _, source := range sources {
		named := declared[path.Dir(source.repositoryPath)]
		for _, function := range source.file.Decls {
			declaration, ok := function.(*ast.FuncDecl)
			if !ok || declaration.Body == nil {
				continue
			}
			checkTransparentFormArguments(t, source, declaration, named)
		}
	}
}

func checkTransparentFormArguments(
	t *testing.T,
	source repositoryGoSource,
	function *ast.FuncDecl,
	named map[string]bool,
) {
	t.Helper()
	locals := transparentFormLocals(function.Body)
	for name := range named {
		locals[name] = true
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		index, ok := transparentFormParameters[calleeName(call.Fun)]
		if !ok || index >= len(call.Args) {
			return true
		}
		if namesTransparentForms(call.Args[index], locals) {
			return true
		}
		position := source.fileSet.Position(call.Args[index].Pos())
		t.Errorf("%s:%d passes transparent forms that are not named here; write the Transparent... constants this "+
			"proof may follow, so a form added later does not widen it silently",
			source.repositoryPath, position.Line)
		return true
	})
}

// transparentFormDeclarations returns the file-level names declared as an
// explicit union of transparent forms.
func transparentFormDeclarations(file *ast.File) map[string]bool {
	declared := map[string]bool{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range group.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range value.Names {
				if index < len(value.Values) && namesTransparentForms(value.Values[index], nil) {
					declared[name.Name] = true
				}
			}
		}
	}
	return declared
}

// transparentFormLocals returns the local names assigned an explicit union of
// transparent forms, which callers use when the same list serves several
// queries in one function.
func transparentFormLocals(body *ast.BlockStmt) map[string]bool {
	locals := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, target := range assignment.Lhs {
			name, ok := target.(*ast.Ident)
			if !ok || index >= len(assignment.Rhs) {
				continue
			}
			if namesTransparentForms(assignment.Rhs[index], nil) {
				locals[name.Name] = true
			}
		}
		return true
	})
	return locals
}

// namesTransparentForms reports whether the expression is a union of
// Transparent... constants, or a local holding one.
func namesTransparentForms(expression ast.Expr, locals map[string]bool) bool {
	switch typed := expression.(type) {
	case *ast.BinaryExpr:
		return namesTransparentForms(typed.X, locals) && namesTransparentForms(typed.Y, locals)
	case *ast.ParenExpr:
		return namesTransparentForms(typed.X, locals)
	case *ast.SelectorExpr:
		// A walk built with a checked mask may forward it: the choice was made,
		// and checked, where the walk was constructed.
		return strings.HasPrefix(typed.Sel.Name, "Transparent") || typed.Sel.Name == "forms"
	case *ast.Ident:
		return strings.HasPrefix(typed.Name, "Transparent") || locals[typed.Name]
	}
	return false
}

func calleeName(function ast.Expr) string {
	switch typed := function.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	}
	return ""
}

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
// changes which proofs succeed. calls.CallGraphMemo owns both, so this test
// requires the guard to go through it rather than be rebuilt by hand.
//
// The rule says nothing about a visited set that only ever marks. A monotonic
// set is already sound on its own.

// callGraphMemoImplementation is the one file allowed to un-mark a call-graph
// visited set, because it is the shared guard every other walk delegates to.
const callGraphMemoImplementation = "internal/engine/ssaflow/calls/call_graph_memo.go"

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
		t.Errorf("%s:%d un-marks a call-graph visited set by hand; use calls.CallGraphMemo, which keeps the "+
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

func TestAnalyzersUseSymbolIdentity(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)

	// All current analyzers can identify known declarations through Symbol.
	rawIdentityPatterns := []string{
		"CallPackage(",
		".Pkg().Path()",
		".Pkg.Pkg.Path()",
		"Imported().Path()",
		"*types.Builtin",
		"BuiltinClose",
	}
	for _, source := range inventory.productionGoFiles(t, "internal/analysis/analyzers") {
		text := string(source.source)
		escapes := 0
		for _, pattern := range rawIdentityPatterns {
			escapes += strings.Count(text, pattern)
		}
		relative := strings.TrimPrefix(source.repositoryPath, "internal/analysis/analyzers/")
		if escapes != 0 {
			t.Errorf("%s has %d raw package-identity escapes; use Symbol", relative, escapes)
		}
	}
}
