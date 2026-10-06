package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReadLockWriterWitnessSharesAllowance(t *testing.T) {
	pkg := writerBudgetPackage(t)
	function := pkg.Func("subject")
	pass := lockSetupPass(function, concurrencyfacts.NewEngine())
	reports := 0
	pass.Report = func(analysis.Diagnostic) { reports++ }
	setup := buildLockSetup(pass, function, proofs.NewSearchBudget(lockStateWorkBudget)).setup
	var receiver ssa.Value
	for _, effect := range setup.direct {
		if effect.operation == mutexAcquire {
			receiver = effect.receiver
		}
	}
	if receiver == nil || len(setup.defers) != 1 {
		t.Fatal("no read lock/writer defer")
	}
	identity := lockIdentityOf(receiver)
	stores := ssaflow.InstructionsOf[*ssa.Store](function)
	if len(stores) != 1 {
		t.Fatal("no single write")
	}
	for _, limit := range []int{0, 1, proofs.SummaryBudget} {
		budget := proofs.NewSearchBudget(limit)
		flow := lockFlowContext{pass: pass, setup: setup, budget: budget, readLockWrites: map[readLockWriteWitness]bool{}}
		reportReadLockWrites(flow, stores[0], []string{identity}, []string{identity}, map[string][]ssa.Value{identity: {receiver}}, setup.defers)
		if limit == 0 && !budget.Exhausted() {
			t.Fatal("writer witness bypassed zero request allowance")
		}
		if limit == proofs.SummaryBudget && budget.Exhausted() {
			t.Fatal("complete writer witness exhausted allowance")
		}
		if reports != 0 {
			t.Fatal("possible writer or interrupted witness reported read-lock mutation")
		}
	}
}

func TestWriterTemporalProofAllowance(t *testing.T) {
	pkg := writerBudgetPackage(t)
	for _, test := range []struct {
		name  string
		state proofs.EvidenceState
	}{
		{"subject", proofs.EvidenceProven},
		{"released", proofs.EvidenceDisproven},
		{"later", proofs.EvidenceDisproven},
		{"branched", proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			deferred := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
			store := ssaflow.InstructionsOf[*ssa.Store](function)[0]
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			baseline := possibleWriterAt(deferred, store, calls, nil)
			if baseline.State != test.state {
				t.Fatalf("default writer proof=%+v; want %v", baseline, test.state)
			}
			checkWriterAllowances(t, deferred, store, calls, baseline)
		})
	}
}

func checkWriterAllowances(t *testing.T, deferred *ssa.Defer, store *ssa.Store, calls []*ssa.Call, baseline proofs.Proof) {
	t.Helper()
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(proofs.SummaryBudget)
		budget := pool.Within(limit)
		proof := possibleWriterAt(deferred, store, calls, budget)
		if budget.Exhausted() {
			if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
				t.Fatalf("cut%d proof=%+v exhausted=%v/%v", limit, proof, budget.Exhausted(), pool.Exhausted())
			}
			if fresh := possibleWriterAt(deferred, store, calls, pool.Within(proofs.SummaryBudget)); fresh != baseline {
				t.Fatal("fresh writer query failed")
			}
			continue
		}
		if proof != baseline {
			t.Fatalf("complete writer proof=%+v; want %+v", proof, baseline)
		}
		return
	}
	t.Fatal("writer query never completed")
}

func writerBudgetPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "writerbudget", `package writerbudget
import "sync"
type owner struct { mu sync.RWMutex; writer sync.Mutex; count int }
func subject(o *owner){o.mu.RLock();defer o.writer.Unlock();o.count++}
func released(o *owner){defer o.writer.Unlock();o.writer.Unlock();o.count++}
func later(o *owner){o.count++;defer o.writer.Unlock()}
func branched(o *owner,yes bool){defer o.writer.Unlock();if yes{o.count++}}
`)
}
