package architecture

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestAnalyzerCodeUsesStructuredTracing(t *testing.T) {
	t.Parallel()
	t.Run("typed identity", testForbiddenAnalyzerPrintIdentity)
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
			inspectUnstructuredPrints(t, pkg, file, repositoryPath)
		}
	}
}

func inspectUnstructuredPrints(t *testing.T, pkg *packages.Package, file *ast.File, repositoryPath string) {
	t.Helper()
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		printName, forbidden := forbiddenAnalyzerPrint(pkg.TypesInfo.Uses[identifier])
		if !forbidden {
			return true
		}
		position := pkg.Fset.Position(identifier.Pos())
		t.Errorf("%s:%d uses %s in analyzer code; use internal/trace for diagnostic instrumentation", repositoryPath, position.Line, printName)
		return true
	})
}

func forbiddenAnalyzerPrint(object types.Object) (string, bool) {
	function, ok := object.(*types.Func)
	if !ok || function.Pkg() == nil || function.Pkg().Path() != "fmt" {
		return "", false
	}
	switch function.Name() {
	case "Print", "Printf", "Println":
		return "fmt." + function.Name(), true
	default:
		return "", false
	}
}

func testForbiddenAnalyzerPrintIdentity(t *testing.T) {
	t.Helper()
	fmtPackage := types.NewPackage("fmt", "fmt")
	applicationPackage := types.NewPackage("example.com/application", "application")
	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	for name, test := range map[string]struct {
		object types.Object
		want   bool
	}{
		"fmt Print":          {object: types.NewFunc(token.NoPos, fmtPackage, "Print", signature), want: true},
		"fmt Printf":         {object: types.NewFunc(token.NoPos, fmtPackage, "Printf", signature), want: true},
		"fmt Println":        {object: types.NewFunc(token.NoPos, fmtPackage, "Println", signature), want: true},
		"fmt Fprintf":        {object: types.NewFunc(token.NoPos, fmtPackage, "Fprintf", signature)},
		"application Printf": {object: types.NewFunc(token.NoPos, applicationPackage, "Printf", signature)},
		"non-function":       {object: types.NewVar(token.NoPos, fmtPackage, "Print", types.Typ[types.String])},
	} {
		t.Run(name, func(t *testing.T) {
			_, got := forbiddenAnalyzerPrint(test.object)
			if got != test.want {
				t.Fatalf("forbiddenAnalyzerPrint() = %t, want %t", got, test.want)
			}
		})
	}
}
