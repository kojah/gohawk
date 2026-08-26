package checkutil

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// BuiltinClose names Go's channel-closing builtin.
const BuiltinClose = "close"

// GeneratedFile reports whether file carries Go's generated-file marker.
func GeneratedFile(file *ast.File) bool {
	return ast.IsGenerated(file)
}

// FunctionSymbol identifies one package-level Go function.
type FunctionSymbol struct {
	Package string
	Name    string
}

// IsPackageCall reports whether call statically names symbol.
func IsPackageCall(pass *analysis.Pass, call *ast.CallExpr, symbol FunctionSymbol) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != symbol.Name {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	imported, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
	return ok && imported.Imported().Path() == symbol.Package
}

// IsErrorType reports whether value implements Go's predeclared error interface.
func IsErrorType(value types.Type) bool {
	if value == nil {
		return false
	}
	errorType, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return ok && types.Implements(value, errorType)
}

// IsStringType reports whether value has string as its underlying type.
func IsStringType(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

// NamedType reports whether value names packagePath.name, allowing one pointer layer.
func NamedType(value types.Type, packagePath, name string) bool {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}
