package architecture

import (
	"go/ast"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestAnalyzersUseSharedReporting(t *testing.T) {
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
			absolutePath := stableAbsolutePath(pkg.CompiledGoFiles[index])
			repositoryPath, ok := production[absolutePath]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || !analysisPassReport(pkg.TypesInfo, selector) {
					return true
				}
				position := pkg.Fset.Position(selector.Sel.Pos())
				t.Errorf("%s:%d reports through analysis.Pass directly; use check.Report or check.Reportf", repositoryPath, position.Line)
				return true
			})
		}
	}
}

// analysisPassReport reports whether the selector reads one of the reporting
// members of analysis.Pass: the Report field, which is a func-typed field and
// so resolves to a variable rather than a method, or the Reportf and
// ReportRangef methods.
func analysisPassReport(info *types.Info, selector *ast.SelectorExpr) bool {
	name, ok := analysisPassMember(info, selector)
	return ok && (name == "Report" || name == "Reportf" || name == "ReportRangef")
}

// analysisPassMember resolves a selector to a field or method of analysis.Pass
// and returns its name. Fields resolve to *types.Var and methods to
// *types.Func; both must be accepted, or a guard on a func-typed field such as
// Pass.Report silently never matches.
func analysisPassMember(info *types.Info, selector *ast.SelectorExpr) (string, bool) {
	selection, ok := info.Selections[selector]
	if !ok {
		return "", false
	}
	receiver := selection.Recv()
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = pointer.Elem()
	}
	named, ok := receiver.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "golang.org/x/tools/go/analysis" || named.Obj().Name() != "Pass" {
		return "", false
	}
	switch object := selection.Obj().(type) {
	case *types.Var, *types.Func:
		return object.Name(), true
	}
	return "", false
}
