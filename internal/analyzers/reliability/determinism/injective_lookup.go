package determinism

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
)

// Injective lookup evidence accepts only fresh constant maps whose loop's complete effect selects the unique key for one compared value.
func injectiveMapValueLookup(pass *analysis.Pass, block *ast.BlockStmt, index int, ranged *ast.RangeStmt) mapRangeDecision {
	if !injectiveRangeSource(pass, block, index, ranged.X) || mapMayMutateOrEscape(pass, ranged.Body, ranged.X) {
		return mapRangeDecision{}
	}
	keyName, keyOK := ranged.Key.(*ast.Ident)
	valueName, valueOK := ranged.Value.(*ast.Ident)
	if ranged.Tok != token.DEFINE || !keyOK || !valueOK || keyName.Name == "_" || valueName.Name == "_" || len(ranged.Body.List) != 1 {
		return mapRangeDecision{}
	}
	key, value := pass.TypesInfo.ObjectOf(keyName), pass.TypesInfo.ObjectOf(valueName)
	condition, ok := ranged.Body.List[0].(*ast.IfStmt)
	if !ok || condition.Init != nil || condition.Else != nil || len(condition.Body.List) != 1 {
		return mapRangeDecision{}
	}
	target := equalityTarget(pass, condition.Cond, value, key)
	if target == nil || !selectedKeyEffect(pass, block, index, condition.Body.List[0], key, target) {
		return mapRangeDecision{}
	}
	// These real-world lookups are stable because distinct constant values make the reverse lookup injective, not because iteration became ordered.
	// https://github.com/fclairamb/ftpserverlib/blob/e6a24183e0664dbd265bbb9d01b21c8a91c98f51/client_handler.go#L67-L89
	// https://github.com/tgragnato/snowflake/blob/29b45d7671e92a797ccdc68fab0d1210d37a038d/dtls/pkg/crypto/fingerprint/hash.go#L13-L41
	return mapRangeDecision{reason: "injective-map-value-lookup"}
}

func equalityTarget(pass *analysis.Pass, expression ast.Expr, value, key types.Object) types.Object {
	comparison, ok := syntax.Unparen(expression).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL {
		return nil
	}
	for _, pair := range [][2]ast.Expr{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
		left, leftOK := syntax.Unparen(pair[0]).(*ast.Ident)
		right, rightOK := syntax.Unparen(pair[1]).(*ast.Ident)
		target := pass.TypesInfo.ObjectOf(right)
		targetVariable, variable := target.(*types.Var)
		if leftOK && rightOK && pass.TypesInfo.ObjectOf(left) == value && variable && target != value && target != key &&
			targetVariable.Parent() != nil && targetVariable.Parent() != pass.Pkg.Scope() {
			return target
		}
	}
	return nil
}

func selectedKeyEffect(pass *analysis.Pass, block *ast.BlockStmt, index int, effect ast.Stmt, key, target types.Object) bool {
	if returned, ok := effect.(*ast.ReturnStmt); ok {
		return returnsSelectedKey(pass, returned, key)
	}
	assignment, ok := effect.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	return selectedKeyAccumulator(pass, block, index, assignment, key, target)
}

func selectedKeyAccumulator(pass *analysis.Pass, block *ast.BlockStmt, index int, assignment *ast.AssignStmt, key, target types.Object) bool {
	accumulator, accumulatorOK := assignment.Lhs[0].(*ast.Ident)
	selected, selectedOK := syntax.Unparen(assignment.Rhs[0]).(*ast.Ident)
	accumulatorObject := pass.TypesInfo.ObjectOf(accumulator)
	if !accumulatorOK || !selectedOK || pass.TypesInfo.ObjectOf(selected) != key || accumulatorObject == nil || accumulatorObject == target ||
		index < 2 || index+2 != len(block.List) {
		return false
	}
	returned, ok := block.List[index+1].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	returnedName, returnedOK := syntax.Unparen(returned.Results[0]).(*ast.Ident)
	if !returnedOK || pass.TypesInfo.ObjectOf(returnedName) != accumulatorObject {
		return false
	}
	initialization, initOK := block.List[index-2].(*ast.AssignStmt)
	name, nameOK := singleAssignedIdentifier(initialization)
	return initOK && nameOK && initialization.Tok == token.DEFINE && len(initialization.Rhs) == 1 &&
		pass.TypesInfo.ObjectOf(name) == accumulatorObject && pass.TypesInfo.Types[initialization.Rhs[0]].Value != nil
}

func singleAssignedIdentifier(assignment *ast.AssignStmt) (*ast.Ident, bool) {
	if assignment == nil || len(assignment.Lhs) != 1 {
		return nil, false
	}
	identifier, ok := assignment.Lhs[0].(*ast.Ident)
	return identifier, ok
}

func returnsSelectedKey(pass *analysis.Pass, returned *ast.ReturnStmt, key types.Object) bool {
	if len(returned.Results) == 0 || len(returned.Results) > 2 {
		return false
	}
	identifier, ok := syntax.Unparen(returned.Results[0]).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(identifier) != key || len(returned.Results) == 1 {
		return ok && pass.TypesInfo.ObjectOf(identifier) == key
	}
	nilName, nilOK := syntax.Unparen(returned.Results[1]).(*ast.Ident)
	return nilOK && pass.TypesInfo.ObjectOf(nilName) == types.Universe.Lookup("nil")
}

func injectiveRangeSource(pass *analysis.Pass, block *ast.BlockStmt, index int, expression ast.Expr) bool {
	if injectiveMapExpression(pass, expression) {
		return true
	}
	identifier, ok := syntax.Unparen(expression).(*ast.Ident)
	if !ok || index == 0 {
		return false
	}
	assignment, ok := block.List[index-1].(*ast.AssignStmt)
	name, nameOK := singleAssignedIdentifier(assignment)
	return ok && nameOK && assignment.Tok == token.DEFINE && len(assignment.Rhs) == 1 &&
		pass.TypesInfo.ObjectOf(name) == pass.TypesInfo.ObjectOf(identifier) && injectiveMapExpression(pass, assignment.Rhs[0])
}

func injectiveMapExpression(pass *analysis.Pass, expression ast.Expr) bool {
	switch source := syntax.Unparen(expression).(type) {
	case *ast.CompositeLit:
		values := make(map[string]bool)
		return addLiteralEntries(pass, source, values) && len(values) > 0
	case *ast.CallExpr:
		return injectiveMapHelper(pass, source)
	default:
		return false
	}
}

func injectiveMapHelper(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, ok := syntax.Unparen(call.Fun).(*ast.Ident)
	if !ok || len(call.Args) != 0 {
		return false
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Type.Params.NumFields() != 0 || ast.IsExported(function.Name.Name) ||
				pass.TypesInfo.Defs[function.Name] != pass.TypesInfo.ObjectOf(identifier) || function.Body == nil {
				continue
			}
			return injectiveMapBody(pass, function.Body)
		}
	}
	return false
}

func injectiveMapBody(pass *analysis.Pass, body *ast.BlockStmt) bool {
	values := make(map[string]bool)
	if len(body.List) == 1 {
		returned, ok := body.List[0].(*ast.ReturnStmt)
		literal, literalOK := singleResult(returned).(*ast.CompositeLit)
		return ok && literalOK && addLiteralEntries(pass, literal, values) && len(values) > 0
	}
	if len(body.List) < 3 {
		return false
	}
	initialization, ok := body.List[0].(*ast.AssignStmt)
	name, nameOK := singleAssignedIdentifier(initialization)
	if !ok || !nameOK || initialization.Tok != token.DEFINE || len(initialization.Rhs) != 1 {
		return false
	}
	makeCall, makeOK := syntax.Unparen(initialization.Rhs[0]).(*ast.CallExpr)
	if !makeOK || len(makeCall.Args) != 1 || !syntax.IsCallTo(pass, makeCall, syntax.Builtin("make")) || !isMapType(pass.TypesInfo.TypeOf(makeCall.Args[0])) {
		return false
	}
	mapObject := pass.TypesInfo.ObjectOf(name)
	for _, statement := range body.List[1 : len(body.List)-1] {
		if !addConstantMapAssignment(pass, statement, mapObject, values) {
			return false
		}
	}
	returned, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
	result, resultOK := singleResult(returned).(*ast.Ident)
	return ok && resultOK && pass.TypesInfo.ObjectOf(result) == mapObject && len(values) > 0
}

func addConstantMapAssignment(pass *analysis.Pass, statement ast.Stmt, mapObject types.Object, values map[string]bool) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	indexed, ok := syntax.Unparen(assignment.Lhs[0]).(*ast.IndexExpr)
	if !ok {
		return false
	}
	mapName, ok := syntax.Unparen(indexed.X).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(mapName) == mapObject && addConstantEntry(pass, values, indexed.Index, assignment.Rhs[0])
}

func singleResult(returned *ast.ReturnStmt) ast.Expr {
	if returned == nil || len(returned.Results) != 1 {
		return nil
	}
	return syntax.Unparen(returned.Results[0])
}

func addLiteralEntries(pass *analysis.Pass, literal *ast.CompositeLit, values map[string]bool) bool {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok || !addConstantEntry(pass, values, pair.Key, pair.Value) {
			return false
		}
	}
	return true
}

func addConstantEntry(pass *analysis.Pass, values map[string]bool, key, value ast.Expr) bool {
	keyValue, valueValue := pass.TypesInfo.Types[key].Value, pass.TypesInfo.Types[value].Value
	if keyValue == nil || valueValue == nil || values[valueValue.ExactString()] {
		return false
	}
	values[valueValue.ExactString()] = true
	return true
}
