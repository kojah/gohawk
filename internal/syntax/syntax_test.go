package syntax

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestUnparen(t *testing.T) {
	expression, err := parser.ParseExpr("(((value)))")
	if err != nil {
		t.Fatal(err)
	}
	identifier, ok := Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name != "value" {
		t.Fatalf("Unparen() = %T, want identifier value", Unparen(expression))
	}
}

func TestFunctionParameterObject(t *testing.T) {
	first := ast.NewIdent("first")
	second := ast.NewIdent("second")
	third := ast.NewIdent("third")
	objects := map[*ast.Ident]types.Object{
		first:  types.NewVar(token.NoPos, nil, first.Name, types.Typ[types.Int]),
		second: types.NewVar(token.NoPos, nil, second.Name, types.Typ[types.Int]),
		third:  types.NewVar(token.NoPos, nil, third.Name, types.Typ[types.Int]),
	}
	function := &ast.FuncDecl{Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{first, second}},
		{Type: ast.NewIdent("string")},
		{Names: []*ast.Ident{third}},
	}}}}
	pass := &analysis.Pass{TypesInfo: &types.Info{Defs: objects}}

	for _, test := range []struct {
		index int
		want  types.Object
	}{
		{index: 0, want: objects[first]},
		{index: 1, want: objects[second]},
		{index: 2},
		{index: 3, want: objects[third]},
		{index: 4},
	} {
		if got := FunctionParameterObject(pass, function, test.index); got != test.want {
			t.Errorf("FunctionParameterObject(..., %d) = %v, want %v", test.index, got, test.want)
		}
	}
}
