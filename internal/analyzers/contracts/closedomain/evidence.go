package closedomain

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

func recordEnumFieldAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow, fields map[*types.Var]enumFlow) {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return
	}
	for index, left := range assignment.Lhs {
		field := selectedField(pass, left)
		if field != nil {
			mergeEnumFlowMap(fields, field, enumExpressionFlow(pass, assignment.Rhs[index], locals, summaries))
		}
	}
}

func recordEnumFieldComposite(pass *analysis.Pass, literal *ast.CompositeLit, locals map[*types.Var]enumFlow, summaries map[*types.Func]enumFlow, fields map[*types.Var]enumFlow) {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		identifier, ok := keyed.Key.(*ast.Ident)
		if !ok {
			continue
		}
		field, _ := pass.TypesInfo.Uses[identifier].(*types.Var)
		if field != nil && field.IsField() {
			mergeEnumFlowMap(fields, field, enumExpressionFlow(pass, keyed.Value, locals, summaries))
		}
	}
}

func recordEnumComparison(pass *analysis.Pass, expression *ast.BinaryExpr, values map[*types.Var]map[string]bool) {
	field, literal := enumFieldAndLiteral(pass, expression.X, expression.Y)
	if field == nil {
		field, literal = enumFieldAndLiteral(pass, expression.Y, expression.X)
	}
	if field != nil && (expression.Op == token.EQL || expression.Op == token.NEQ) {
		recordEnumValue(values, field, literal)
	}
}

func enumFieldAndLiteral(pass *analysis.Pass, fieldExpression, literalExpression ast.Expr) (*types.Var, string) {
	field := selectedField(pass, fieldExpression)
	if field == nil {
		return nil, ""
	}
	value := pass.TypesInfo.Types[literalExpression].Value
	if value == nil || value.Kind() != constant.String {
		return nil, ""
	}
	return field, constant.StringVal(value)
}

func recordEnumSwitch(pass *analysis.Pass, statement *ast.SwitchStmt, values map[*types.Var]map[string]bool) {
	field := selectedField(pass, statement.Tag)
	if field == nil {
		return
	}
	for _, item := range statement.Body.List {
		clause, ok := item.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, expression := range clause.List {
			value := pass.TypesInfo.Types[expression].Value
			if value != nil && value.Kind() == constant.String {
				recordEnumValue(values, field, constant.StringVal(value))
			}
		}
	}
}

func selectedField(pass *analysis.Pass, expression ast.Expr) *types.Var {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return nil
	}
	field, _ := selection.Obj().(*types.Var)
	if field != nil && field.IsField() {
		return field
	}
	return nil
}

func closeEnumTaggedUnionFields(pass *analysis.Pass, candidates map[*types.Var]enumCandidate, directValues map[*types.Var]map[string]bool, fieldFlows map[*types.Var]enumFlow, closed map[*types.Var]bool) {
	interfaces := enumPackageInterfaces(pass)
	for _, iface := range interfaces {
		groups := make(map[string][]*types.Var)
		for field, candidate := range candidates {
			if candidate.owner == nil || !enumImplements(candidate.owner, iface) {
				continue
			}
			groups[field.Name()] = append(groups[field.Name()], field)
		}
		for _, fields := range groups {
			values := make(map[string]bool)
			open := false
			for _, field := range fields {
				mergeEnumValues(values, directValues[field])
				mergeEnumValues(values, fieldFlows[field].values)
				open = open || fieldFlows[field].open
			}
			if len(values) < 2 || open {
				continue
			}
			for _, field := range fields {
				closed[field] = true
			}
		}
	}
}

func enumPackageInterfaces(pass *analysis.Pass) []*types.Interface {
	result := []*types.Interface{}
	for _, name := range pass.Pkg.Scope().Names() {
		typeName, ok := pass.Pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := typeName.Type().(*types.Named)
		if !ok {
			continue
		}
		iface, ok := named.Underlying().(*types.Interface)
		if ok {
			result = append(result, iface.Complete())
		}
	}
	return result
}

func enumImplements(named *types.Named, iface *types.Interface) bool {
	return types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface)
}

func propagateClosedEnumFields(pass *analysis.Pass, candidates map[*types.Var]enumCandidate, fieldFlows map[*types.Var]enumFlow, closed map[*types.Var]bool) {
	for {
		changed := false
		for field := range candidates {
			if closed[field] {
				continue
			}
			for source := range fieldFlows[field].sourceFields {
				var imported closedStringDomainFact
				if closed[source] || pass.ImportObjectFact(source, &imported) {
					closed[field] = true
					changed = true
					break
				}
			}
		}
		if !changed {
			return
		}
	}
}
