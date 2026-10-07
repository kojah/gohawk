package lockorder

import (
	"go/types"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestReadLockFieldContractsUnknown(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "readlockfields")
}

func TestReadLockFieldEvidenceAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fieldcontracts", `package fieldcontracts
 type state struct{a,b int;peer *state}
 type other struct{a int}
 func opaque(){}
 func(s *state) reset(){s.a=0}
 func(s *state) called(){s.b=0;opaque()}
 func(s *state) loaded(){s.peer.b=0}
 func(o *other) reset(){o.a=0}
 func(s *state) inspect(){s.a++;s.b++}
 `)
	method := func(name, member string) *ssa.Function {
		named := pkg.Pkg.Scope().Lookup(name).Type().(*types.Named)
		for object := range named.Methods() {
			if object.Name() == member {
				return pkg.Prog.FuncValue(object)
			}
		}
		t.Fatalf("missing method %s.%s", name, member)
		return nil
	}
	functions := []*ssa.Function{method("state", "reset"), method("state", "called"), method("state", "loaded"), method("other", "reset")}
	stores := ssaflow.InstructionsOf[*ssa.Store](method("state", "inspect"))
	pool := proofs.NewSearchBudget(lockStateWorkBudget)
	fields := collectReadLockFieldEvidence(functions, pool)
	if fields.unavailable || fields.guard(stores[0]).state != proofs.EvidenceUnknown || (fields.guard(stores[1]).state == proofs.EvidenceUnknown) {
		t.Fatalf("field-specific evidence = %+v", fields)
	}
	otherOnly := collectReadLockFieldEvidence(functions[3:], proofs.NewSearchBudget(lockStateWorkBudget))
	if otherOnly.guard(stores[0]).state == proofs.EvidenceUnknown {
		t.Fatal("same field spelling on another type supplies evidence")
	}
	cut := collectReadLockFieldEvidence(functions, pool.Within(0))
	if !cut.unavailable || cut.guard(stores[0]).state != proofs.EvidenceUnknown || cut.guard(stores[1]).state != proofs.EvidenceUnknown {
		t.Fatal("cutoff supplied negative field evidence")
	}
	fresh := collectReadLockFieldEvidence(functions, pool.Within(lockStateWorkBudget))
	if fresh.unavailable || fresh.guard(stores[0]).state != proofs.EvidenceUnknown || (fresh.guard(stores[1]).state == proofs.EvidenceUnknown) {
		t.Fatal("fresh allowance failed to recover field-specific evidence")
	}
}

func TestReadLockWritesPrivateOwners(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "privateread")
}

func TestReadLockWritesConvergingPaths(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "readlockpaths")
}

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
