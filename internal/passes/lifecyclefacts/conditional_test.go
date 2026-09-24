package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestConditionalFactsComposeWithoutWideningMasks(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type resource struct{}
func (*resource) Close() {}
func Base(r *resource, yes bool) bool { if yes { r.Close(); return true }; return false }
func Forward(r *resource, yes bool) bool { return Base(r, yes) }
func Wrong(r, other *resource, yes bool) bool { return Base(other, yes) }
func Partial(r *resource, yes, skip bool) bool { if yes { if !skip { r.Close() }; return true }; return false }
func Async(r *resource, yes bool) bool { if yes { go r.Close(); return true }; return false }
func Invoke(fn func(), yes bool) bool { if yes { fn(); return true }; return false }
func Caller(r *resource, yes bool) { if Forward(r, yes) { return }; r.Close() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	base := summarize(pass, pkg.Func("Base"))
	predicate := ssainfer.CompletionPredicate{Outcome: ssainfer.CompletionWhenTrue}
	if base.Closed != 0 || conditionalMask(base, "Close", false, predicate) != parameterMaskFor(0) {
		t.Fatalf("base = %+v, conditional = %+v", base, base.Conditional)
	}
	// Erase dependency SSA to require the serialized fact rather than a local
	// body walk. Forwarding must export the relation in its own parameter space.
	baseFunction := pkg.Func("Base")
	baseFunction.Blocks = nil
	pass.ImportObjectFact = func(object types.Object, fact analysis.Fact) bool {
		if object != baseFunction.Object() {
			return false
		}
		if target, ok := fact.(*Fact); ok {
			*target = base
			return true
		}
		return false
	}
	for _, test := range []struct {
		name string
		mask ParameterMask
	}{
		{"Forward", parameterMaskFor(0)},
		{"Wrong", parameterMaskFor(1)},
		{"Partial", 0},
		{"Async", 0},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		if got := conditionalMask(fact, "Close", false, predicate); got != test.mask || fact.Closed != 0 {
			t.Errorf("%s: conditional %x, unconditional %x, want %x / 0", test.name, got, fact.Closed, test.mask)
		}
	}
	forward := pkg.Func("Forward")
	forwardFact := summarize(pass, forward)
	forward.Blocks = nil
	pass.ResultOf = map[*analysis.Analyzer]any{Analyzer: Summaries{forward: forwardFact}}
	caller := pkg.Func("Caller")
	branch := ssaflow.InstructionsOf[*ssa.If](caller)[0].Block()
	evidence := NewLifecycleEvidence(pass, "test", "test")
	request := ssainfer.CompletionRequest{Target: caller.Params[0], Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1000)}
	if proof := evidence.CompletionOnEdge(branch, branch.Succs[0], request); !proof.Proven() || proof.Provenance != ssaflow.EvidenceFromImportedFact {
		t.Fatalf("imported true edge: %+v", proof)
	}
	if proof := evidence.CompletionOnEdge(branch, branch.Succs[1], request); proof.Proven() {
		t.Fatalf("imported false edge: %+v", proof)
	}
	invoke := summarize(pass, pkg.Func("Invoke"))
	if invoke.SynchronouslyInvoked != 0 || conditionalMask(invoke, "", true, predicate) != parameterMaskFor(0) {
		t.Fatalf("invocation: %+v", invoke)
	}
}

func TestRowsNextResultSetConditionalRelease(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
import "database/sql"
func Advance(rows *sql.Rows) bool { return rows.NextResultSet() }
func Forward(rows *sql.Rows) bool { return Advance(rows) }
func Wrong(rows, other *sql.Rows) bool { return other.NextResultSet() }
func Next(rows *sql.Rows) bool { return rows.Next() }
type fakeRows struct{}
func (*fakeRows) NextResultSet() bool { return false }
func (*fakeRows) Close() error { return nil }
func Fake(rows *fakeRows) bool { return rows.NextResultSet() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	falseResult := ssainfer.CompletionPredicate{Outcome: ssainfer.CompletionWhenFalse}
	trueResult := ssainfer.CompletionPredicate{Outcome: ssainfer.CompletionWhenTrue}
	for _, test := range []struct {
		name string
		mask ParameterMask
	}{
		{"Advance", parameterMaskFor(0)},
		{"Forward", parameterMaskFor(0)},
		{"Wrong", parameterMaskFor(1)},
		{"Next", 0},
		{"Fake", 0},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		if got := conditionalMask(fact, "Close", false, falseResult); got != test.mask || fact.Closed != 0 {
			t.Errorf("%s: false-edge mask %x, unconditional mask %x, want %x / 0", test.name, got, fact.Closed, test.mask)
		}
		if got := conditionalMask(fact, "Close", false, trueResult); got != 0 {
			t.Errorf("%s: true-edge mask %x, want 0", test.name, got)
		}
	}
}
