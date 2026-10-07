package lockorder

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestExclusiveParameterRequiresCompleteCallerSet(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"freshDirect", "", true},
		{"sharedDirect", "func other(){target(shared)}", false},
		{"sharedGo", "func other(){go target(shared)}", false},
		{"sharedDefer", "func other(){defer target(shared)}", false},
		{"callback", "var callback=target", false},
		{"initialization", "var initialized=initialize();func initialize()int{target(shared);return 0}", false},
		{"callerLimit", "func many(){" + strings.Repeat("target(&box{});", 31) + "}", true},
		{"tooManyCallers", "func many(){" + strings.Repeat("target(&box{});", 32) + "}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "exclusivecallers", `package exclusivecallers
type box struct{value int}
var shared=&box{}
var published *box
func target(value *box){value.value++;published=value}
func direct(){target(&box{})}
`+test.extra)
			inventory := collectLockCallers(pkg.Func("init"), ssaflow.DeclaredFunctions(pkg), nil)
			callers := newExclusiveCallers(nil, inventory)
			var dump strings.Builder
			for _, fn := range []*ssa.Function{pkg.Func("init"), pkg.Func("direct"), pkg.Func("other")} {
				if fn != nil {
					if _, err := fn.WriteTo(&dump); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Log(dump.String())
			if got := callers.parameterExclusive(pkg.Func("target"), 0, nil).state == proofs.EvidenceProven; got != test.want {
				t.Fatalf("parameter exclusivity = %v, want %v", got, test.want)
			}
			cutoff := collectLockCallers(pkg.Func("init"), ssaflow.DeclaredFunctions(pkg), proofs.NewSearchBudget(0))
			if newExclusiveCallers(nil, cutoff).parameterExclusive(pkg.Func("target"), 0, nil).state == proofs.EvidenceProven {
				t.Fatal("incomplete caller inventory proved parameter exclusivity")
			}
		})
	}
}

func TestExclusiveMethodCallerSetRemainsUnknown(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "exclusivemethod", `package exclusivemethod
type box struct{value int}
type receiver struct{}
type worker interface{run(*box)}
var dynamic worker=receiver{}
func (receiver)run(value *box){value.value++}
func direct(){receiver{}.run(&box{})}
func other(value *box){dynamic.run(value)}
`)
	functions := ssaflow.DeclaredFunctions(pkg)
	inventory := collectLockCallers(pkg.Func("init"), functions, nil)
	for _, function := range functions {
		if function.Name() == "run" {
			if newExclusiveCallers(nil, inventory).parameterExclusive(function, 1, nil).state == proofs.EvidenceProven {
				t.Fatal("fresh method call hid an unmodeled interface caller")
			}
			return
		}
	}
	t.Fatal("missing method SSA")
}

func TestExclusiveCallerRequestAllowance(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"fresh", "func second(){target(&box{})}", true},
		{"shared", "var shared=&box{};func second(){target(shared)}", false},
		{"callback", "var callback=target", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "exclusivebudget", `package exclusivebudget
 import "sync"
 type box struct{mu sync.Mutex}
 var published *box
 func target(value *box){value.mu.Lock();value.mu.Unlock();published=value}
 func first(){target(&box{})}
 `+test.extra)
			function := pkg.Func("target")
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			inventory := collectLockCallers(pkg.Func("init"), ssaflow.DeclaredFunctions(pkg), nil)
			callers := newExclusiveCallers(nil, inventory)
			budget := proofs.NewSearchBudget(0)
			proof := callers.parameterExclusive(function, 0, budget)
			if proof.state == proofs.EvidenceProven || proof.reason != lockReasonLockStateBudgetExhausted || !budget.Exhausted() {
				t.Fatalf("caller allowance bypass: %+v", proof)
			}
			if len(callers.exclusive) != 0 {
				t.Fatal("interrupted proof entered cache")
			}
			defaultProof := callers.parameterExclusive(function, 0, nil)
			if (defaultProof.state == proofs.EvidenceProven) != test.want {
				t.Fatalf("default proof %+v", defaultProof)
			}
			checkExclusiveAllowances(t, inventory, function, defaultProof)
			// A cache hit still belongs to a live request, but needs no caller rescan.
			if cached := callers.parameterExclusive(function, 0, proofs.NewSearchBudget(1)); cached != defaultProof {
				t.Fatal("cache hit changed proof")
			}
			if cut := callers.parameterExclusive(function, 0, proofs.NewSearchBudget(0)); cut.reason != lockReasonLockStateBudgetExhausted {
				t.Fatal("cache bypassed admission")
			}
			checkExclusiveAcquisitionAllowances(t, inventory, function, test.want)
		})
	}
}

func checkExclusiveAllowances(t *testing.T, inventory map[*ssa.Function]conditionalCallerSet, function *ssa.Function, want exclusiveProof) {
	t.Helper()
	for limit := range 16 {
		callers := newExclusiveCallers(nil, inventory)
		pool := proofs.NewSearchBudget(128)
		budget := pool.Within(limit)
		proof := callers.parameterExclusive(function, 0, budget)
		if budget.Exhausted() {
			if proof.reason != lockReasonLockStateBudgetExhausted || len(callers.exclusive) != 0 || pool.Exhausted() {
				t.Fatalf("cut%d leaked proof/cache: %+v", limit, proof)
			}
			if fresh := callers.parameterExclusive(function, 0, pool.Within(16)); fresh != want {
				t.Fatalf("fresh proof %+v want %+v", fresh, want)
			}
			continue
		}
		if proof != want {
			t.Fatalf("complete proof %+v want %+v", proof, want)
		}
		return
	}
	t.Fatal("caller proof never completes")
}

func checkExclusiveAcquisitionAllowances(t *testing.T, inventory map[*ssa.Function]conditionalCallerSet, function *ssa.Function, want bool) {
	t.Helper()
	call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	receiver := ssaflow.CallReceiver(call.Common())
	defaultProof := newExclusiveCallers(nil, inventory).acquisitionExclusive(function, call, receiver, nil)
	if (defaultProof.state == proofs.EvidenceProven) != want {
		t.Fatalf("acquisition proof %+v", defaultProof)
	}
	for limit := range 16 {
		callers := newExclusiveCallers(nil, inventory)
		pool := proofs.NewSearchBudget(128)
		budget := pool.Within(limit)
		proof := callers.acquisitionExclusive(function, call, receiver, budget)
		if budget.Exhausted() {
			if proof.reason != lockReasonLockStateBudgetExhausted || len(callers.exclusive) != 0 || pool.Exhausted() {
				t.Fatalf("cut%d leaked acquisition/cache: %+v", limit, proof)
			}
			if fresh := callers.acquisitionExclusive(function, call, receiver, pool.Within(16)); fresh != defaultProof {
				t.Fatal("fresh acquisition differs")
			}
			continue
		}
		if proof != defaultProof {
			t.Fatal("complete acquisition differs")
		}
		return
	}
	t.Fatal("acquisition never completes")
}
