package resourcelifetime

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func completeTimerLifecyclePositions(pass *analysis.Pass) map[token.Pos]bool {
	result := make(map[token.Pos]bool)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				return true
			}
			acquisitions := make(map[types.Object][]token.Pos)
			ast.Inspect(function.Body, func(candidate ast.Node) bool {
				assignment, ok := candidate.(*ast.AssignStmt)
				if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
					return true
				}
				name, nameOK := assignment.Lhs[0].(*ast.Ident)
				call, callOK := assignment.Rhs[0].(*ast.CallExpr)
				if nameOK && callOK && analysisutil.IsPackageCall(pass, call, analysisutil.FunctionSymbol{Package: "time", Name: "NewTimer"}) {
					// SSA records the call position at the opening parenthesis, while
					// the AST call begins at the package selector. Retain both so the
					// proven source-level lifecycle can be associated with its SSA call.
					acquisitions[pass.TypesInfo.ObjectOf(name)] = []token.Pos{call.Pos(), call.Lparen}
				}
				return true
			})
			for timer, positions := range acquisitions {
				if timerHasStopDrainAndReceive(pass, function.Body, timer) {
					// A timer that sets a timeout flag when its channel fires and is
					// otherwise stopped-and-drained has exactly one cleanup action on
					// every path. ccLoad uses the canonical form while waiting for two
					// concurrent identities:
					// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/proxy_responses_websocket_test.go#L3949-L3968
					for _, position := range positions {
						result[position] = true
					}
				}
			}
			return false
		})
	}
	return result
}

func timerHasStopDrainAndReceive(pass *analysis.Pass, body *ast.BlockStmt, timer types.Object) bool {
	stopAndDrain := false
	selected := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.IfStmt:
			if expressionCallsTimerStop(pass, typed.Cond, timer) && blockReceivesTimer(pass, typed.Body, timer) {
				stopAndDrain = true
			}
		case *ast.SelectStmt:
			for _, statement := range typed.Body.List {
				clause, ok := statement.(*ast.CommClause)
				if ok && receivesTimer(pass, clause.Comm, timer) {
					selected = true
				}
			}
		}
		return !stopAndDrain || !selected
	})
	return stopAndDrain && selected
}

func expressionCallsTimerStop(pass *analysis.Pass, expression ast.Expr, timer types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name, nameOK := selector.X.(*ast.Ident)
		if nameOK && selector.Sel.Name == "Stop" && pass.TypesInfo.ObjectOf(name) == timer {
			found = true
			return false
		}
		return true
	})
	return found
}

func blockReceivesTimer(pass *analysis.Pass, block *ast.BlockStmt, timer types.Object) bool {
	found := false
	ast.Inspect(block, func(node ast.Node) bool {
		if receivesTimer(pass, node, timer) {
			found = true
			return false
		}
		return true
	})
	return found
}

func receivesTimer(pass *analysis.Pass, node ast.Node, timer types.Object) bool {
	if statement, ok := node.(*ast.ExprStmt); ok {
		node = statement.X
	}
	receive, ok := node.(*ast.UnaryExpr)
	if !ok || receive.Op != token.ARROW {
		return false
	}
	channel, ok := receive.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name, nameOK := channel.X.(*ast.Ident)
	return nameOK && channel.Sel.Name == "C" && pass.TypesInfo.ObjectOf(name) == timer
}
