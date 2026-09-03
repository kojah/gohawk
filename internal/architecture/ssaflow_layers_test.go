package architecture

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// ssaflowFamilies names the layers inside internal/ssaflow from lowest to
// highest. A file's family is the prefix of its name, and a file may
// reference package-level declarations only from its own family or a lower
// one. The package stays one Go package, so nothing needs exporting to
// cross a family; this test is what keeps the layering real.
var ssaflowFamilies = []string{"proof", "value", "call", "flow", "store", "completion", "evidence"}

// TestSSAFlowFamiliesLayerDownward keeps the file families of
// internal/ssaflow acyclic: the proof types sit under value provenance,
// which sits under call metadata and symbol matching, control flow, storage
// and transfer evidence, the completion search, and finally the memoizing
// evidence wrappers.
func TestSSAFlowFamiliesLayerDownward(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  inventory.root,
	}
	loaded, err := packages.Load(config, "./internal/ssaflow")
	if err != nil {
		t.Fatal(err)
	}
	if errors := packages.PrintErrors(loaded); errors > 0 || len(loaded) != 1 {
		t.Fatalf("load internal/ssaflow: %d packages, %d errors", len(loaded), errors)
	}
	pkg := loaded[0]
	familyOf := make(map[string]int, len(pkg.CompiledGoFiles))
	for _, path := range pkg.CompiledGoFiles {
		family := ssaflowFamily(t, filepath.Base(path))
		familyOf[path] = family
	}
	for index, file := range pkg.Syntax {
		path := pkg.CompiledGoFiles[index]
		using := familyOf[path]
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := pkg.TypesInfo.Uses[identifier]
			if object == nil || object.Pkg() != pkg.Types || !packageLevel(object) {
				return true
			}
			defined := pkg.Fset.Position(object.Pos()).Filename
			definedFamily, known := familyOf[defined]
			if !known || definedFamily <= using {
				return true
			}
			t.Errorf("%s (%s) references %s from the higher %s family",
				filepath.Base(path), ssaflowFamilies[using], object.Name(), ssaflowFamilies[definedFamily])
			return true
		})
	}
}

// ssaflowFamily maps a file name to its family index by prefix. doc.go
// belongs to no family and is the lowest so it may be referenced by all.
func ssaflowFamily(t *testing.T, name string) int {
	t.Helper()
	if name == "doc.go" {
		return 0
	}
	prefix, _, _ := strings.Cut(name, "_")
	index := slices.Index(ssaflowFamilies, prefix)
	if index < 0 {
		t.Fatalf("internal/ssaflow/%s does not start with a family prefix (%s)", name, strings.Join(ssaflowFamilies, ", "))
	}
	return index
}

// packageLevel reports whether object is declared at package scope rather
// than as a local, parameter, field, or method.
func packageLevel(object types.Object) bool {
	return object.Parent() != nil && object.Parent().Parent() == types.Universe
}
