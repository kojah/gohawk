package general

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func determinismAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "determinism",
		Doc:  "checks map iteration reaching ordered output without explicit sorting",
		Run:  runDeterminism,
	}
}

func runDeterminism(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !orderedFunctionResult(pass, function) && !hasOrderedSink(pass, function.Body) {
				continue
			}
			sortPositions := sortPositions(pass, function.Body)
			orderingHelperPositions := orderingHelperPositions(function.Body)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				rangeStatement, rangeOK := node.(*ast.RangeStmt)
				if !rangeOK || !isMapType(pass.TypesInfo.TypeOf(rangeStatement.X)) {
					return true
				}
				if !hasSortAfter(sortPositions, rangeStatement.Pos()) && !hasSortAfter(orderingHelperPositions, rangeStatement.Pos()) {
					pass.Reportf(rangeStatement.Pos(), "map iteration reaches ordered output without sorting")
				}
				return true
			})
		}
	}
	return nil, nil
}

func orderedFunctionResult(pass *analysis.Pass, function *ast.FuncDecl) bool {
	signature, ok := pass.TypesInfo.TypeOf(function.Name).(*types.Signature)
	if !ok {
		return false
	}
	for result := range signature.Results().Variables() {
		switch underlying := result.Type().Underlying().(type) {
		case *types.Array, *types.Slice:
			return true
		case *types.Basic:
			if underlying.Kind() == types.String {
				return true
			}
		}
	}
	return false
}

func hasOrderedSink(pass *analysis.Pass, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := selector.Sel.Name
		found = found || analysisutil.IsPackageCall(pass, call, analysisutil.FunctionSymbol{Package: "encoding/json", Name: "Marshal"}) || strings.HasPrefix(name, "Write") || strings.HasPrefix(name, "Fprint")
		return !found
	})
	return found
}

func sortPositions(pass *analysis.Pass, body *ast.BlockStmt) []token.Pos {
	var positions []token.Pos
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		imported, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
		if ok && (imported.Imported().Path() == "sort" || imported.Imported().Path() == "slices" && strings.HasPrefix(selector.Sel.Name, "Sort")) {
			positions = append(positions, call.Pos())
		}
		return true
	})
	return positions
}

func orderingHelperPositions(body *ast.BlockStmt) []token.Pos {
	var positions []token.Pos
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "sorted") || strings.HasPrefix(lower, "ordered") || strings.HasPrefix(lower, "canonical") {
			positions = append(positions, call.Pos())
		}
		return true
	})
	return positions
}

func hasSortAfter(positions []token.Pos, position token.Pos) bool {
	for _, candidate := range positions {
		if candidate > position {
			return true
		}
	}
	return false
}

func isMapType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := value.Underlying().(*types.Map)
	return ok
}
