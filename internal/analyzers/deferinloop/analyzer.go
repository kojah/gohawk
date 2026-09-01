// Package deferinloop implements the deferinloop gohawk analyzer.
package deferinloop

import (
	"go/ast"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	ssautil "github.com/kojah/gohawk/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type sourceLine struct {
	file string
	line int
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "deferinloop",
		Doc:      "checks cleanup defers whose lifetime extends across loop iterations",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runDeferInLoop,
	}
}

func runDeferInLoop(pass *analysis.Pass) (any, error) {
	iteratingDefers, err := defersReachingAnotherIteration(pass)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			var body *ast.BlockStmt
			switch loop := node.(type) {
			case *ast.ForStmt:
				body = loop.Body
			case *ast.RangeStmt:
				body = loop.Body
			default:
				return true
			}
			ast.Inspect(body, func(candidate ast.Node) bool {
				switch typed := candidate.(type) {
				case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
					return false
				case *ast.DeferStmt:
					position := pass.Fset.PositionFor(typed.Pos(), false)
					if cleanupDefer(pass, body, typed.Call) && iteratingDefers[sourceLine{file: position.Filename, line: position.Line}] {
						analyzerbase.Reportf(pass, analyzerbase.CheckDeferCleanupInLoop, typed.Pos(), "deferred cleanup runs after the loop instead of after this iteration")
					}
					return false
				default:
					return true
				}
			})
			return true
		})
	}
	return nil, nil
}

func defersReachingAnotherIteration(pass *analysis.Pass) (map[sourceLine]bool, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	result := make(map[sourceLine]bool)
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				deferred, ok := instruction.(*ssa.Defer)
				if !ok {
					continue
				}
				position := pass.Fset.PositionFor(deferred.Pos(), false)
				key := sourceLine{file: position.Filename, line: position.Line}
				result[key] = result[key] || reachesLoopBackedge(deferred)
			}
		}
	}
	return result, nil
}

func reachesLoopBackedge(deferred *ssa.Defer) bool {
	// A defer in a terminal match branch is harmless when no path from that
	// branch reaches a block dominating the defer's block (the next iteration).
	// Real examples use this pattern to return a matched package immediately:
	// https://github.com/ruaan-deysel/vault/blob/0676007385e0b5bd31dd27d515951a867ee708fe/internal/diagnostics/package_test.go#L160
	start := deferred.Block()
	queue := append([]*ssa.BasicBlock(nil), start.Succs...)
	seen := map[*ssa.BasicBlock]bool{}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			continue
		}
		seen[block] = true
		if block.Dominates(start) {
			return true
		}
		queue = append(queue, block.Succs...)
	}
	return false
}

func cleanupDefer(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	var name string
	var target ast.Expr
	switch function := call.Fun.(type) {
	case *ast.Ident:
		name = function.Name
		if name == "close" {
			return false
		}
		target = function
	case *ast.SelectorExpr:
		name = function.Sel.Name
		target = function.X
	}
	name = strings.ToLower(name)
	cleanup := false
	for _, fragment := range []string{"cancel", "cleanup", "close", "commit", "release", "rollback", "stop", "unlock"} {
		if strings.Contains(name, fragment) {
			cleanup = true
			break
		}
	}
	if !cleanup || target == nil {
		return false
	}
	if strings.Contains(name, "unlock") {
		return loopAcquiresTarget(pass, body, target)
	}
	root := expressionRoot(target)
	object := pass.TypesInfo.ObjectOf(root)
	return object != nil && object.Pos() >= body.Pos() && object.Pos() < body.End()
}

func loopAcquiresTarget(pass *analysis.Pass, body *ast.BlockStmt, target ast.Expr) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") && analysisutil.SameExpression(pass, selector.X, target) {
			found = true
			return false
		}
		return true
	})
	return found
}

func expressionRoot(expression ast.Expr) *ast.Ident {
	switch candidate := expression.(type) {
	case *ast.Ident:
		return candidate
	case *ast.SelectorExpr:
		return expressionRoot(candidate.X)
	case *ast.IndexExpr:
		return expressionRoot(candidate.X)
	case *ast.ParenExpr:
		return expressionRoot(candidate.X)
	case *ast.StarExpr:
		return expressionRoot(candidate.X)
	default:
		return nil
	}
}
