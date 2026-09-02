package architecture

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestObjectFactsStayInTheirDefiningPackage enforces that a package may call
// analysis.Pass.ImportObjectFact or ExportObjectFact only for a fact type it
// defines itself.
//
// Fact families are the boundary between intra-procedural evidence discovery
// and cross-package propagation. Each family keeps that propagation inside the
// package that owns the fact type (lifecyclefacts owns the lifecycle summary;
// closedomain owns the closed-string-domain marker), and any analyzer that
// needs a family's conclusions consumes them through that package's facade
// (for example lifecyclefacts.LifecycleEvidence) rather than importing the raw
// fact and re-deriving the call-site argument-to-parameter mapping. Reaching
// into another package's facts by hand is exactly where that mapping gets lost,
// so this test keeps the import/export calls co-located with their definition.
func TestObjectFactsStayInTheirDefiningPackage(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers", "internal/passes") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/analyzers/...", "./internal/passes/...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load fact-owning packages: %d errors", errors)
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
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := objectFactCall(pkg.TypesInfo, call)
				if !ok {
					return true
				}
				owner, resolved := factArgumentPackage(pkg.TypesInfo, call)
				if !resolved {
					// The fact argument is not a pointer to a named type we can
					// resolve (for example an interface value). Stay silent
					// rather than invent a boundary violation.
					return true
				}
				if owner == pkg.Types.Path() {
					return true
				}
				position := pkg.Fset.Position(call.Pos())
				t.Errorf("%s:%d calls analysis.Pass.%s for a fact defined in %q; import and export object facts only in the package that defines the fact type",
					repositoryPath, position.Line, method, owner)
				return true
			})
		}
	}
}

// objectFactCall reports whether call invokes analysis.Pass.ImportObjectFact or
// ExportObjectFact, returning the field name. Both are func-typed fields of
// analysis.Pass, so the selector resolves to a *types.Var, not a method.
func objectFactCall(info *types.Info, call *ast.CallExpr) (string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	field, ok := info.Uses[selector.Sel].(*types.Var)
	if !ok || !field.IsField() || field.Pkg() == nil || field.Pkg().Path() != "golang.org/x/tools/go/analysis" {
		return "", false
	}
	switch field.Name() {
	case "ImportObjectFact", "ExportObjectFact":
		return field.Name(), true
	default:
		return "", false
	}
}

// factArgumentPackage resolves the defining package path of the fact value
// passed to an object-fact call. Both fields take the fact as their second
// argument, and callers pass a pointer to the concrete fact type, so the type
// resolves to a *T whose named element identifies the owning package.
func factArgumentPackage(info *types.Info, call *ast.CallExpr) (string, bool) {
	if len(call.Args) < 2 {
		return "", false
	}
	argument := info.TypeOf(call.Args[1])
	if pointer, ok := argument.(*types.Pointer); ok {
		argument = pointer.Elem()
	}
	named, ok := argument.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return "", false
	}
	return named.Obj().Pkg().Path(), true
}
