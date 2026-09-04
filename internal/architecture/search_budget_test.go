package architecture

import (
	"fmt"
	"go/ast"
	"maps"
	"testing"
)

// An interprocedural question walks from one call site into every callee it can
// resolve. The cycle guard keeps that walk finite, but a memo cannot retain an
// answer the guard cut short, so a densely recursive package is re-walked once
// per route rather than once per function. Twenty-two mutually recursive
// functions with four calls each were enough to stop the analysis finishing in
// a minute, and it reached a core check before anyone noticed.
//
// A budget makes the walk give up instead of hanging. It cannot be added
// centrally, because what an abandoned question permits depends on the polarity
// of the proof being sought: a walk that claims an obligation was discharged
// must not claim it on a guess, and a walk that claims one remains open must
// not invent one. The caller therefore chooses both the bound and the meaning
// of exhaustion, and this test keeps that choice from being forgotten.
//
// The rule says nothing about how large the bound should be. It only requires
// that somebody decided.

// unbudgetedCompletionRequests are the construction sites that predate the
// bound. They are recorded rather than fixed because each needs its own
// decision about what an abandoned search permits, and for the lifecyclefacts
// pair that decision is a fact-model question: a mask left clear by an
// abandoned walk reads to an importer as a positive disproof, not as absence of
// evidence. See https://github.com/kojah/gohawk/issues/32.
//
// Do not add entries. A new completion request names its own budget.
var unbudgetedCompletionRequests = map[string]int{
	"internal/analyzers/ownership/processownership/ownership.go": 4,
	"internal/passes/lifecyclefacts/analyzer.go":                 1,
	"internal/passes/lifecyclefacts/fields.go":                   1,
}

func TestInterproceduralSearchesNameABudget(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	remaining := maps.Clone(unbudgetedCompletionRequests)
	for _, source := range inventory.productionGoFiles(t, "internal") {
		ast.Inspect(source.file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !isCompletionRequestType(literal.Type) || compositeLitHasKey(literal, "Budget") {
				return true
			}
			if remaining[source.repositoryPath] > 0 {
				remaining[source.repositoryPath]--
				return true
			}
			position := source.fileSet.Position(literal.Pos())
			t.Errorf("%s:%d builds a CompletionRequest without a Budget; an unbounded interprocedural "+
				"search is exponential on mutually recursive callees. Set Budget and decide what an "+
				"exhausted search permits, as lockorder and resourcelifetime do",
				source.repositoryPath, position.Line)
			return true
		})
	}
	for path, count := range remaining {
		if count > 0 {
			t.Errorf("%s no longer has %s; remove the entry from unbudgetedCompletionRequests",
				path, fmt.Sprintf("%d unbudgeted CompletionRequest(s)", count))
		}
	}
}

// isCompletionRequestType reports whether the composite literal builds an
// ssaflow.CompletionRequest, named either through the package or directly from
// inside ssaflow.
func isCompletionRequestType(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name == "CompletionRequest"
	case *ast.SelectorExpr:
		package_, ok := typed.X.(*ast.Ident)
		return ok && package_.Name == "ssaflow" && typed.Sel.Name == "CompletionRequest"
	}
	return false
}

func compositeLitHasKey(literal *ast.CompositeLit, field string) bool {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if name, ok := keyed.Key.(*ast.Ident); ok && name.Name == field {
			return true
		}
	}
	return false
}
