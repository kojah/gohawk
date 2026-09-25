package resourcelifetime

import (
	"go/ast"
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ssa"
)

// This file turns a missing-release proof into what a reader needs to act on
// it: the return the flow walk reached with the resource still owed, and the
// variable that holds the resource. The witness is the proof's own, not a
// second analysis, so the report and its evidence cannot disagree.

// missingReleaseEvidence cites the leaking return, labeled with the variable
// that holds the resource when the acquisition assigns one.
func missingReleaseEvidence(pass *analysis.Pass, call *ssa.Call, result int, leak *ssa.Return) []analysis.RelatedInformation {
	if leak == nil {
		return nil
	}
	subject := "the resource"
	if name := acquiredVariableName(pass, call.Pos(), result); name != "" {
		subject = "`" + name + "`"
	}
	if leak.Pos().IsValid() {
		return []analysis.RelatedInformation{check.Evidence(pass, leak.Pos(), "returns here without releasing "+subject)}
	}
	// A function that falls off its end has no return statement to cite.
	if end := functionEnd(leak.Parent()); end.IsValid() {
		return []analysis.RelatedInformation{{Pos: end, End: end + 1, Message: "reaches the end of the function without releasing " + subject}}
	}
	return nil
}

// acquiredVariableName returns the variable an assignment or declaration
// stores the call's result at index into, or "" when there is none.
func acquiredVariableName(pass *analysis.Pass, position token.Pos, result int) string {
	for _, file := range pass.Files {
		if position < file.Pos() || position >= file.End() {
			continue
		}
		path, _ := astutil.PathEnclosingInterval(file, position, position)
		for index, node := range path {
			if _, ok := node.(*ast.CallExpr); !ok || index+1 >= len(path) {
				continue
			}
			var names []ast.Expr
			switch parent := path[index+1].(type) {
			case *ast.AssignStmt:
				if len(parent.Rhs) == 1 {
					names = parent.Lhs
				}
			case *ast.ValueSpec:
				if len(parent.Values) == 1 {
					for _, name := range parent.Names {
						names = append(names, name)
					}
				}
			}
			if result < 0 {
				result = 0
			}
			if result < len(names) {
				if ident, ok := names[result].(*ast.Ident); ok && ident.Name != "_" {
					return ident.Name
				}
			}
			return ""
		}
	}
	return ""
}

func functionEnd(function *ssa.Function) token.Pos {
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		if syntax.Body != nil {
			return syntax.Body.Rbrace
		}
	case *ast.FuncLit:
		return syntax.Body.Rbrace
	}
	return token.NoPos
}
