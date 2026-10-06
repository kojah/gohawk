package syntax

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestCallReceiverText(t *testing.T) {
	files := token.NewFileSet()
	source := "package p\nfunc f() { l.mu.Lock(); (c.index).RLock(); unlock() }\n"
	file, err := parser.ParseFile(files, "example.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: files, Files: []*ast.File{file}}
	var got []string
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			// SSA positions a call at its left parenthesis.
			got = append(got, CallReceiverText(pass, call.Lparen))
		}
		return true
	})
	want := []string{"l.mu", "(c.index)", ""}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("call %d: got %q, want %q", index, got[index], want[index])
		}
	}
	if text := CallReceiverText(pass, token.NoPos); text != "" {
		t.Errorf("a position outside the files named %q", text)
	}
}
