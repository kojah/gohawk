package contracts

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func enumLocalFlows(pass *analysis.Pass, files []*ast.File) (map[*types.Var]enumFlow, map[*types.Func]enumFlow) {
	locals := make(map[*types.Var]enumFlow)
	summaries := make(map[*types.Func]enumFlow)
	seedEnumOpenParameters(pass, files, locals)
	for range 32 {
		changed := false
		for _, file := range files {
			ast.Inspect(file, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.AssignStmt:
					changed = mergeEnumLocalAssignment(pass, typed, locals, summaries) || changed
				case *ast.ValueSpec:
					changed = mergeEnumValueSpec(pass, typed, locals, summaries) || changed
				case *ast.FuncDecl:
					changed = mergeEnumFunctionReturns(pass, typed, locals, summaries) || changed
				}
				return true
			})
		}
		if !changed {
			break
		}
	}
	return locals, summaries
}

func seedEnumOpenParameters(pass *analysis.Pass, files []*ast.File, locals map[*types.Var]enumFlow) {
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}
			for _, fields := range []*ast.FieldList{declaration.Recv, declaration.Type.Params} {
				if fields == nil {
					continue
				}
				for _, field := range fields.List {
					for _, name := range field.Names {
						if variable := enumIdentifierVariable(pass, name); variable != nil {
							locals[variable] = enumFlow{open: true}
						}
					}
				}
			}
			return false
		})
	}
}

func mergeEnumLocalAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow) bool {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return false
	}
	changed := false
	for index, left := range assignment.Lhs {
		identifier, ok := left.(*ast.Ident)
		if !ok {
			continue
		}
		variable := enumIdentifierVariable(pass, identifier)
		if variable == nil || variable.IsField() {
			continue
		}
		changed = mergeEnumFlowMap(locals, variable, enumExpressionFlow(pass, assignment.Rhs[index], locals, summaries)) || changed
	}
	return changed
}

func mergeEnumValueSpec(pass *analysis.Pass, specification *ast.ValueSpec, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow) bool {
	if len(specification.Names) != len(specification.Values) {
		return false
	}
	changed := false
	for index, name := range specification.Names {
		variable := enumIdentifierVariable(pass, name)
		if variable == nil || variable.IsField() {
			continue
		}
		changed = mergeEnumFlowMap(locals, variable, enumExpressionFlow(pass, specification.Values[index], locals, summaries)) || changed
	}
	return changed
}

func mergeEnumFunctionReturns(pass *analysis.Pass, declaration *ast.FuncDecl, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow) bool {
	function, _ := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if function == nil || declaration.Body == nil {
		return false
	}
	flow := enumFlow{}
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok || len(statement.Results) != 1 {
			return true
		}
		mergeEnumFlow(&flow, enumExpressionFlow(pass, statement.Results[0], locals, summaries))
		return true
	})
	return mergeEnumFlowMap(summaries, function, flow)
}

func enumIdentifierVariable(pass *analysis.Pass, identifier *ast.Ident) *types.Var {
	if variable, ok := pass.TypesInfo.Defs[identifier].(*types.Var); ok {
		return variable
	}
	variable, _ := pass.TypesInfo.Uses[identifier].(*types.Var)
	return variable
}

func enumExpressionFlow(pass *analysis.Pass, expression ast.Expr, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow) enumFlow {
	if expression == nil {
		return enumFlow{open: true}
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.String {
		return enumLiteralFlow(constant.StringVal(value))
	}
	switch typed := expression.(type) {
	case *ast.Ident:
		return enumIdentifierFlow(pass, typed, locals)
	case *ast.ParenExpr:
		return enumExpressionFlow(pass, typed.X, locals, summaries)
	case *ast.UnaryExpr:
		if typed.Op == token.AND {
			return enumExpressionFlow(pass, typed.X, locals, summaries)
		}
	case *ast.SelectorExpr:
		if field := selectedField(pass, typed); field != nil {
			return enumFlow{sourceFields: map[*types.Var]bool{field: true}}
		}
	case *ast.CallExpr:
		return enumCallFlow(pass, typed, locals, summaries)
	}
	return enumFlow{open: true}
}

func enumIdentifierFlow(pass *analysis.Pass, identifier *ast.Ident, locals map[*types.Var]enumFlow) enumFlow {
	variable := enumIdentifierVariable(pass, identifier)
	if variable == nil {
		return enumFlow{open: true}
	}
	if flow, ok := locals[variable]; ok {
		return cloneEnumFlow(flow)
	}
	if variable.Parent() != pass.Pkg.Scope() {
		return enumFlow{}
	}
	return enumFlow{open: true}
}

func enumCallFlow(pass *analysis.Pass, call *ast.CallExpr, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow) enumFlow {
	if enumStringConversion(pass, call) {
		flow := enumExpressionFlow(pass, call.Args[0], locals, summaries)
		if enumNamedString(pass.TypesInfo.TypeOf(call.Args[0])) {
			flow.erasedNamed = true
		}
		return flow
	}
	function := enumCalledFunction(pass, call.Fun)
	if function == nil {
		return enumFlow{open: true}
	}
	if flow, ok := summaries[function]; ok {
		return cloneEnumFlow(flow)
	}
	if function.Pkg() == pass.Pkg {
		return enumFlow{}
	}
	return enumFlow{open: true}
}

func enumStringConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 1 || !pass.TypesInfo.Types[call.Fun].IsType() {
		return false
	}
	basic, ok := types.Unalias(pass.TypesInfo.TypeOf(call.Fun)).(*types.Basic)
	return ok && basic.Kind() == types.String
}

func enumNamedString(value types.Type) bool {
	named, ok := types.Unalias(value).(*types.Named)
	return ok && analysisutil.IsStringType(named.Underlying())
}

func enumCalledFunction(pass *analysis.Pass, expression ast.Expr) *types.Func {
	switch typed := expression.(type) {
	case *ast.Ident:
		function, _ := pass.TypesInfo.Uses[typed].(*types.Func)
		return function
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[typed]; selection != nil {
			function, _ := selection.Obj().(*types.Func)
			return function
		}
		function, _ := pass.TypesInfo.Uses[typed.Sel].(*types.Func)
		return function
	default:
		return nil
	}
}

func enumLiteralFlow(value string) enumFlow {
	return enumFlow{values: map[string]bool{value: true}}
}

func cloneEnumFlow(flow enumFlow) enumFlow {
	clone := enumFlow{erasedNamed: flow.erasedNamed, open: flow.open}
	mergeEnumValuesIntoFlow(&clone, flow.values)
	for field := range flow.sourceFields {
		if clone.sourceFields == nil {
			clone.sourceFields = make(map[*types.Var]bool)
		}
		clone.sourceFields[field] = true
	}
	return clone
}

func mergeEnumFlowMap[K comparable](values map[K]enumFlow, key K, addition enumFlow) bool {
	current := values[key]
	changed := mergeEnumFlow(&current, addition)
	if changed {
		values[key] = current
	}
	return changed
}

func mergeEnumFlow(target *enumFlow, addition enumFlow) bool {
	changed := false
	before := len(target.values)
	mergeEnumValuesIntoFlow(target, addition.values)
	changed = changed || len(target.values) != before
	before = len(target.sourceFields)
	for field := range addition.sourceFields {
		if target.sourceFields == nil {
			target.sourceFields = make(map[*types.Var]bool)
		}
		target.sourceFields[field] = true
	}
	changed = changed || len(target.sourceFields) != before
	if addition.erasedNamed && !target.erasedNamed {
		target.erasedNamed = true
		changed = true
	}
	if addition.open && !target.open {
		target.open = true
		changed = true
	}
	return changed
}

func mergeEnumValuesIntoFlow(flow *enumFlow, values map[string]bool) {
	if len(values) == 0 {
		return
	}
	if flow.values == nil {
		flow.values = make(map[string]bool)
	}
	mergeEnumValues(flow.values, values)
}

func mergeEnumValues(target, source map[string]bool) {
	for value := range source {
		target[value] = true
	}
}

func recordEnumValue(values map[*types.Var]map[string]bool, field *types.Var, value string) {
	if values[field] == nil {
		values[field] = make(map[string]bool)
	}
	values[field][value] = true
}
