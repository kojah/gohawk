package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBoundMutexIdentitySharesOperationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identitybudget", `package identitybudget
import "sync"
var global sync.Mutex
func subject(){global.Lock()}
`)
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	operations := []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock, Source: call.Pos()}}
	operations[0].Resource.Value = pkg.Var("global")
	// Operation selection, identity reaching/storage, and variant selection each
	// require work. Two visits cannot complete them by hiding identity queries.
	budget := proofs.NewSearchBudget(2)
	effects, complete := bindMutexEffects(call, operations, budget)
	if complete || len(effects) != 0 || !budget.Exhausted() {
		t.Fatalf("identity bypass: effects=%d complete=%v exhausted=%v", len(effects), complete, budget.Exhausted())
	}
}

func TestMutexIdentityAndReceiverAllowances(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identityforms", `package identityforms
import "sync"
var first,second sync.Mutex
type owner struct{mu sync.Mutex;pointer *sync.Mutex}
func (o *owner) Mutex()*sync.Mutex{return &o.mu}
func direct(){first.Lock()}
func field(o *owner){o.mu.Lock()}
func copied(o owner){o.pointer.Lock()}
func snapshot(o owner){slot:=new(owner);*slot=o;slot.pointer.Lock()}
func getter(o *owner){o.Mutex().Lock()}
func indexed(values [2]*sync.Mutex){values[0].Lock()}
func agreed(yes bool){var lock sync.Locker=&first;if yes{lock=&first};lock.Lock()}
func mixed(yes bool){var lock sync.Locker=&first;if yes{lock=&second};lock.Lock()}
`)
	for _, name := range []string{"direct", "field", "copied", "snapshot", "getter", "indexed", "agreed", "mixed"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			instruction := calls[len(calls)-1]
			operation, identity, receiver, known := mutexAction(instruction)
			if known != (name != "mixed") {
				t.Fatalf("default action known=%v identity=%q", known, identity)
			}
			checkMutexActionAllowances(t, instruction, operation, identity, receiver, known)
			if known {
				checkMutexIdentityAllowances(t, receiver, identity)
			}
		})
	}
}

func checkMutexActionAllowances(t *testing.T, instruction ssa.Instruction, operation mutexOperation, identity string, receiver ssa.Value, known bool) {
	t.Helper()
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
		budget := pool.Within(limit)
		gotOperation, gotIdentity, gotReceiver, gotKnown := mutexActionWithin(instruction, budget)
		if budget.Exhausted() {
			if gotKnown || gotIdentity != "" || gotReceiver != nil || pool.Exhausted() {
				t.Fatalf("cut%d supplied action identity=%q receiver=%v known=%v", limit, gotIdentity, gotReceiver, gotKnown)
			}
			_, freshIdentity, _, freshKnown := mutexActionWithin(instruction, pool.Within(proofs.SummaryBudget))
			if freshIdentity != identity || freshKnown != known {
				t.Fatal("fresh action differs from default")
			}
			continue
		}
		if gotOperation != operation || gotIdentity != identity || gotReceiver != receiver || gotKnown != known {
			t.Fatal("complete action differs from default")
		}
		return
	}
	t.Fatal("action never completes")
}

func checkMutexIdentityAllowances(t *testing.T, value ssa.Value, identity string) {
	t.Helper()
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
		budget := pool.Within(limit)
		got := lockIdentityWithin(value, budget)
		if budget.Exhausted() {
			if got != "" || pool.Exhausted() {
				t.Fatal("cutoff supplied identity or exhausted pool")
			}
			if fresh := lockIdentityWithin(value, pool.Within(proofs.SummaryBudget)); fresh != identity {
				t.Fatal("fresh identity differs from default")
			}
			continue
		}
		if got != identity {
			t.Fatalf("identity=%q; want %q", got, identity)
		}
		return
	}
	t.Fatal("identity never completes")
}
