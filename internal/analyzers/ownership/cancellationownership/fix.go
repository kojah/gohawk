package cancellationownership

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

func cancellationFix(pass *analysis.Pass, callPosition token.Pos, constructor string) []analysis.SuggestedFix {
	for _, file := range pass.Files {
		var fix []analysis.SuggestedFix
		ast.Inspect(file, func(node ast.Node) bool {
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return fix == nil
			}
			for _, statement := range block.List {
				assignment, assignmentOK := statement.(*ast.AssignStmt)
				if !assignmentOK || len(assignment.Rhs) != 1 || len(assignment.Lhs) < 2 {
					continue
				}
				call, callOK := assignment.Rhs[0].(*ast.CallExpr)
				cancel, cancelOK := assignment.Lhs[1].(*ast.Ident)
				if !callOK || !cancelOK || call.Pos() != callPosition || cancel.Name == "_" {
					continue
				}
				insertAt, ok := nextLineStart(pass.Fset, assignment)
				if !ok {
					continue
				}
				fix = []analysis.SuggestedFix{{
					Message: "Defer " + cancel.Name + " immediately after creation",
					TextEdits: []analysis.TextEdit{{
						Pos:     insertAt,
						NewText: []byte("\tdefer " + cancelInvocation(cancel.Name, constructor) + "\n"),
					}},
				}}
				return false
			}
			return true
		})
		if fix != nil {
			return fix
		}
	}
	return nil
}

func nextLineStart(files *token.FileSet, node ast.Node) (token.Pos, bool) {
	file := files.File(node.End())
	if file == nil {
		return token.NoPos, false
	}
	line := file.Line(node.End())
	if line >= file.LineCount() {
		return token.NoPos, false
	}
	return file.LineStart(line + 1), true
}

func cancelInvocation(name, constructor string) string {
	if constructor == "WithCancelCause" {
		return name + "(nil)"
	}
	return name + "()"
}
