package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestFreshBindingIdentityAndLoopBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "bindingidentity", `package bindingidentity
 import "sync"
 type owner struct{mu sync.Mutex}
 func take(value *owner){value.mu.Lock();value.mu.Unlock()}
 func plain(){take(new(owner))}
 func repeated(n int){for i:=0;i<n;i++{take(new(owner))}}
 `)
	lock := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("take"))[0]
	effect, known := directMutexEffectWithin(lock, nil)
	if !known {
		t.Fatal("missing helper lock")
	}
	for _, name := range []string{"plain", "repeated"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
			bound := bindLockAcquisition(effect.acquired, call)
			if bound.class != bound.instance || bound.widened {
				t.Fatalf("fresh identity/class disagree: %+v", bound)
			}
			if (bound.instance != "") != (name == "plain") {
				t.Fatalf("loop boundary: %+v", bound)
			}
		})
	}
}

const conditionIdentityFixture = `package state
func computed() bool
func direct(flag bool) bool { return flag }
func named(flag namedBool) namedBool { return flag }
type namedBool bool
func number(n int) int { return n }
func once(flag bool) bool { return !flag }
func loaded(flag *bool) bool { return *flag }
func called() bool { return computed() }
func merged(a, b, choose bool) bool { value := a; if choose { value = b }; return value }
func repeated(flag bool) { for computed() { if !flag { break } } }
func equal(a,b int) bool { return a == b }
func unequal(a,b int) bool { return a != b }
func ordered(a,b int) bool { return a < b }
`

func TestConditionIdentityStableBooleanSources(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "state", conditionIdentityFixture)
	for _, test := range []struct {
		name   string
		prefix string
	}{
		{"direct", "boolean:"},
		{"named", "boolean:"},
		{"number", ""},
		{"once", "boolean:"},
		{"loaded", "boolean:"},
		{"called", "boolean:"},
		{"merged", "boolean:"},
		{"equal", "==:"},
		{"unequal", "!=:"},
		{"ordered", ""},
	} {
		fn := pkg.Func(test.name)
		value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		identity, known := conditionIdentity(value, nil)
		if known != (test.prefix != "") || !strings.HasPrefix(identity, test.prefix) {
			t.Errorf("%s: identity %q, known=%t", test.name, identity, known)
		}
		if test.prefix == "boolean:" && identity != "boolean:"+conditionOperandIdentity(value) {
			t.Errorf("%s: did not preserve exact SSA identity", test.name)
		}
	}
	fn := pkg.Func("repeated")
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if identity, known := conditionIdentity(call, nil); known || identity != "" {
			t.Errorf("cyclic computation became stable: %q/%t", identity, known)
		}
	}
	// The parameter stays stable even when an instruction using it is in a loop.
	if identity, known := conditionIdentity(fn.Params[0], nil); !known || !strings.HasPrefix(identity, "boolean:") {
		t.Errorf("loop parameter: %q/%t", identity, known)
	}
	if identity, known := conditionIdentity(nil, nil); known || identity != "" {
		t.Errorf("nil condition: %q/%t", identity, known)
	}
}

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

func TestSlotBindingCutoffVetoAndFreshRecovery(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "slotbindings", `package slotbindings
 import "sync"
 type owner struct{mu *sync.Mutex}
 var shared sync.Mutex
 func observe(o *owner){}
 func replace(o *owner){o.mu=&shared}
 func plain(o *owner){observe(o);o.mu.Lock()}
 func changed(o *owner){replace(o);o.mu.Lock()}
 `)
	for _, test := range []struct {
		name        string
		replacement bool
	}{{"plain", false}, {"changed", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			field := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
			pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
			cut := pool.Within(0)
			if !boundSlotMutation(call, field, heapmodel.NewStorage(cut), cut) || !cut.Exhausted() || pool.Exhausted() {
				t.Fatalf("cut binding did not veto freshness: exhausted=%v/%v", cut.Exhausted(), pool.Exhausted())
			}
			fresh := pool.Within(proofs.QueryBudget)
			if got := boundSlotMutation(call, field, heapmodel.NewStorage(fresh), fresh); got != test.replacement || fresh.Exhausted() {
				t.Fatalf("fresh replacement=%v want=%v exhausted=%v", got, test.replacement, fresh.Exhausted())
			}
		})
	}
}

func TestFreshOwnerResultCensusCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "freshresults", `package freshresults
 type owner struct{value int}
 var shared owner
 func fresh()*owner{return new(owner)}
 func borrowed()*owner{return &shared}
 func mixed(flag bool)*owner{if flag{return new(owner)};return &shared}
 func subject(flag bool){fresh();borrowed();mixed(flag)}
 `)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for index, call := range calls {
		pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
		cut := pool.Within(0)
		if freshOwnerResult(call, cut) || !cut.Exhausted() || pool.Exhausted() {
			t.Fatalf("constructor %d accepted partial census", index)
		}
		fresh := pool.Within(proofs.QueryBudget)
		if got := freshOwnerResult(call, fresh); got != (index == 0) || fresh.Exhausted() {
			t.Fatalf("constructor %d freshness=%v exhausted=%v", index, got, fresh.Exhausted())
		}
	}
}
