package syntax

import (
	"go/ast"
	"go/parser"
	"testing"
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
