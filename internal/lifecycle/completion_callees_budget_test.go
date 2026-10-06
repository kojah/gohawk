package lifecycle

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestCompletionCalleeResolutionAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type resource struct{}
 func (*resource) Close(){}
 func (*resource) Touch(){}
 func literal(p *resource) { defer func(){p.Close()}() }
 func alternatives(p *resource, yes bool) {
  f:=func(){p.Close()}; if yes { f=func(){p.Close()} }; defer f()
 }
 func mixed(p *resource, yes bool) {
  f:=func(){p.Close()}; if yes { f=func(){p.Touch()} }; defer f()
 }
 func opaque(p *resource, f func(), yes bool) {
  if yes { f=func(){p.Close()} }; defer f()
 }
 func stored(p *resource) {
  f:=func(){p.Close()}; keep:=func(){f()}; _=keep; defer f()
 }
 func replaced(p *resource) {
  f:=func(){p.Close()}; keep:=func(){f=func(){}}; defer keep(); defer f()
 }
 `)
	for _, test := range []struct {
		name      string
		count     int
		completes bool
	}{
		{"literal", 1, true},
		{"alternatives", 2, true},
		{"mixed", 2, false},
		{"opaque", 0, false},
		{"stored", 1, true},
		{"replaced", 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var launch *ssa.Defer
			for instruction := range ssaflow.InstructionsWithin(fn, nil) {
				if deferred, ok := instruction.(*ssa.Defer); ok {
					launch = deferred
				}
			}
			if launch == nil {
				t.Fatal("missing deferred invocation")
			}
			baseline, ok := resolveCallees(launch, nil)
			if ok != (test.count > 0) || len(baseline) != test.count {
				t.Fatalf("baseline targets=%d available=%v", len(baseline), ok)
			}
			request := CompletionRequest{Instruction: launch, Target: fn.Params[0], Methods: []string{"Close"}}
			if test.name == "alternatives" || test.name == "stored" {
				budget := proofs.NewSearchBudget(2)
				callbacks, resolved := exactCallbacks(launch.Common().Value, launch, false, budget)
				if resolved || len(callbacks) != 0 || !budget.Exhausted() {
					t.Fatalf("origin cutoff callbacks=%d resolved=%v exhausted=%v", len(callbacks), resolved, budget.Exhausted())
				}
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v", proof)
			}
			pool := proofs.NewSearchBudget(200000)
			assertCompletionCalleeCutoffs(t, launch, baseline, ok, pool)
			request.Budget = pool.Within(proofs.QueryBudget)
			freshProof := ProveCompletion(request)
			if request.Budget.Exhausted() || freshProof.Proof != proof.Proof {
				t.Fatalf("fresh completion=%+v baseline=%+v", freshProof, proof)
			}
		})
	}
}

func assertCompletionCallees(t *testing.T, got []completionCallee, available bool, want []completionCallee, wantAvailable bool) {
	t.Helper()
	if available != wantAvailable || len(got) != len(want) {
		t.Fatalf("targets=%d/%d available=%v/%v", len(got), len(want), available, wantAvailable)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("target %d changed identity, order or launch", i)
		}
	}
}

func TestCompletionPaddedCalleeOrigins(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package ssaflowtest
type resource struct{}
func (*resource) Close(){}
func padded(p *resource, bits uint64){
f:=func(){p.Close()}
`)
	for i := range 40 {
		fmt.Fprintf(&source, "if bits & (1<<%d) != 0 { f=func(){p.Close()} }\n", i)
	}
	source.WriteString("defer f()\n}\n")
	pkg := buildTestSSA(t, source.String())
	fn := pkg.Func("padded")
	var launch *ssa.Defer
	for instruction := range ssaflow.InstructionsWithin(fn, nil) {
		if deferred, ok := instruction.(*ssa.Defer); ok {
			launch = deferred
		}
	}
	baseline, ok := resolveCallees(launch, nil)
	if !ok || len(baseline) != 41 {
		t.Fatalf("baseline targets=%d available=%v", len(baseline), ok)
	}
	pool := proofs.NewSearchBudget(200000)
	originsBudget := pool.Within(30)
	callbacks, resolved := exactCallbacks(launch.Common().Value, launch, false, originsBudget)
	if resolved || len(callbacks) != 0 || !originsBudget.Exhausted() {
		t.Fatalf("padded origin cutoff callbacks=%d resolved=%v exhausted=%v", len(callbacks), resolved, originsBudget.Exhausted())
	}
	child := pool.Within(30)
	targets, available := resolveCallees(launch, child)
	if available || len(targets) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut targets=%d available=%v exhausted=%v", len(targets), available, child.Exhausted())
	}
	targets, available = resolveCallees(launch, pool.Within(proofs.SummaryBudget))
	assertCompletionCallees(t, targets, available, baseline, ok)
	request := CompletionRequest{Instruction: launch, Target: fn.Params[0], Methods: []string{"Close"}, Budget: pool.Within(30)}
	if proof := ProveCompletion(request); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("cut proof=%+v", proof)
	}
}

func assertCompletionCalleeCutoffs(t *testing.T, launch ssa.Instruction, baseline []completionCallee, ok bool, pool *proofs.SearchBudget) {
	t.Helper()

	finished := false
	for limit := range proofs.SummaryBudget {
		child := pool.Within(limit)
		targets, available := resolveCallees(launch, child)
		if child.Exhausted() {
			if available || len(targets) != 0 {
				t.Fatalf("cut%d publishes %d targets, available=%v", limit, len(targets), available)
			}
		} else {
			assertCompletionCallees(t, targets, available, baseline, ok)
			finished = true
		}
		fresh := pool.Within(proofs.SummaryBudget)
		targets, available = resolveCallees(launch, fresh)
		if fresh.Exhausted() || pool.Exhausted() {
			t.Fatal("fresh or parent exhausted")
		}
		assertCompletionCallees(t, targets, available, baseline, ok)
		if finished {
			break
		}
	}
	if !finished {
		t.Fatal("resolution never completed")
	}
}
