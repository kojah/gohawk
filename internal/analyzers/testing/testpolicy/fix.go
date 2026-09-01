package testpolicy

import (
	"go/ast"
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This file owns the conservative source edit that inserts Helper at function
// entry. It declines fixes when the handle or source body is not safe to edit.

func testHelperFix(pass *analysis.Pass, function *ssa.Function, handle *ssa.Parameter) []analysis.SuggestedFix {
	name := handle.Name()
	if name == "" || name == "_" || !token.IsIdentifier(name) || hasHelperCall(function, handle) {
		return nil
	}
	var body *ast.BlockStmt
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	}
	if body == nil {
		return nil
	}
	position, newText := body.Rbrace, []byte("\n\t"+name+".Helper()\n")
	if file := pass.Fset.File(body.Lbrace); file != nil {
		braceLine := file.Line(body.Lbrace)
		if file.Line(body.Rbrace) > braceLine && braceLine < file.LineCount() {
			position = file.LineStart(braceLine + 1)
			newText = []byte("\t" + name + ".Helper()\n")
		}
	}
	return []analysis.SuggestedFix{{
		Message: "Call " + name + ".Helper() at function entry",
		TextEdits: []analysis.TextEdit{{
			Pos:     position,
			NewText: newText,
		}},
	}}
}

func hasHelperCall(function *ssa.Function, handle *ssa.Parameter) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if ssaflow.CallName(common) == "Helper" && ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), handle, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}
