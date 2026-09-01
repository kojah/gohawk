package analysisutil

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

type symbolKind uint8

const (
	symbolPackageFunction symbolKind = iota + 1
	symbolMethod
	symbolBuiltin
	symbolPackageVariable
)

// Symbol identifies one exact Go declaration. Package functions, methods,
// builtins, and package variables are distinct so callers cannot accidentally
// treat a common method name as sufficient identity evidence.
type Symbol struct {
	kind        symbolKind
	packagePath string
	receiver    string
	name        string
}

// PackageFunction identifies a package-level function.
func PackageFunction(packagePath, name string) Symbol {
	return Symbol{kind: symbolPackageFunction, packagePath: packagePath, name: name}
}

// PackageMethod identifies a method declared on receiver in packagePath.
// Receiver is the declared named type without pointer syntax.
func PackageMethod(packagePath, receiver, name string) Symbol {
	return Symbol{kind: symbolMethod, packagePath: packagePath, receiver: receiver, name: name}
}

// Builtin identifies a predeclared Go function.
func Builtin(name string) Symbol {
	return Symbol{kind: symbolBuiltin, name: name}
}

// PackageVariable identifies a package-level variable.
func PackageVariable(packagePath, name string) Symbol {
	return Symbol{kind: symbolPackageVariable, packagePath: packagePath, name: name}
}

// MatchesObject reports whether object is the exact declaration identified by
// symbol. It uses type information rather than source spelling, so import
// aliases and promoted method selections do not affect identity.
func (symbol Symbol) MatchesObject(object types.Object) bool {
	if object == nil || object.Name() != symbol.name {
		return false
	}
	switch symbol.kind {
	case symbolBuiltin:
		_, ok := object.(*types.Builtin)
		return ok
	case symbolPackageVariable:
		variable, ok := object.(*types.Var)
		return ok && packageObject(variable, symbol.packagePath)
	case symbolPackageFunction:
		function, ok := object.(*types.Func)
		return ok && packageObject(function, symbol.packagePath) && functionReceiver(function) == nil
	case symbolMethod:
		function, ok := object.(*types.Func)
		if !ok {
			return false
		}
		receiver := functionReceiver(function)
		return namedType(receiver, symbol.packagePath, symbol.receiver)
	default:
		return false
	}
}

// MatchesMethod reports whether name is selected on the receiver identified by
// symbol. This preserves the public receiver identity for promoted methods,
// whose declaring object may belong to an embedded implementation type.
func (symbol Symbol) MatchesMethod(name string, receiver types.Type) bool {
	return symbol.kind == symbolMethod && symbol.name == name && namedType(receiver, symbol.packagePath, symbol.receiver)
}

// IsCallTo reports whether call statically resolves to symbol.
func IsCallTo(pass *analysis.Pass, call *ast.CallExpr, symbol Symbol) bool {
	if pass == nil || call == nil {
		return false
	}
	if symbol.MatchesObject(calledObject(pass, call.Fun)) {
		return true
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && symbol.MatchesMethod(selection.Obj().Name(), selection.Recv())
}

func calledObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch typed := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.Uses[typed]
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[typed]; selection != nil {
			return selection.Obj()
		}
		return pass.TypesInfo.Uses[typed.Sel]
	default:
		return nil
	}
}

func packageObject(object types.Object, packagePath string) bool {
	return object.Pkg() != nil && object.Pkg().Path() == packagePath && object.Parent() == object.Pkg().Scope()
}

func functionReceiver(function *types.Func) types.Type {
	signature, ok := function.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return nil
	}
	return signature.Recv().Type()
}

func namedType(value types.Type, packagePath, name string) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath && named.Obj().Name() == name
}
