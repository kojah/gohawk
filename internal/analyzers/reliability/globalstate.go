package reliability

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func globalStateAnalyzer() *analysis.Analyzer {
	config := globalStateConfig{}
	analyzer := &analysis.Analyzer{
		Name: "globalstate",
		Doc:  "checks mutable package-level state",
	}
	analyzer.Flags.StringVar(&config.allowNames, "allow-names", "", "comma-separated package variable names to allow")
	analyzer.Flags.StringVar(&config.allowTypes, "allow-types", "", "comma-separated fully-qualified named types to allow")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runGlobalState(pass, config)
	}
	return analyzer
}

type globalStateConfig struct {
	allowNames string
	allowTypes string
}

type globalStateAllowlist struct {
	names map[string]bool
	types map[string]bool
}

type globalStateUsage struct {
	files        []*ast.File
	parents      map[ast.Node]ast.Node
	calleeParams map[types.Object][]types.Object
}

func runGlobalState(pass *analysis.Pass, config globalStateConfig) (any, error) {
	allowlist := globalStateAllowlist{names: commaSeparatedSet(config.allowNames), types: commaSeparatedSet(config.allowTypes)}
	usage := globalStateUsage{files: pass.Files, parents: globalSyntaxParents(pass.Files), calleeParams: globalCalleeParameters(pass)}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.VAR {
				continue
			}
			checkGlobalDeclaration(pass, generic, allowlist, usage)
		}
	}
	return nil, nil
}

func checkGlobalDeclaration(pass *analysis.Pass, declaration *ast.GenDecl, allowlist globalStateAllowlist, usage globalStateUsage) {
	for _, specification := range declaration.Specs {
		value, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			if name.Name == "_" {
				continue
			}
			object := pass.TypesInfo.Defs[name]
			if object == nil || !mutableGlobal(object.Type()) || allowlist.names[name.Name] || allowlist.types[qualifiedTypeName(object.Type())] || allowedGlobal(pass, name, object, declaration, value, index, usage) {
				continue
			}
			reportf(pass, checkMutableGlobalState, name.Pos(), "mutable package state %s requires an immutable owner or //gohawk:ignore globalstate", name.Name)
		}
	}
}

func qualifiedTypeName(value types.Type) string {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

func mutableGlobal(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Map, *types.Slice, *types.Pointer, *types.Interface, *types.Signature, *types.Chan:
		return true
	default:
		return false
	}
}

func allowedGlobal(pass *analysis.Pass, name *ast.Ident, object types.Object, declaration *ast.GenDecl, specification *ast.ValueSpec, index int, usage globalStateUsage) bool {
	value := object.Type()
	if strings.HasSuffix(name.Name, "Schema") || analysisutil.NamedType(value, "sync", "Once") || analysisutil.NamedType(value, "regexp", "Regexp") {
		return true
	}
	if documentedTestSeam(name, object, declaration, specification) {
		return true
	}
	if index < len(specification.Values) {
		call, ok := specification.Values[index].(*ast.CallExpr)
		if ok && analysisutil.IsPackageCall(pass, call, analysisutil.FunctionSymbol{Package: "errors", Name: "New"}) {
			return true
		}
	}
	return effectivelyImmutableComposite(pass, name, object, specification, index, usage)
}

func documentedTestSeam(name *ast.Ident, object types.Object, declaration *ast.GenDecl, specification *ast.ValueSpec) bool {
	if name.IsExported() {
		return false
	}
	switch object.Type().Underlying().(type) {
	case *types.Signature, *types.Interface:
	default:
		return false
	}
	comment := strings.ToLower(globalDeclarationComment(declaration, specification))
	if !strings.Contains(comment, "test") {
		return false
	}
	for _, contract := range []string{"fake", "override", "pin", "replace", "seam", "stub"} {
		if strings.Contains(comment, contract) {
			// A documented, unexported function or interface replacement is an
			// intentional dependency-injection boundary rather than ambient
			// application state. Network Doctor uses this contract for testable
			// OS integration:
			// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/cmd/netdoc-sim/clipboard.go#L25-L29
			return true
		}
	}
	return false
}

func globalDeclarationComment(declaration *ast.GenDecl, specification *ast.ValueSpec) string {
	var result strings.Builder
	for _, group := range []*ast.CommentGroup{declaration.Doc, specification.Doc, specification.Comment} {
		if group == nil {
			continue
		}
		result.WriteString(group.Text())
		result.WriteByte('\n')
	}
	return result.String()
}

func effectivelyImmutableComposite(pass *analysis.Pass, name *ast.Ident, object types.Object, specification *ast.ValueSpec, index int, usage globalStateUsage) bool {
	if name.IsExported() || index >= len(specification.Values) {
		return false
	}
	literal, ok := specification.Values[index].(*ast.CompositeLit)
	if !ok {
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
	if pass.TypesInfo.TypeOf(literal) == nil {
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
		for index := range underlying.NumFields() {
			if !deeplyImmutableGlobalValue(underlying.Field(index).Type(), seen) {
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
		if typed.X != current || globalIndexIsWritten(typed, usage.parents) {
			return false
		}
		if deeplyImmutableGlobalValue(collectionIndexValueType(pass.TypesInfo.TypeOf(typed)), map[types.Type]bool{}) {
			return true
		}
		// Follow a nested mutable value through the expression that consumes
		// it. This proves patterns such as cloning m[key] before returning it,
		// without treating a returned or assigned alias as read-only.
		return readOnlyCollectionUse(pass, typed, usage, seen)
	case *ast.RangeStmt:
		return typed.X == current && collectionElementsDeeplyImmutable(pass.TypesInfo.TypeOf(value))
	case *ast.CallExpr:
		if collectionElementsDeeplyImmutable(pass.TypesInfo.TypeOf(value)) && (readOnlyCollectionBuiltin(pass, typed, current) || readOnlyCollectionPackageCall(pass, typed)) {
			return true
		}
		argument := collectionArgumentIndex(typed, current)
		parameters := usage.calleeParams[calledObject(pass, typed.Fun)]
		if argument < 0 || argument >= len(parameters) || parameters[argument] == nil || seen[parameters[argument]] {
			return false
		}
		nextSeen := make(map[types.Object]bool, len(seen)+1)
		for object := range seen {
			nextSeen[object] = true
		}
		nextSeen[parameters[argument]] = true
		return collectionObjectReadOnly(pass, parameters[argument], usage, nextSeen)
	case *ast.BinaryExpr:
		return typed.Op == token.EQL || typed.Op == token.NEQ
	default:
		return false
	}
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
	function, ok := call.Fun.(*ast.Ident)
	builtin, builtinOK := pass.TypesInfo.Uses[function].(*types.Builtin)
	if !ok || !builtinOK {
		return false
	}
	switch builtin.Name() {
	case "len", "cap":
		return true
	case "append":
		// Reading a global through append's variadic inputs copies its
		// elements; using it as the destination may mutate its backing array.
		return collectionArgumentIndex(call, target) > 0
	case "copy":
		return collectionArgumentIndex(call, target) == 1
	default:
		return false
	}
}

func readOnlyCollectionPackageCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, symbol := range []analysisutil.FunctionSymbol{
		{Package: "slices", Name: "Contains"},
		{Package: "slices", Name: "ContainsFunc"},
		{Package: "slices", Name: "Index"},
		{Package: "slices", Name: "IndexFunc"},
		{Package: "slices", Name: "Equal"},
		{Package: "slices", Name: "EqualFunc"},
		{Package: "slices", Name: "Compare"},
		{Package: "slices", Name: "CompareFunc"},
		{Package: "slices", Name: "IsSorted"},
		{Package: "slices", Name: "IsSortedFunc"},
		{Package: "slices", Name: "Clone"},
		{Package: "strings", Name: "Join"},
		{Package: "sort", Name: "SearchStrings"},
		{Package: "bytes", Name: "Equal"},
		{Package: "bytes", Name: "HasPrefix"},
		{Package: "bytes", Name: "HasSuffix"},
		{Package: "maps", Name: "Clone"},
		{Package: "maps", Name: "Equal"},
		{Package: "maps", Name: "EqualFunc"},
		{Package: "maps", Name: "Keys"},
		{Package: "maps", Name: "Values"},
		{Package: "maps", Name: "All"},
	} {
		if analysisutil.IsPackageCall(pass, call, symbol) {
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
