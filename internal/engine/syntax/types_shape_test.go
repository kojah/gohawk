package syntax_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/engine/syntax"
)

func TestPointerStructTypeBoundaries(t *testing.T) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "shapes.go", `package shapes
type Model struct { value int }
type ModelAlias = Model
type Pointer *Model
type PointerAlias = Pointer
type AliasedElement *ModelAlias
type Anonymous *struct { value int }
type Nested **Model
type Scalar *int
type Slice []Model
type Interface interface { Read() }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	config := types.Config{}
	pkg, err := config.Check("shapes", set, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		want bool
	}{
		{"Model", false},
		{"ModelAlias", false},
		{"Pointer", true},
		{"PointerAlias", true},
		{"AliasedElement", true},
		{"Anonymous", true},
		{"Nested", false},
		{"Scalar", false},
		{"Slice", false},
		{"Interface", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			structure := syntax.PointerStruct(pkg.Scope().Lookup(test.name).Type())
			if (structure != nil) != test.want {
				t.Fatalf("PointerStruct() = %v, want struct %t", structure, test.want)
			}
			if structure != nil && (structure.NumFields() != 1 || structure.Field(0).Name() != "value") {
				t.Error("lost the selected struct's field information")
			}
		})
	}
	if syntax.PointerStruct(nil) != nil {
		t.Error("missing type supplied a struct")
	}
}
