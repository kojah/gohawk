// Package determinism implements the determinism gohawk analyzer.
package determinism

import (
	"go/ast"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{Name: "determinism", Doc: "checks map iteration reaching ordered output without explicit sorting", Run: runDeterminism}
}

func runDeterminism(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !syntax.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil {
				analyzeDeterministicBlock(pass, function, function.Body, nil)
			}
		}
	}
	return nil, nil
}

func analyzeDeterministicBlock(pass *analysis.Pass, function *ast.FuncDecl, block *ast.BlockStmt, guardedMap ast.Expr) {
	for index, statement := range block.List {
		ranged, ok := statement.(*ast.RangeStmt)
		if ok && isMapType(pass.TypesInfo.TypeOf(ranged.X)) {
			decision := evaluateMapRange(pass, function, block, index, ranged, guardedMap)
			emitMapRangeDecision(pass, ranged, decision)
			if !decision.report {
				inspectNestedDeterministicBlocks(pass, function, statement)
				continue
			}
			check.Report(
				pass,
				check.DeterministicMapOutput,
				analysis.Diagnostic{Pos: ranged.Pos(), End: ranged.X.End(), Message: "map iteration reaches ordered output without sorting"},
			)
		}
		inspectNestedDeterministicBlocks(pass, function, statement)
	}
}

func emitMapRangeDecision(pass *analysis.Pass, ranged *ast.RangeStmt, decision mapRangeDecision) {
	if decision.reason == "" || !analysisTrace.Enabled("determinism", string(check.DeterministicMapOutput)) {
		return
	}
	analysisTrace.For(pass, "determinism", string(check.DeterministicMapOutput), ranged.Pos()).Evidence(analysisTrace.Step{
		Reason:  decision.reason,
		Outcome: analysisTrace.OutcomeAccepted,
		Pos:     ranged.Pos(),
	})
}

func inspectNestedDeterministicBlocks(pass *analysis.Pass, function *ast.FuncDecl, statement ast.Stmt) {
	switch typed := statement.(type) {
	case *ast.BlockStmt:
		analyzeDeterministicBlock(pass, function, typed, nil)
	case *ast.LabeledStmt:
		analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: []ast.Stmt{typed.Stmt}}, nil)
	case *ast.IfStmt:
		analyzeDeterministicBlock(pass, function, typed.Body, positiveSingletonMap(pass, typed.Cond))
		if alternate, ok := typed.Else.(*ast.BlockStmt); ok {
			analyzeDeterministicBlock(pass, function, alternate, nil)
		} else if alternate, ok := typed.Else.(*ast.IfStmt); ok {
			inspectNestedDeterministicBlocks(pass, function, alternate)
		}
	case *ast.ForStmt:
		analyzeDeterministicBlock(pass, function, typed.Body, nil)
	case *ast.RangeStmt:
		analyzeDeterministicBlock(pass, function, typed.Body, nil)
	case *ast.SwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body}, nil)
			}
		}
	case *ast.TypeSwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body}, nil)
			}
		}
	case *ast.SelectStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CommClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body}, nil)
			}
		}
	}
}
