package architecture

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestAnalyzersUseSharedTraversal keeps value-provenance mechanics in
// ssaflow. An analyzer must not fan out over phi edges itself, and must not
// thread its own visited set through a recursive walk: both belong to
// ssaflow.ReachingWalk, ssaflow.WalkStates, and the phi helpers, so the
// cycle guard and the edge bounds check exist once.
func TestAnalyzersUseSharedTraversal(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/analyzers/...")
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
