package syntax

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ExpressionUsesObject reports whether node refers to object.
func ExpressionUsesObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	used := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == object {
			used = true
			return false
		}
		return true
	})
	return used
}

// Unparen removes every enclosing parenthesized expression.
func Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}
