package contracts

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

type closedStringDomainFact struct{}

func (*closedStringDomainFact) AFact() {}

func (*closedStringDomainFact) String() string { return "closedStringDomain" }

type enumCandidate struct {
	field    *types.Var
	owner    *types.Named
	position *ast.Ident
}

type enumFlow struct {
	values       map[string]bool
	sourceFields map[*types.Var]bool
	erasedNamed  bool
	open         bool
}

func closedDomainAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:      "closedomain",
		Doc:       "finds builtin-string fields used as closed semantic domains",
		Run:       runEnumField,
		FactTypes: []analysis.Fact{new(closedStringDomainFact)},
	}
}

func runEnumField(pass *analysis.Pass) (any, error) {
	files := enumProductionFiles(pass)
	candidates := enumCandidates(pass, files)
	locals, summaries := enumLocalFlows(pass, files)
	directValues := make(map[*types.Var]map[string]bool)
	fieldFlows := make(map[*types.Var]enumFlow)

	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.AssignStmt:
				recordEnumFieldAssignment(pass, typed, locals, summaries, fieldFlows)
			case *ast.CompositeLit:
				recordEnumFieldComposite(pass, typed, locals, summaries, fieldFlows)
			case *ast.BinaryExpr:
				recordEnumComparison(pass, typed, directValues)
			case *ast.SwitchStmt:
				recordEnumSwitch(pass, typed, directValues)
			}
			return true
		})
	}

	closed := make(map[*types.Var]bool)
	for field := range candidates {
		flow := fieldFlows[field]
		if len(directValues[field]) >= 2 || flow.erasedNamed || !flow.open && len(flow.values) >= 2 {
			closed[field] = true
		}
	}
	closeEnumTaggedUnionFields(pass, candidates, directValues, fieldFlows, closed)
	propagateClosedEnumFields(pass, candidates, fieldFlows, closed)

	for field, candidate := range candidates {
		if !closed[field] {
			continue
		}
		reportf(pass, checkClosedStringDomain, candidate.position.Pos(), "field %s uses a closed string domain; define a named string type and constants", candidate.position.Name)
		pass.ExportObjectFact(field, new(closedStringDomainFact))
	}
	return nil, nil
}

func enumProductionFiles(pass *analysis.Pass) []*ast.File {
	files := make([]*ast.File, 0, len(pass.Files))
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) || strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		files = append(files, file)
	}
	return files
}

func enumCandidates(pass *analysis.Pass, files []*ast.File) map[*types.Var]enumCandidate {
	result := make(map[*types.Var]enumCandidate)
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || enumStagingType(typeSpec.Name.Name) {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				typeName, _ := pass.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
				owner, _ := typeName.Type().(*types.Named)
				for _, field := range structure.Fields.List {
					recordEnumFieldCandidate(pass, owner, field, result)
				}
			}
		}
	}
	return result
}

func enumStagingType(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, "input") || strings.HasSuffix(name, "wire") || strings.HasPrefix(name, "raw")
}

func recordEnumFieldCandidate(pass *analysis.Pass, owner *types.Named, field *ast.Field, candidates map[*types.Var]enumCandidate) {
	if !enumBuiltinStringStorage(pass.TypesInfo.TypeOf(field.Type)) {
		return
	}
	for _, name := range field.Names {
		if !name.IsExported() || !enumFieldName(name.Name) {
			continue
		}
		variable, ok := pass.TypesInfo.Defs[name].(*types.Var)
		if ok && variable.IsField() {
			candidates[variable] = enumCandidate{field: variable, owner: owner, position: name}
		}
	}
}

func enumBuiltinStringStorage(value types.Type) bool {
	if pointer, ok := types.Unalias(value).(*types.Pointer); ok {
		value = pointer.Elem()
	}
	basic, ok := types.Unalias(value).(*types.Basic)
	return ok && basic.Kind() == types.String
}

func enumFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "action", "adapter", "code", "coverage", "granularity", "kind", "level", "mode", "outcome", "phase", "plugin", "provider", "reason", "requirement", "resultsource", "role", "severity", "state", "status", "trigger", "type":
		return true
	default:
		return false
	}
}

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
