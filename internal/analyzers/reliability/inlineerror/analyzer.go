// Package inlineerror implements the inlineerror gohawk analyzer.
package inlineerror

import (
	"go/ast"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "inlineerror", Doc: "checks inline error declarations for mismatched conditions",
		Requires: []*analysis.Analyzer{inspect.Analyzer}, Run: runInlineError,
	}
}

func runInlineError(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.IfStmt)(nil)}, func(node ast.Node) {
		statement := node.(*ast.IfStmt)
		assignment, ok := statement.Init.(*ast.AssignStmt)
		if !ok || assignment.Tok.String() != ":=" {
			return
		}
		for _, expression := range assignment.Lhs {
			fresh, ok := expression.(*ast.Ident)
			if !ok || pass.TypesInfo.Defs[fresh] == nil || !syntax.IsErrorType(pass.TypesInfo.TypeOf(fresh)) {
				continue
			}
			freshObject := pass.TypesInfo.ObjectOf(fresh)
			if syntax.ExpressionUsesObject(pass, statement.Cond, freshObject) || !returnsOnlyObject(pass, statement.Body, freshObject) {
				continue
			}
			var mismatched *ast.Ident
			ast.Inspect(statement.Cond, func(candidate ast.Node) bool {
				identifier, ok := candidate.(*ast.Ident)
				if ok && pass.TypesInfo.ObjectOf(identifier) != freshObject && syntax.IsErrorType(pass.TypesInfo.TypeOf(identifier)) {
					mismatched = identifier
					return false
				}
				return true
			})
			if mismatched != nil {
				check.Reportf(
					pass, check.ErrorMismatchedInline, mismatched.Pos(),
					"condition checks %s instead of newly declared %s", mismatched.Name, fresh.Name,
				)
			}
		}
	})
	return nil, nil
}

func returnsOnlyObject(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	returned, ok := body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && syntax.ExpressionUsesObject(pass, returned.Results[0], object)
}
