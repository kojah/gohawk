package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"

	"github.com/kojah/gohawk/internal/analyzertest"
	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestInitialPublicationOrdering(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "initialpublication")
}

func TestInitialPublicationGuardBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "publication", `package publication
 import "sync"
 type registry struct{mu sync.RWMutex;extra sync.Mutex;entries map[string]*sync.Mutex}
 type writer struct{mu sync.Mutex}
 var saved *sync.Mutex
 func observe(*sync.Mutex){}
 func plain(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);r.entries[k]=g;g.Lock()}
 func reader(r *registry,k string){r.mu.RLock();g:=new(sync.Mutex);r.entries[k]=g;g.Lock()}
 func called(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);observe(g);r.entries[k]=g;g.Lock()}
 func previous(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);g.Lock();g.Unlock();r.entries[k]=g;g.Lock()}
 func lateGuard(r *registry,k string){g:=new(sync.Mutex);r.entries[k]=g;r.mu.Lock();g.Lock()}
 func stored(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);saved=g;r.entries[k]=g;g.Lock()}
 func twice(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);r.entries[k]=g;r.entries["other"]=g;g.Lock()}
 func released(r *registry,k string){r.mu.Lock();g:=new(sync.Mutex);r.entries[k]=g;r.mu.Unlock();g.Lock()}
 func loaded(r *registry,k string){r.mu.Lock();g:=r.entries[k];g.Lock()}
 func other(r *registry,w *writer,k string){w.mu.Lock();g:=new(sync.Mutex);r.entries[k]=g;g.Lock()}
 func both(r *registry,k string){r.mu.Lock();r.extra.Lock();g:=new(sync.Mutex);r.entries[k]=g;g.Lock()}
 func unrelated(r *registry,w *writer,k string){w.mu.Lock();r.mu.Lock();g:=new(sync.Mutex);r.entries[k]=g;g.Lock()}
 `)
	query := func(name string, budget *proofs.SearchBudget, uncertain bool) publicationGuardProof {
		function := pkg.Func(name)
		calls := ssaflow.InstructionsOf[*ssa.Call](function)
		target := calls[len(calls)-1]
		flow := lockFlowContext{
			budget: budget, setup: &lockFunctionSetup{direct: map[ssa.Instruction]mutexEffect{}},
			lockValues: map[string][]ssa.Value{}, unprovenRelease: map[string]bool{},
		}
		state := lockFlowState{}
		for _, call := range calls {
			effect, known := directMutexEffectWithin(call, nil)
			if !known {
				continue
			}
			flow.setup.direct[call] = effect
			if call == target || effect.operation != mutexAcquire {
				continue
			}
			state.held = appendUniqueString(state.held, effect.identity)
			flow.lockValues[effect.identity] = appendLockValue(flow.lockValues[effect.identity], effect.receiver, nil)
			flow.unprovenRelease[effect.identity] = uncertain
			if effect.acquired.read {
				state.readHeld = appendUniqueString(state.readHeld, effect.identity)
			}
		}
		proof := flow.initialPublicationGuard(target, flow.setup.direct[target].receiver, state)
		if name == "unrelated" && proof.identity != state.held[1] {
			t.Errorf("excluded unrelated held writer: %+v", proof)
		}
		return proof
	}
	for _, test := range []struct {
		name    string
		unknown bool
	}{
		{"plain", true},
		{"reader", false},
		{"called", false},
		{"previous", false},
		{"lateGuard", false},
		{"stored", false},
		{"twice", false},
		{"released", false},
		{"loaded", false},
		{"other", false},
		{"both", false},
		{"unrelated", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			proof := query(test.name, proofs.NewSearchBudget(lockStateWorkBudget), false)
			if (proof.state == proofs.EvidenceUnknown) != test.unknown {
				t.Fatalf("publication = %+v, unknown want %v", proof, test.unknown)
			}
		})
	}
	if proof := query("plain", proofs.NewSearchBudget(lockStateWorkBudget), true); proof.state == proofs.EvidenceUnknown {
		t.Fatal("possibly released writer supplied publication evidence")
	}
	pool := proofs.NewSearchBudget(lockStateWorkBudget)
	cut := query("plain", pool.Within(0), false)
	if cut.state != proofs.EvidenceUnknown || cut.reason != lockReasonLockStateBudgetExhausted || cut.identity != "" {
		t.Fatalf("cutoff supplies guard identity: %+v", cut)
	}
	fresh := query("plain", pool.Within(lockStateWorkBudget), false)
	if fresh.state != proofs.EvidenceUnknown || fresh.reason != lockReasonInitialPublicationUnknown || fresh.identity == "" {
		t.Fatalf("fresh allowance did not recover publication evidence: %+v", fresh)
	}
}
