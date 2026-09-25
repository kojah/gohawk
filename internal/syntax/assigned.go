package syntax

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"
)

// AssignedName returns the variable an assignment or declaration stores the
// result at index of the call at position into, such as f in
// `f, err := os.Open(path)`, or "" when the result is not assigned to a
// named variable. Diagnostics use it to name the value they report on.
func AssignedName(pass *analysis.Pass, position token.Pos, index int) string {
	for _, file := range pass.Files {
		if position < file.Pos() || position >= file.End() {
			continue
		}
		path, _ := astutil.PathEnclosingInterval(file, position, position)
		for depth, node := range path {
			if _, ok := node.(*ast.CallExpr); !ok || depth+1 >= len(path) {
				continue
			}
			return assignedName(path[depth+1], max(index, 0))
		}
	}
	return ""
}

func assignedName(parent ast.Node, index int) string {
	var names []ast.Expr
	switch parent := parent.(type) {
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
	if index >= len(names) {
		return ""
	}
	if ident, ok := names[index].(*ast.Ident); ok && ident.Name != "_" {
		return ident.Name
	}
	return ""
}
