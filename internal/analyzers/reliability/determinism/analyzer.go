// Package determinism implements the determinism gohawk analyzer.
package determinism

import (
	"go/ast"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{Name: "determinism", Doc: "checks map iteration reaching ordered output without explicit sorting", Run: runDeterminism}
}

func runDeterminism(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil {
				analyzeDeterministicBlock(pass, function, function.Body)
			}
		}
	}
	return nil, nil
}

func analyzeDeterministicBlock(pass *analysis.Pass, function *ast.FuncDecl, block *ast.BlockStmt) {
	for index, statement := range block.List {
		ranged, ok := statement.(*ast.RangeStmt)
		if ok && isMapType(pass.TypesInfo.TypeOf(ranged.X)) && mapRangeReachesOrderedOutput(pass, function, block, index, ranged) {
			analyzerbase.Report(pass, analyzerbase.CheckDeterministicMapOutput, analysis.Diagnostic{Pos: ranged.Pos(), End: ranged.X.End(), Message: "map iteration reaches ordered output without sorting"})
		}
		inspectNestedDeterministicBlocks(pass, function, statement)
	}
}

func inspectNestedDeterministicBlocks(pass *analysis.Pass, function *ast.FuncDecl, statement ast.Stmt) {
	switch typed := statement.(type) {
	case *ast.BlockStmt:
		analyzeDeterministicBlock(pass, function, typed)
	case *ast.LabeledStmt:
		analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: []ast.Stmt{typed.Stmt}})
	case *ast.IfStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
		if alternate, ok := typed.Else.(*ast.BlockStmt); ok {
			analyzeDeterministicBlock(pass, function, alternate)
		} else if alternate, ok := typed.Else.(*ast.IfStmt); ok {
			inspectNestedDeterministicBlocks(pass, function, alternate)
		}
	case *ast.ForStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
	case *ast.RangeStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
	case *ast.SwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	case *ast.TypeSwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	case *ast.SelectStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CommClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	}
}
