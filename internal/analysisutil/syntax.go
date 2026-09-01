package analysisutil

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// ParameterTypes expands a field list into one entry per declared parameter.
func ParameterTypes(pass *analysis.Pass, fields *ast.FieldList) []types.Type {
	if fields == nil {
		return nil
	}
	var result []types.Type
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		for range count {
			result = append(result, pass.TypesInfo.TypeOf(field.Type))
		}
	}
	return result
}

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

// SameExpression reports whether two expressions identify the same syntactic
// value, using type information to distinguish identifiers with equal names.
func SameExpression(pass *analysis.Pass, first, second ast.Expr) bool {
	switch left := first.(type) {
	case *ast.Ident:
		right, ok := second.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(left) == pass.TypesInfo.ObjectOf(right)
	case *ast.SelectorExpr:
		right, ok := second.(*ast.SelectorExpr)
		return ok && left.Sel.Name == right.Sel.Name && SameExpression(pass, left.X, right.X)
	case *ast.IndexExpr:
		right, ok := second.(*ast.IndexExpr)
		return ok && SameExpression(pass, left.X, right.X) && SameExpression(pass, left.Index, right.Index)
	case *ast.ParenExpr:
		right, ok := second.(*ast.ParenExpr)
		return ok && SameExpression(pass, left.X, right.X)
	case *ast.StarExpr:
		right, ok := second.(*ast.StarExpr)
		return ok && SameExpression(pass, left.X, right.X)
	case *ast.BasicLit:
		right, ok := second.(*ast.BasicLit)
		return ok && left.Kind == right.Kind && left.Value == right.Value
	default:
		return false
	}
}
