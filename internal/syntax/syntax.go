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

// FunctionParameterObject returns the declared object at the positional
// parameter index. An unnamed parameter occupies a position but has no object.
func FunctionParameterObject(pass *analysis.Pass, function *ast.FuncDecl, target int) types.Object {
	if pass == nil || function == nil || function.Type.Params == nil || target < 0 {
		return nil
	}
	position := 0
	for _, field := range function.Type.Params.List {
		count := max(1, len(field.Names))
		if target >= position+count {
			position += count
			continue
		}
		if len(field.Names) == 0 {
			return nil
		}
		return pass.TypesInfo.Defs[field.Names[target-position]]
	}
	return nil
}
