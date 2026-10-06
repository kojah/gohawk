package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockReleaseQueryCoverageAndLaunch(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "lockrelease", `package lockrelease
 import "sync"
 func yes(mu *sync.Mutex){mu.Unlock()}
 func maybe(mu *sync.Mutex,flag bool){if flag{mu.Unlock()}}
 func none(mu *sync.Mutex){}
 func called(mu *sync.Mutex,flag bool,opaque func(*sync.Mutex)){
 yes(mu);maybe(mu,flag);none(mu);opaque(mu);go yes(mu);defer maybe(mu,flag)
 }
 `)
	fn := pkg.Func("called")
	calls := ssaflow.InstructionsOf[ssa.CallInstruction](fn)
	if len(calls) != 6 {
		t.Fatalf("calls=%d", len(calls))
	}
	for _, test := range []struct {
		index    int
		coverage lifecycle.CompletionCoverage
		reason   proofs.EvidenceReason
		proven   bool
	}{
		{0, lifecycle.CoverageEveryReturn, proofs.EvidenceCalledCompletion, true},
		{1, lifecycle.CoverageEveryReturn, proofs.EvidenceNone, false},
		{1, lifecycle.CoverageAnywhere, proofs.EvidenceCalledCompletion, true},
		{2, lifecycle.CoverageEveryReturn, proofs.EvidenceNone, false},
		{3, lifecycle.CoverageEveryReturn, proofs.EvidenceNone, false},
		{4, lifecycle.CoverageEveryReturn, proofs.EvidenceStartedCompletion, true},
		{5, lifecycle.CoverageAnywhere, proofs.EvidenceDeferredCompletion, true},
	} {
		var evidence lifecycle.LocalEvidence
		queries := newLockReleaseQueries(&evidence, proofs.NewSearchBudget(proofs.SummaryBudget))
		proof := queries.query(calls[test.index], fn.Params[0], test.coverage, nil)
		if proof.Proven() != test.proven || test.proven && !releaseSettled(proof, test.reason) || queries.cutoff.Reason != proofs.EvidenceNone {
			t.Fatalf("call%d coverage%d proof=%+v cutoff=%+v", test.index, test.coverage, proof, queries.cutoff)
		}
		if test.index == 3 && proof.State != proofs.EvidenceUnknown {
			t.Fatalf("opaque callback became known: %+v", proof)
		}
	}
}

func TestLockReleaseQueryCutoffAndFreshEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "lockreleasebudget", `package lockreleasebudget
 import "sync"
 var total int
 func release(mu *sync.Mutex){`+strings.Repeat("total++\n", 20)+`mu.Unlock()}
 func called(mu *sync.Mutex){release(mu)}
 `)
	fn := pkg.Func("called")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	completed := false
	for limit := range proofs.SummaryBudget {
		var evidence lifecycle.LocalEvidence
		pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
		child := pool.Within(limit)
		queries := newLockReleaseQueries(&evidence, child)
		proof := queries.query(call, fn.Params[0], lifecycle.CoverageEveryReturn, nil)
		if !child.Exhausted() {
			completed = true
			break
		}
		if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
			t.Fatalf("cut%d proof=%+v pool=%v", limit, proof, pool.Exhausted())
		}
		fresh := newLockReleaseQueries(&evidence, pool.Within(proofs.SummaryBudget))
		if proof := fresh.query(call, fn.Params[0], lifecycle.CoverageEveryReturn, nil); !releaseSettled(proof, proofs.EvidenceCalledCompletion) {
			t.Fatalf("fresh at%d: %+v", limit, proof)
		}
		warm := newLockReleaseQueries(&evidence, pool.Within(0))
		if proof := warm.query(call, fn.Params[0], lifecycle.CoverageEveryReturn, nil); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
			t.Fatalf("warm zero allowance at%d: %+v", limit, proof)
		}
	}
	if !completed {
		t.Fatal("completion never finished")
	}
	// A question's local cap can fail while its function pool remains usable.
	// That failure must still make the work list's publication barrier unavailable.
	var cold lifecycle.LocalEvidence
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	queries := newLockReleaseQueries(&cold, pool)
	queries.limit = 1
	proof := queries.query(call, fn.Params[0], lifecycle.CoverageEveryReturn, nil)
	walk := lockStateWalk{budget: pool, flow: lockFlowContext{releases: queries}}
	if proof.Proven() || releaseSettled(proof, proofs.EvidenceCalledCompletion) ||
		queries.cutoff.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() || !walk.incomplete() {
		t.Fatalf("local cap proof=%+v cutoff=%+v pool=%v", proof, queries.cutoff, pool.Exhausted())
	}
	retry := newLockReleaseQueries(&cold, pool)
	if proof := retry.query(call, fn.Params[0], lifecycle.CoverageEveryReturn, nil); !releaseSettled(proof, proofs.EvidenceCalledCompletion) {
		t.Fatalf("fresh after local cap: %+v", proof)
	}
}

func TestLockReleaseCutoffDiscardsEarlierFindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "lockreleaseflow", `package lockreleaseflow
 import "sync"
 var first,second sync.Mutex
 var shared struct {mu sync.RWMutex;value int}
 var total int
 func inspect(mu *sync.Mutex){`+strings.Repeat("total++\n", 30)+`}
 func root(){
 shared.mu.RLock();shared.value++;shared.mu.RUnlock()
 first.Lock();second.Lock();inspect(&second);second.Unlock();first.Unlock()
 }
 `)
	fixture := newLockWalkFixture(pkg.Func("root"))
	// Leave result summaries unavailable so this isolates release inference,
	// rather than cutting during the independent termination-result query.
	fixture.results = nil
	budget := proofs.NewSearchBudget(100)
	if ok, reports, edges := fixture.run(budget); ok || !budget.Exhausted() || len(reports) != 0 || edges != 0 {
		t.Fatalf("late release cutoff complete=%v reports=%v edges=%d exhausted=%v", ok, reports, edges, budget.Exhausted())
	}
	fresh := proofs.NewSearchBudget(lockStateWorkBudget)
	if ok, reports, edges := fixture.run(fresh); !ok || len(reports) != 1 || edges != 1 || fresh.Exhausted() {
		t.Fatalf("fresh release flow complete=%v reports=%v edges=%d", ok, reports, edges)
	}
}
