package ssainfer

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedCleanupCacheKeepsFixedPolicy(t *testing.T) {
	pkg := buildTestSSA(t, returnedCleanupFixture)
	factory, caller := pkg.Func("cleanup"), pkg.Func("direct")
	factory.Blocks = nil
	lookups := 0
	lookup := func(function *ssa.Function, method string, invoke bool) []ReturnedCleanupRelation {
		lookups++
		if function == factory && method == "Close" && !invoke {
			return []ReturnedCleanupRelation{{CallbackResult: 0, Target: 0}}
		}
		return nil
	}
	evidence := NewLocalEvidenceWithReturnedCleanup(lookup)
	request := CompletionRequest{
		Instruction: ssaflow.InstructionsOf[*ssa.Defer](caller)[0], Target: caller.Params[0], Methods: []string{"Close"},
		Budget: ssaflow.NewSearchBudget(1000),
	}
	if proof := evidence.Completion(request); !proof.Proven() {
		t.Fatalf("fixed imported relation unavailable: %+v", proof)
	}
	previous := lookups
	request.Budget = ssaflow.NewSearchBudget(0)
	if proof := evidence.Completion(request); !proof.Proven() || lookups != previous {
		t.Fatalf("completed proof not cached: %+v, lookups %d -> %d", proof, previous, lookups)
	}
	request.Budget = ssaflow.NewSearchBudget(1000)
	request.ReturnedSummaries = func(*ssa.Function, string, bool) []ReturnedCleanupRelation { return nil }
	if proof := evidence.Completion(request); proof.Proven() {
		t.Fatalf("request override reused fixed-policy proof: %+v", proof)
	}
	var localOnly LocalEvidence
	request.ReturnedSummaries = nil
	if proof := localOnly.Completion(request); proof.Proven() {
		t.Fatalf("local-only scope reused imported proof: %+v", proof)
	}
}
