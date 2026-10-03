package lockorder

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
			budget := ssaflow.NewSearchBudget(0)
			proof := callers.parameterExclusive(function, 0, budget)
			if proof.state == ssaflow.EvidenceProven || proof.reason != lockReasonLockStateBudgetExhausted || !budget.Exhausted() {
				t.Fatalf("caller allowance bypass: %+v", proof)
			}
			if len(callers.exclusive) != 0 {
				t.Fatal("interrupted proof entered cache")
			}
			defaultProof := callers.parameterExclusive(function, 0, nil)
			if (defaultProof.state == ssaflow.EvidenceProven) != test.want {
				t.Fatalf("default proof %+v", defaultProof)
			}
			checkExclusiveAllowances(t, inventory, function, defaultProof)
			// A cache hit still belongs to a live request, but needs no caller rescan.
			if cached := callers.parameterExclusive(function, 0, ssaflow.NewSearchBudget(1)); cached != defaultProof {
				t.Fatal("cache hit changed proof")
			}
			if cut := callers.parameterExclusive(function, 0, ssaflow.NewSearchBudget(0)); cut.reason != lockReasonLockStateBudgetExhausted {
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
		pool := ssaflow.NewSearchBudget(128)
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
	if (defaultProof.state == ssaflow.EvidenceProven) != want {
		t.Fatalf("acquisition proof %+v", defaultProof)
	}
	for limit := range 16 {
		callers := newExclusiveCallers(nil, inventory)
		pool := ssaflow.NewSearchBudget(128)
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
