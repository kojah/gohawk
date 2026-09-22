// Package inlineerror implements the inlineerror gohawk analyzer.
package inlineerror

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "inlineerror", Doc: "checks inline error declarations for mismatched conditions",
		Requires: []*analysis.Analyzer{inspect.Analyzer}, Run: runInlineError,
	}
}

func runInlineError(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	following := immediateFollowingStatements(pass.Files)
	in.Preorder([]ast.Node{(*ast.IfStmt)(nil)}, func(node ast.Node) {
		statement := node.(*ast.IfStmt)
		assignment, ok := statement.Init.(*ast.AssignStmt)
		if !ok || assignment.Tok.String() != ":=" {
			return
		}
		for _, expression := range assignment.Lhs {
			fresh, ok := expression.(*ast.Ident)
			if !ok || pass.TypesInfo.Defs[fresh] == nil || !syntax.IsErrorType(pass.TypesInfo.TypeOf(fresh)) {
				continue
			}
			freshObject := pass.TypesInfo.ObjectOf(fresh)
			if syntax.ExpressionUsesObject(pass, statement.Cond, freshObject) || !returnsOnlyObject(pass, statement.Body, freshObject) {
				continue
			}
			var mismatched *ast.Ident
			ast.Inspect(statement.Cond, func(candidate ast.Node) bool {
				identifier, ok := candidate.(*ast.Ident)
				if ok && pass.TypesInfo.ObjectOf(identifier) != freshObject && syntax.IsErrorType(pass.TypesInfo.TypeOf(identifier)) {
					mismatched = identifier
					return false
				}
				return true
			})
			if mismatched != nil {
				proof := priorErrorPrecedenceProof(pass, statement, assignment, freshObject, pass.TypesInfo.ObjectOf(mismatched), following[statement])
				if proof.accepted {
					emitInlineErrorDecision(pass, statement, proof)
					continue
				}
				check.Reportf(
					pass, check.ErrorMismatchedInline, mismatched.Pos(),
					"condition checks %s instead of newly declared %s", mismatched.Name, fresh.Name,
				)
			}
		}
	})
	return nil, nil
}

type inlineErrorProof struct {
	accepted bool
	reason   string
}

func priorErrorPrecedenceProof(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	assignment *ast.AssignStmt,
	fresh, prior types.Object,
	following ast.Stmt,
) inlineErrorProof {
	if statement.Else != nil || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return inlineErrorProof{}
	}
	if _, ok := syntax.Unparen(assignment.Rhs[0]).(*ast.CallExpr); !ok || !nilErrorComparison(pass, statement.Cond, prior) {
		return inlineErrorProof{}
	}
	if !returnsExactObject(pass, statement.Body, fresh) || !statementReturnsExactObject(pass, following, prior) {
		return inlineErrorProof{}
	}
	// Cleanup may fail independently after earlier work has already failed. Returning
	// the cleanup error only when the prior error is nil deliberately preserves the
	// original failure otherwise. Pyroscope's syncFD uses this exact precedence:
	// https://github.com/grafana/pyroscope/blob/d1212251265e7dab4b03ef0d80af565f6d519e1b/pkg/metastore/fsm/boltdb.go#L234-L239
	return inlineErrorProof{accepted: true, reason: "prior-error-precedence"}
}

func nilErrorComparison(pass *analysis.Pass, condition ast.Expr, prior types.Object) bool {
	comparison, ok := syntax.Unparen(condition).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL {
		return false
	}
	return exactObject(pass, comparison.X, prior) && nilIdentifier(pass, comparison.Y) ||
		nilIdentifier(pass, comparison.X) && exactObject(pass, comparison.Y, prior)
}

func nilIdentifier(pass *analysis.Pass, expression ast.Expr) bool {
	identifier, ok := syntax.Unparen(expression).(*ast.Ident)
	return ok && identifier.Name == "nil" && pass.TypesInfo.ObjectOf(identifier) == types.Universe.Lookup("nil")
}

func returnsExactObject(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	return statementReturnsExactObject(pass, body.List[0], object)
}

func statementReturnsExactObject(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && exactObject(pass, returned.Results[0], object)
}

func exactObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := syntax.Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(identifier) == object
}

func immediateFollowingStatements(files []*ast.File) map[*ast.IfStmt]ast.Stmt {
	result := make(map[*ast.IfStmt]ast.Stmt)
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for index := 0; index+1 < len(block.List); index++ {
				candidate := block.List[index]
				if statement, ok := candidate.(*ast.IfStmt); ok {
					result[statement] = block.List[index+1]
				}
			}
			return true
		})
	}
	return result
}

func emitInlineErrorDecision(pass *analysis.Pass, statement *ast.IfStmt, proof inlineErrorProof) {
	checkID := string(check.ErrorMismatchedInline)
	analysisTrace.For(pass, "inlineerror", checkID, statement.Pos()).Evidence(analysisTrace.Step{
		Reason:  proof.reason,
		Outcome: analysisTrace.OutcomeAccepted,
		Pos:     statement.Pos(),
	})
}

func returnsOnlyObject(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	returned, ok := body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && syntax.ExpressionUsesObject(pass, returned.Results[0], object)
}
