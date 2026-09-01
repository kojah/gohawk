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
				t.Errorf("%s:%d calls analysis.Pass.Report directly; use check.Report or check.Reportf", repositoryPath, position.Line)
				return true
			})
		}
	}
}

func analysisPassReport(info *types.Info, selector *ast.SelectorExpr) bool {
	object, ok := info.Uses[selector.Sel].(*types.Func)
	return ok && object.Name() == "Report" && object.Pkg() != nil && object.Pkg().Path() == "golang.org/x/tools/go/analysis"
}
