package globalstate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/analysis"
)

// Immutability evidence proves when a map or slice global is effectively
// read-only despite its mutable Go type. Every reachable alias and use must be
// recognized as non-mutating; uncertain calls or escaping values end the proof.

func effectivelyImmutableComposite(
	pass *analysis.Pass,
	name *ast.Ident,
	object types.Object,
	specification *ast.ValueSpec,
	index int,
	usage globalStateUsage,
) bool {
	if name.IsExported() || index >= len(specification.Values) {
		return false
	}
	initializer := specification.Values[index]
	if _, ok := initializer.(*ast.CompositeLit); !ok && !immutableCollectionConversion(pass, initializer) {
		return false
	}
	var element types.Type
	switch underlying := object.Type().Underlying().(type) {
	case *types.Map:
		if !deeplyImmutableGlobalValue(underlying.Key(), map[types.Type]bool{}) {
			return false
		}
		element = underlying.Elem()
	case *types.Slice:
		element = underlying.Elem()
	default:
		return false
	}
	if pass.TypesInfo.TypeOf(initializer) == nil {
		return false
	}
	// An unexported composite literal whose every use is a direct read has no
	// mutation or escaping alias for callers to exploit. Network Doctor uses
	// this shape for stable operating-system lookup data:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/diagnostic/route.go#L192-L200
	sawUse := false
	for _, file := range usage.files {
		immutable := true
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[identifier] != object {
				return true
			}
			sawUse = true
			if !readOnlyCollectionUse(pass, identifier, usage, map[types.Object]bool{object: true}) {
				immutable = false
				return false
			}
			return true
		})
		if !immutable {
			return false
		}
	}
	return deeplyImmutableGlobalValue(element, map[types.Type]bool{}) || sawUse
}

func immutableCollectionConversion(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || pass.TypesInfo.Types[call.Args[0]].Value == nil {
		return false
	}
	slice, ok := pass.TypesInfo.TypeOf(call).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	element, ok := slice.Elem().Underlying().(*types.Basic)
	// A conversion of a constant string to []byte/[]rune creates fresh backing
	// storage during package initialization. The ordinary read-only-use proof
	// below then establishes that no mutable alias escapes.
	return ok && (element.Kind() == types.Byte || element.Kind() == types.Rune)
}

func deeplyImmutableGlobalValue(value types.Type, seen map[types.Type]bool) bool {
	if value == nil || seen[value] {
		return value != nil
	}
	seen[value] = true
	switch underlying := value.Underlying().(type) {
	case *types.Basic:
		return true
	case *types.Signature:
		// A function value stored inside a read-only collection is itself
		// immutable. This differs from a package variable of function type,
		// whose slot can be replaced and is still reported unless it is a
		// documented test seam.
		return true
	case *types.Array:
		return deeplyImmutableGlobalValue(underlying.Elem(), seen)
	case *types.Struct:
		for field := range underlying.Fields() {
			if !deeplyImmutableGlobalValue(field.Type(), seen) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func readOnlyCollectionUse(pass *analysis.Pass, value ast.Expr, usage globalStateUsage, seen map[types.Object]bool) bool {
	var current ast.Node = value
	parent := usage.parents[current]
	for {
		if _, ok := parent.(*ast.ParenExpr); !ok {
			break
		}
		current = parent
		parent = usage.parents[current]
	}
	switch typed := parent.(type) {
	case *ast.IndexExpr:
		return readOnlyIndexUse(pass, typed, current, usage, seen)
	case *ast.RangeStmt:
		return typed.X == current && collectionElementsDeeplyImmutable(pass.TypesInfo.TypeOf(value))
	case *ast.CallExpr:
		return readOnlyCallUse(pass, typed, current, value, usage, seen)
	case *ast.BinaryExpr:
		return typed.Op == token.EQL || typed.Op == token.NEQ
	default:
		return false
	}
}

func readOnlyIndexUse(
	pass *analysis.Pass,
	index *ast.IndexExpr,
	current ast.Node,
	usage globalStateUsage,
	seen map[types.Object]bool,
) bool {
	if index.X != current || globalIndexIsWritten(index, usage.parents) {
		return false
	}
	if deeplyImmutableGlobalValue(collectionIndexValueType(pass.TypesInfo.TypeOf(index)), map[types.Type]bool{}) {
		return true
	}
	// Follow a nested mutable value through the expression that consumes it.
	// This proves cloning m[key] without accepting a returned or assigned alias.
	return readOnlyCollectionUse(pass, index, usage, seen)
}

func readOnlyCallUse(
	pass *analysis.Pass,
	call *ast.CallExpr,
	current ast.Node,
	value ast.Expr,
	usage globalStateUsage,
	seen map[types.Object]bool,
) bool {
	if collectionElementsDeeplyImmutable(pass.TypesInfo.TypeOf(value)) &&
		(readOnlyCollectionBuiltin(pass, call, current) || readOnlyCollectionPackageCall(pass, call)) {
		return true
	}
	argument := collectionArgumentIndex(call, current)
	parameters := usage.calleeParams[calledObject(pass, call.Fun)]
	if argument < 0 || argument >= len(parameters) || parameters[argument] == nil || seen[parameters[argument]] {
		return false
	}
	nextSeen := make(map[types.Object]bool, len(seen)+1)
	for object := range seen {
		nextSeen[object] = true
	}
	nextSeen[parameters[argument]] = true
	return collectionObjectReadOnly(pass, parameters[argument], usage, nextSeen)
}

func collectionIndexValueType(value types.Type) types.Type {
	if tuple, ok := value.(*types.Tuple); ok && tuple.Len() > 0 {
		return tuple.At(0).Type()
	}
	return value
}

func collectionElementsDeeplyImmutable(value types.Type) bool {
	if value == nil {
		return false
	}
	var element types.Type
	switch underlying := value.Underlying().(type) {
	case *types.Map:
		element = underlying.Elem()
	case *types.Slice:
		element = underlying.Elem()
	case *types.Array:
		element = underlying.Elem()
	default:
		return false
	}
	return deeplyImmutableGlobalValue(element, map[types.Type]bool{})
}

func collectionObjectReadOnly(pass *analysis.Pass, object types.Object, usage globalStateUsage, seen map[types.Object]bool) bool {
	for _, file := range usage.files {
		readOnly := true
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[identifier] != object {
				return true
			}
			if !readOnlyCollectionUse(pass, identifier, usage, seen) {
				readOnly = false
				return false
			}
			return true
		})
		if !readOnly {
			return false
		}
	}
	return true
}

func readOnlyCollectionBuiltin(pass *analysis.Pass, call *ast.CallExpr, target ast.Node) bool {
	switch {
	case analysisutil.IsCallToAny(pass, call, analysisutil.Builtin("len"), analysisutil.Builtin("cap")):
		return true
	case analysisutil.IsCallTo(pass, call, analysisutil.Builtin("append")):
		// Reading a global through append's variadic inputs copies its
		// elements; using it as the destination may mutate its backing array.
		return collectionArgumentIndex(call, target) > 0
	case analysisutil.IsCallTo(pass, call, analysisutil.Builtin("copy")):
		return collectionArgumentIndex(call, target) == 1
	default:
		return false
	}
}

func readOnlyCollectionPackageCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, symbol := range []analysisutil.Symbol{
		analysisutil.PackageFunction("slices", "Contains"),
		analysisutil.PackageFunction("slices", "ContainsFunc"),
		analysisutil.PackageFunction("slices", "Index"),
		analysisutil.PackageFunction("slices", "IndexFunc"),
		analysisutil.PackageFunction("slices", "Equal"),
		analysisutil.PackageFunction("slices", "EqualFunc"),
		analysisutil.PackageFunction("slices", "Compare"),
		analysisutil.PackageFunction("slices", "CompareFunc"),
		analysisutil.PackageFunction("slices", "IsSorted"),
		analysisutil.PackageFunction("slices", "IsSortedFunc"),
		analysisutil.PackageFunction("slices", "Clone"),
		analysisutil.PackageFunction("strings", "Join"),
		analysisutil.PackageFunction("sort", "SearchStrings"),
		analysisutil.PackageFunction("bytes", "Equal"),
		analysisutil.PackageFunction("bytes", "HasPrefix"),
		analysisutil.PackageFunction("bytes", "HasSuffix"),
		analysisutil.PackageFunction("maps", "Clone"),
		analysisutil.PackageFunction("maps", "Equal"),
		analysisutil.PackageFunction("maps", "EqualFunc"),
		analysisutil.PackageFunction("maps", "Keys"),
		analysisutil.PackageFunction("maps", "Values"),
		analysisutil.PackageFunction("maps", "All"),
	} {
		if analysisutil.IsCallTo(pass, call, symbol) {
			return true
		}
	}
	return false
}

func collectionArgumentIndex(call *ast.CallExpr, target ast.Node) int {
	for index, argument := range call.Args {
		if argument == target {
			return index
		}
		var current ast.Node = argument
		for {
			parenthesized, ok := current.(*ast.ParenExpr)
			if !ok {
				break
			}
			current = parenthesized.X
		}
		if current == target {
			return index
		}
	}
	return -1
}

func calledObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch function := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		return pass.TypesInfo.ObjectOf(function.Sel)
	default:
		return nil
	}
}

func globalCalleeParameters(pass *analysis.Pass) map[types.Object][]types.Object {
	// Parameter objects connect a global argument to uses inside local helpers.
	// Only declarations and directly assigned function literals have a stable
	// source body; dynamic function values intentionally stop the read-only proof.
	result := make(map[types.Object][]types.Object)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncDecl:
				if object := pass.TypesInfo.Defs[typed.Name]; object != nil {
					result[object] = globalParameterObjects(pass, typed.Type.Params)
				}
			case *ast.AssignStmt:
				for index, left := range typed.Lhs {
					name, ok := left.(*ast.Ident)
					if !ok || index >= len(typed.Rhs) {
						continue
					}
					literal, ok := typed.Rhs[index].(*ast.FuncLit)
					if ok && pass.TypesInfo.Defs[name] != nil {
						result[pass.TypesInfo.Defs[name]] = globalParameterObjects(pass, literal.Type.Params)
					}
				}
			case *ast.ValueSpec:
				for index, name := range typed.Names {
					if index >= len(typed.Values) {
						continue
					}
					literal, ok := typed.Values[index].(*ast.FuncLit)
					if ok && pass.TypesInfo.Defs[name] != nil {
						result[pass.TypesInfo.Defs[name]] = globalParameterObjects(pass, literal.Type.Params)
					}
				}
			}
			return true
		})
	}
	return result
}

func globalParameterObjects(pass *analysis.Pass, fields *ast.FieldList) []types.Object {
	if fields == nil {
		return nil
	}
	var result []types.Object
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			result = append(result, nil)
			continue
		}
		for _, name := range field.Names {
			result = append(result, pass.TypesInfo.Defs[name])
		}
	}
	return result
}

func globalIndexIsWritten(index *ast.IndexExpr, parents map[ast.Node]ast.Node) bool {
	var current ast.Node = index
	for {
		parent := parents[current]
		switch typed := parent.(type) {
		case *ast.ParenExpr, *ast.SelectorExpr, *ast.IndexExpr:
			current = parent
			continue
		case *ast.AssignStmt:
			for _, expression := range typed.Lhs {
				if expression == current {
					return true
				}
			}
		case *ast.IncDecStmt:
			return typed.X == current
		case *ast.UnaryExpr:
			return typed.Op == token.AND
		}
		return false
	}
}

func globalSyntaxParents(files []*ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			if len(stack) > 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			return true
		})
	}
	return parents
}
