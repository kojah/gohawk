package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedCleanupFactsForwardAcrossPackages(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type resource struct{}
func (*resource) Close() {}
func Base(r *resource) func() { return func() { r.Close() } }
func Forward(r *resource) func() { return Base(r) }
func Caller(r *resource) { defer Forward(r)() }
func Wrong(r, other *resource) { defer Forward(other)() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	base := summarize(pass, pkg.Func("Base"))
	if base.ReturnedCleanup == nil || len(base.ReturnedCleanup.Effects) != 1 || base.Closed != 0 {
		t.Fatalf("factory fact: %+v", base)
	}
	baseFunction := pkg.Func("Base")
	baseFunction.Blocks = nil
	pass.ImportObjectFact = func(object types.Object, fact analysis.Fact) bool {
		if object == baseFunction.Object() {
			if target, ok := fact.(*Fact); ok {
				*target = base
				return true
			}
		}
		return false
	}
	forward := pkg.Func("Forward")
	fact := summarize(pass, forward)
	if fact.ReturnedCleanup == nil || len(fact.ReturnedCleanup.Effects) != 1 || fact.Closed != 0 {
		t.Fatalf("forwarding factory fact: %+v", fact)
	}
	forward.Blocks = nil
	pass.ResultOf = map[*analysis.Analyzer]any{Analyzer: Summaries{forward: fact}}
	for _, name := range []string{"Caller", "Wrong"} {
		function := pkg.Func(name)
		invocation := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
		request := ssainfer.CompletionRequest{
			Instruction: invocation, Target: function.Params[0], Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1000),
		}
		proof := NewLifecycleEvidence(pass, "test", "test").Prove(EvidenceRequest{
			Instruction: invocation, Target: function.Params[0], Completion: &request,
		})
		if proof.Proven() != (name == "Caller") {
			t.Errorf("%s: %+v", name, proof)
		}
	}
}
