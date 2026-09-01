package architecture

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestProductionCodeReturnsTerminationDecisions(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	production := make(map[string]string)
	for _, source := range inventory.productionGoFiles(t, ".") {
		production[source.absolutePath] = source.repositoryPath
	}
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root,
	}
	loaded, err := packages.Load(config, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 {
		t.Fatalf("load production packages: %d errors", errors)
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
			inspectTerminationFile(t, pkg, file, repositoryPath)
		}
	}
}

func inspectTerminationFile(t *testing.T, pkg *packages.Package, file *ast.File, repositoryPath string) {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			inspectTerminationNode(t, pkg, declaration, repositoryPath, false)
			continue
		}
		commandEntryPoint := pkg.Name == "main" && function.Recv == nil && function.Name.Name == "main"
		inspectTerminationNode(t, pkg, function.Body, repositoryPath, commandEntryPoint)
	}
}

func inspectTerminationNode(t *testing.T, pkg *packages.Package, node ast.Node, repositoryPath string, commandEntryPoint bool) {
	t.Helper()
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if !ok || commandEntryPoint {
			return true
		}
		termination, forbidden := forbiddenTermination(pkg.TypesInfo.Uses[identifier])
		if !forbidden {
			return true
		}
		position := pkg.Fset.Position(identifier.Pos())
		t.Errorf("%s:%d uses %s outside a command main function; return an error or exit code", repositoryPath, position.Line, termination)
		return true
	})
}

func forbiddenTermination(object types.Object) (string, bool) {
	if builtin, ok := object.(*types.Builtin); ok {
		return builtin.Name(), builtin.Name() == "panic"
	}
	function, ok := object.(*types.Func)
	if !ok || function.Pkg() == nil {
		return "", false
	}
	packagePath, name := function.Pkg().Path(), function.Name()
	if packagePath == "os" && name == "Exit" {
		return "os.Exit", true
	}
	if packagePath == "log" {
		switch name {
		case "Fatal", "Fatalf", "Fatalln":
			return "log." + name, true
		}
	}
	return "", false
}

func TestForbiddenTerminationIdentity(t *testing.T) {
	t.Parallel()
	logPackage := types.NewPackage("log", "log")
	osPackage := types.NewPackage("os", "os")
	applicationPackage := types.NewPackage("example.com/application", "application")
	signature := types.NewSignature(nil, nil, nil, false)
	for name, test := range map[string]struct {
		object types.Object
		want   bool
	}{
		"builtin panic":     {object: types.Universe.Lookup("panic"), want: true},
		"builtin len":       {object: types.Universe.Lookup("len")},
		"log Fatal":         {object: types.NewFunc(token.NoPos, logPackage, "Fatal", signature), want: true},
		"log Fatalf":        {object: types.NewFunc(token.NoPos, logPackage, "Fatalf", signature), want: true},
		"log Fatalln":       {object: types.NewFunc(token.NoPos, logPackage, "Fatalln", signature), want: true},
		"log Println":       {object: types.NewFunc(token.NoPos, logPackage, "Println", signature)},
		"os Exit":           {object: types.NewFunc(token.NoPos, osPackage, "Exit", signature), want: true},
		"application Fatal": {object: types.NewFunc(token.NoPos, applicationPackage, "Fatal", signature)},
	} {
		t.Run(name, func(t *testing.T) {
			_, got := forbiddenTermination(test.object)
			if got != test.want {
				t.Fatalf("forbiddenTermination() = %t, want %t", got, test.want)
			}
		})
	}
}
