package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Type checking resolves aliases and inferred declarations in the current build.
// The syntax gate still covers authored files excluded by the active build tags.
func TestNoTypedStringEnums(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	authored := make(map[string]string)
	for _, source := range inventory.authoredGoFiles(t, ".") {
		authored[source.absolutePath] = source.repositoryPath
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root, Tests: true,
	}, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if count := packages.PrintErrors(loaded); count > 0 {
		t.Fatalf("load authored packages: %d errors", count)
	}
	seen := make(map[token.Position]bool)
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			if index >= len(pkg.CompiledGoFiles) {
				continue
			}
			path, ok := authored[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if !typedStringEnum(node, pkg.TypesInfo) {
					return true
				}
				position := pkg.Fset.Position(node.Pos())
				if !seen[position] {
					t.Errorf("%s:%d: classification domains must use numeric enums", path, position.Line)
					seen[position] = true
				}
				return true
			})
		}
	}
}

func typedStringEnum(node ast.Node, info *types.Info) bool {
	switch node := node.(type) {
	case *ast.FuncType:
		return classificationParameter(node, func(expression ast.Expr) bool { return stringUnderlying(info.TypeOf(expression)) })
	case *ast.TypeSpec:
		return classificationDomainName(node.Name.Name) && stringUnderlying(info.TypeOf(node.Name))
	case *ast.Field:
		for _, name := range node.Names {
			if strings.EqualFold(name.Name, "kind") && stringUnderlying(info.TypeOf(node.Type)) {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, name := range node.Names {
			if rawTypedKind(name, info) {
				return true
			}
		}
	case *ast.AssignStmt:
		for _, left := range node.Lhs {
			if name, ok := left.(*ast.Ident); ok && rawTypedKind(name, info) {
				return true
			}
		}
	case *ast.RangeStmt:
		for _, expression := range []ast.Expr{node.Key, node.Value} {
			if name, ok := expression.(*ast.Ident); ok && rawTypedKind(name, info) {
				return true
			}
		}
	}
	return false
}

func rawTypedKind(name *ast.Ident, info *types.Info) bool {
	return strings.EqualFold(name.Name, "kind") && stringUnderlying(info.TypeOf(name))
}

func stringUnderlying(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

func TestTypedStringEnumMatcher(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         bool
	}{
		{"local defined domain", `type text string; type CheckKind text`, true},
		{"local alias domain", `type text = string; type CheckMode = text`, true},
		{"imported string domain", `type CheckState = wire.Text`, true},
		{"aliased field", `type item struct{ Kind wire.Text }`, true},
		{"aliased phase parameter", `func f(phase wire.Text){}`, true},
		{"aliased mode parameter", `func f(mode wire.Text){}`, true},
		{"serialized mode field", `type record struct{Mode wire.Text}`, false},
		{"numeric mode parameter", `func f(mode wire.Number){}`, false},
		{"serialized phase field", `type record struct{Phase wire.Text}`, false},
		{"indirect phase domain", `type QueryPhase wire.Text`, true},
		{"aliased parameter", `func f(kind wire.Text){}`, true},
		{"aliased result", `func f()(kind wire.Text){return ""}`, true},
		{"inferred variable", `func text()string{return ""}; var kind = text()`, true},
		{"inferred assignment", `func text()string{return ""}; func f(){kind:=text();_=kind}`, true},
		{"tuple assignment", `func text()(string,int){return "",0}; func f(){kind,n:=text();_,_=kind,n}`, true},
		{"range declaration", `func f(){for _,kind:=range []string{"a"}{_=kind}}`, true},
		{"numeric import", `type CheckKind = wire.Number; func f(kind CheckKind){}`, false},
		{"numeric inferred", `func number()wire.Number{return 0}; func f(){kind:=number();_=kind}`, false},
		{"text identity", `type AnalyzerID wire.Text; func f(prefix wire.Text){}`, false},
		{"fixture text", `const fixture="type Outcome string"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fileSet := token.NewFileSet()
			source := "package fixture\n"
			if strings.Contains(test.source, "wire.") {
				source += "import \"wire\"\n"
			}
			file, err := parser.ParseFile(fileSet, "fixture.go", source+test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{
				Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object),
			}
			config := types.Config{Importer: enumFixtureImporter{}}
			if _, err := config.Check("fixture", fileSet, []*ast.File{file}, info); err != nil {
				t.Fatal(err)
			}
			found := false
			ast.Inspect(file, func(node ast.Node) bool {
				found = found || typedStringEnum(node, info)
				return true
			})
			if found != test.want {
				t.Errorf("typedStringEnum() = %t, want %t", found, test.want)
			}
		})
	}
}

type enumFixtureImporter struct{}

func (enumFixtureImporter) Import(path string) (*types.Package, error) {
	pkg := types.NewPackage(path, "wire")
	for name, basic := range map[string]*types.Basic{"Text": types.Typ[types.String], "Number": types.Typ[types.Uint8]} {
		object := types.NewTypeName(token.NoPos, pkg, name, nil)
		types.NewNamed(object, basic, nil)
		pkg.Scope().Insert(object)
	}
	pkg.MarkComplete()
	return pkg, nil
}
