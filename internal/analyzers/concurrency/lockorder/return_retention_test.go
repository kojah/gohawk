package lockorder

import (
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockReturnRetentionAndMerge(t *testing.T) {
	pkg := lockRetentionPackage(t)
	for _, name := range []string{"keep", "callback", "returnedOwner"} {
		fn := pkg.Func(name)
		returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
		value := ssa.Value(fn.Params[0])
		if name == "returnedOwner" {
			value = ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
		}
		identity := lockIdentityOf(value)
		values := map[string][]ssa.Value{identity: {value}}
		heldAt := map[*ssa.Return]lockReturnState{}
		unreleased := map[string][]token.Pos{}
		query := lockReturnQueries{budget: ssaflow.NewSearchBudget(ssaflow.SummaryBudget)}
		incoming := []string{identity}
		query.recordUnreleasedLocks(returned, incoming, nil, values, unreleased, heldAt)
		if query.budget.Exhausted() || len(unreleased[identity]) != boolCount(name == "keep") {
			t.Fatalf("%s retained=%v exhausted=%v", name, unreleased, query.budget.Exhausted())
		}
		if name != "keep" {
			continue
		}
		incoming[0] = "different"
		if !slices.Equal(heldAt[returned].definite, []string{identity}) {
			t.Fatal("return mask aliases incoming state")
		}
		query.recordUnreleasedLocks(returned, nil, nil, values, unreleased, heldAt)
		if !slices.Equal(heldAt[returned].possible, []string{identity}) || len(heldAt[returned].definite) != 0 {
			t.Fatalf("merged possible/definite retention: %+v", heldAt[returned])
		}
		query.recordUnreleasedLocks(returned, []string{identity}, []string{identity}, values, unreleased, heldAt)
		if len(unreleased[identity]) != 1 {
			t.Fatal("deferred release added another uncovered return")
		}
	}
}

func TestLockReturnOwnerAndHandoffCutoff(t *testing.T) {
	pkg := lockRetentionPackage(t)
	for _, name := range []string{"padded", "handoff"} {
		fn := pkg.Func(name)
		pool := ssaflow.NewSearchBudget(lockStateWorkBudget)
		child := pool.Within(30)
		ask := func(budget *ssaflow.SearchBudget) bool {
			if name == "handoff" {
				call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
				return handedUnlockCallback(call, fn.Params[0], budget)
			}
			query := lockReturnQueries{budget: budget}
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			return query.returnedUnlockOwner(returned, []ssa.Value{fn.Params[0]})
		}
		if ask(child) || !child.Exhausted() || pool.Exhausted() {
			t.Fatalf("%s cutoff exhausted=%v/%v", name, child.Exhausted(), pool.Exhausted())
		}
		if !ask(pool.Within(ssaflow.SummaryBudget)) {
			t.Fatalf("%s fresh capability query fails", name)
		}
	}
}

func TestLockReturnMergeCutoffDiscardsMasks(t *testing.T) {
	for _, seen := range []bool{false, true} {
		pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
		child := pool.Within(1)
		query := lockReturnQueries{budget: child}
		previous := lockReturnState{possible: []string{"first"}, definite: []string{"first"}}
		merged := query.mergeReturnState(previous, []string{"first", "second"}, seen)
		if !child.Exhausted() || pool.Exhausted() || len(merged.possible) != 0 || len(merged.definite) != 0 {
			t.Fatalf("seen=%v partial merge: %+v exhausted=%v/%v", seen, merged, child.Exhausted(), pool.Exhausted())
		}
		fresh := lockReturnQueries{budget: pool.Within(ssaflow.SummaryBudget)}
		merged = fresh.mergeReturnState(previous, []string{"first", "second"}, seen)
		if !slices.Equal(merged.possible, []string{"first", "second"}) || len(merged.definite) != 2-boolCount(seen) {
			t.Fatalf("seen=%v fresh merge: %+v", seen, merged)
		}
	}
}

func TestLockRetentionCutoffDiscardsEarlierFindings(t *testing.T) {
	assertLockWalkCutoffs(t, newLockWalkFixture(lockRetentionPackage(t).Func("witness")), 1)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func lockRetentionPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockretention", `package lockretention
 import "sync"
 var total int
 var global, second sync.Mutex
 var shared struct {mu sync.RWMutex; value int}
 type owner struct {mu sync.Mutex}
 func keep(mu *sync.Mutex)int{return 1}
 func callback(mu *sync.Mutex)func(){return mu.Unlock}
 func returnedOwner(o *owner)*owner{_=&o.mu;return o}
 func padded(mu *sync.Mutex,n int)func(){return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()}}
 func accept(cb func())
 func handoff(mu *sync.Mutex,n int){accept(func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()})}
 func witness(n int)func(){
 shared.mu.RLock();shared.value++;shared.mu.RUnlock()
 global.Lock();second.Lock();second.Unlock();global.Unlock();global.Lock()
 return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`global.Unlock()}
 }
 `)
}
