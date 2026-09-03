package architecture

import (
	"go/ast"
	"path"
	"strings"
	"testing"
)

// A transparent form says that one SSA wrapper may be looked through. Which
// wrappers are sound to follow depends on what is being proved: an ownership
// proof may follow a type assertion, because the assertion selects the same
// object, while a proof about which methods a value has must stop there.
//
// Every caller therefore names the forms it wants. Passing a mask assembled
// elsewhere would make a later form apply to proofs nobody reviewed for it,
// and the widening would be silent: no test fails when a proof quietly starts
// following one more wrapper. This test keeps the choice at the call site.

// transparentFormParameters are the functions whose forms argument selects
// which wrappers a proof may look through.
var transparentFormParameters = map[string]int{
	"UnwrapTransparentValue": 1,
	"NewReachingWalk":        0,
}

func TestTransparentFormsAreNamedAtTheCallSite(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	sources := inventory.productionGoFiles(t, "internal")
	// A named mask declared beside the proof, such as contextForms, is the
	// preferred shape: it is explicit and can be documented once. It is
	// declared per package, not per file, so collect the whole package first.
	declared := map[string]map[string]bool{}
	for _, source := range sources {
		directory := path.Dir(source.repositoryPath)
		if declared[directory] == nil {
			declared[directory] = map[string]bool{}
		}
		for name := range transparentFormDeclarations(source.file) {
			declared[directory][name] = true
		}
	}
	for _, source := range sources {
		named := declared[path.Dir(source.repositoryPath)]
		for _, function := range source.file.Decls {
			declaration, ok := function.(*ast.FuncDecl)
			if !ok || declaration.Body == nil {
				continue
			}
			checkTransparentFormArguments(t, source, declaration, named)
		}
	}
}

func checkTransparentFormArguments(
	t *testing.T,
	source productionGoSource,
	function *ast.FuncDecl,
	named map[string]bool,
) {
	t.Helper()
	locals := transparentFormLocals(function.Body)
	for name := range named {
		locals[name] = true
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		index, ok := transparentFormParameters[calleeName(call.Fun)]
		if !ok || index >= len(call.Args) {
			return true
		}
		if namesTransparentForms(call.Args[index], locals) {
			return true
		}
		position := source.fileSet.Position(call.Args[index].Pos())
		t.Errorf("%s:%d passes transparent forms that are not named here; write the Transparent... constants this "+
			"proof may follow, so a form added later does not widen it silently",
			source.repositoryPath, position.Line)
		return true
	})
}

// transparentFormDeclarations returns the file-level names declared as an
// explicit union of transparent forms.
func transparentFormDeclarations(file *ast.File) map[string]bool {
	declared := map[string]bool{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range group.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range value.Names {
				if index < len(value.Values) && namesTransparentForms(value.Values[index], nil) {
					declared[name.Name] = true
				}
			}
		}
	}
	return declared
}

// transparentFormLocals returns the local names assigned an explicit union of
// transparent forms, which callers use when the same list serves several
// queries in one function.
func transparentFormLocals(body *ast.BlockStmt) map[string]bool {
	locals := map[string]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, target := range assignment.Lhs {
			name, ok := target.(*ast.Ident)
			if !ok || index >= len(assignment.Rhs) {
				continue
			}
			if namesTransparentForms(assignment.Rhs[index], nil) {
				locals[name.Name] = true
			}
		}
		return true
	})
	return locals
}

// namesTransparentForms reports whether the expression is a union of
// Transparent... constants, or a local holding one.
func namesTransparentForms(expression ast.Expr, locals map[string]bool) bool {
	switch typed := expression.(type) {
	case *ast.BinaryExpr:
		return namesTransparentForms(typed.X, locals) && namesTransparentForms(typed.Y, locals)
	case *ast.ParenExpr:
		return namesTransparentForms(typed.X, locals)
	case *ast.SelectorExpr:
		// A walk built with a checked mask may forward it: the choice was made,
		// and checked, where the walk was constructed.
		return strings.HasPrefix(typed.Sel.Name, "Transparent") || typed.Sel.Name == "forms"
	case *ast.Ident:
		return strings.HasPrefix(typed.Name, "Transparent") || locals[typed.Name]
	}
	return false
}

func calleeName(function ast.Expr) string {
	switch typed := function.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	}
	return ""
}
