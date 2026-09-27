package ssaflow

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// FunctionFile returns source file containing function.
func FunctionFile(pass *analysis.Pass, function *ssa.Function) *ast.File {
	for _, file := range pass.Files {
		if file.Pos() <= function.Pos() && function.Pos() <= file.End() {
			return file
		}
	}
	return nil
}
