package lockorder

import (
	"fmt"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A call made while a lock is held orders that lock before every lock the
// callee takes. Declaration classes remain the fallback, but exact embedded
// field paths retain their caller roots through helper bindings. A loaded
// pointer is one snapshot, not an alias for the mutable cell it came from.
//
// The search stops where the pass stops seeing code. A dynamic callee has no
// resolvable body, and a callee in another package is created without one,
// because go vet analyses one package per invocation. Both contribute nothing
// rather than an assumption; the missing orders are stable false negatives.
//
// Release is deliberately not modelled here. A callee that unlocks the
// caller's lock is already proven at the exact value by transferCalledUnlocks,
// which drops the lock from the held set before this search is consulted. A
// class-level release set would add nothing where that proof holds, and would
// wrongly suppress an order where the callee releases a different object of
// the same class.
type calleeLocks struct {
	acquires []lockAcquisition
}

// calleeLockSummaryBudget bounds instruction visits and witness expansion per
// root query. Recursive cuts cannot be memoized, so memoization alone does not
// bound the number of call paths a mutually recursive package can expose.
const calleeLockSummaryBudget = 250_000

// calleeLockSearch reuses completed per-function summaries. Recursive cuts
// contribute no new witness and are never retained; a shared work budget
// bounds the paths that must be searched again.
type calleeLockSearch struct {
	summaries *ssaflow.FunctionSummaries[calleeLocks]
}

func newCalleeLockSearch() *calleeLockSearch {
	search := &calleeLockSearch{}
	search.summaries = ssaflow.NewFunctionSummaries(search.searchLocks, func(ssaflow.SummaryUnavailable) calleeLocks {
		return calleeLocks{}
	})
	return search
}

// locks reports the lock classes function may acquire, following the static
// calls it makes.
func (search *calleeLockSearch) locks(function *ssa.Function) calleeLocks {
	return search.summaries.Function(function, ssaflow.NewSearchBudget(calleeLockSummaryBudget))
}

// locksAt reports the locks call's callee may take, given the constant Boolean
// arguments the call passes. A constant decides a branch that tests the
// parameter directly or negated, so an acquisition only on the arm the
// constant rules out is not taken by this call: libovsdb's monitor locks
// rpcMutex only when reconnecting is false, and connect passes true.
// Nested calls keep their ordinary summaries.
// https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L933-L941
func (search *calleeLockSearch) locksAt(call *ssa.Call) calleeLocks {
	return search.locksAtWithin(call, ssaflow.NewSearchBudget(calleeLockSummaryBudget))
}

func (search *calleeLockSearch) locksAtWithin(call *ssa.Call, budget *ssaflow.SearchBudget) calleeLocks {
	callee := call.Common().StaticCallee()
	fixed := ssaflow.ProveFixedArgumentsWithin(call.Common(), nil, callee, nil, budget)
	// A cutoff is not a complete empty binding set: unconstrained summaries
	// could revive acquisitions ruled out by arguments already encountered.
	// Losing ordering witnesses is safer than inventing caller feasibility.
	if !fixed.Proven() {
		return calleeLocks{}
	}
	if len(fixed.Values) == 0 {
		return search.locks(callee)
	}
	context := &constantContext{budget: budget, visiting: map[string]bool{}}
	result, complete := search.locksUnder(callee, fixed.Values, context)
	if !complete {
		if budget.Exhausted() || budget.PoolExhausted() {
			return calleeLocks{}
		}
		return search.locks(callee)
	}
	return result
}

// constantContext bounds a context-sensitive lock search. A call forwarding a
// constant parameter, as a recursive retry does, keeps its constant; a
// context already being searched adds nothing new to a may-acquire set.
type constantContext struct {
	budget   *ssaflow.SearchBudget
	visiting map[string]bool
	depth    int
}

func (search *calleeLockSearch) locksUnder(
	function *ssa.Function, constants ssaflow.FixedValues, context *constantContext,
) (calleeLocks, bool) {
	key := fmt.Sprintf("%p;%s", function, constants.Key(function))
	if context.visiting[key] {
		return calleeLocks{}, true
	}
	if context.depth >= maxOrderDepth {
		return calleeLocks{}, false
	}
	context.visiting[key] = true
	context.depth++
	defer func() { context.depth-- }()
	var result calleeLocks
	// Constants and the block census form one context. A truncated census
	// cannot establish an impossible acquisition arm or an empty may-acquire
	// set; abandon it so the root drops those acquisition witnesses.
	blocks := ssaflow.ReachableBlocksAssumingWithin(function, constants, context.budget)
	if context.budget.Exhausted() || context.budget.PoolExhausted() {
		return calleeLocks{}, false
	}
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			if !context.budget.Spend() {
				return calleeLocks{}, false
			}
			var nested ssaflow.FixedValues
			call, ok := instruction.(*ssa.Call)
			if ok {
				// Forwarded outcomes apply only to this call. Losing a binding
				// could revive an impossible acquisition arm, so discard the
				// context rather than treating a partial map as an ordinary call.
				fixed := ssaflow.ProveFixedArgumentsWithin(call.Common(), nil, call.Common().StaticCallee(), constants, context.budget)
				if !fixed.Proven() {
					return calleeLocks{}, false
				}
				nested = fixed.Values
			}
			if len(nested) == 0 {
				result.observe(search, instruction, context.budget)
				continue
			}
			locks, complete := search.locksUnder(call.Common().StaticCallee(), nested, context)
			if !complete {
				return calleeLocks{}, false
			}
			for _, acquired := range locks.acquires {
				result.add(acquired.through(call))
			}
		}
	}
	if context.budget.Exhausted() || context.budget.PoolExhausted() {
		return calleeLocks{}, false
	}
	return result, true
}

func (search *calleeLockSearch) searchLocks(function *ssa.Function, budget *ssaflow.SearchBudget) calleeLocks {
	var result calleeLocks
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				// Missing acquisition witnesses never establish absence. Dropping
				// a budget-shortened set can only lose an ordering diagnostic.
				return calleeLocks{}
			}
			result.observe(search, instruction, budget)
			if budget.Exhausted() || budget.PoolExhausted() {
				return calleeLocks{}
			}
		}
	}
	return result
}

func (locks *calleeLocks) observe(search *calleeLockSearch, instruction ssa.Instruction, budget *ssaflow.SearchBudget) {
	if operation, _, receiver, ok := mutexActionWithin(instruction, budget); ok {
		// A mutex selected by a map or slice index may be a different lock on
		// every iteration, which is why the acquisition walk declines it. The
		// same uncertainty applies when the acquisition is a callee's.
		if class := lockClassOf(receiver); operation == mutexAcquire && class != "" && !dynamicIndexedMutex(receiver) {
			locks.add(acquisitionAtWithin(instruction, class, budget))
		}
		return
	}
	// Only a synchronous call runs before the caller continues holding its
	// lock. A goroutine runs on its own stack, and a deferred call runs at a
	// return whose held set the walk decides separately.
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return
	}
	nested := search.summaries.Function(call.Common().StaticCallee(), budget)
	for _, acquired := range nested.acquires {
		if !budget.Spend() {
			return
		}
		locks.add(acquired.through(call))
	}
}

// Retain one finite witness per class, mode, and symbolic resource, not every
// route. Merging two formal owners before binding could discard a shared
// participant merely because another participant later binds to a fresh owner.
func (locks *calleeLocks) add(acquired lockAcquisition) {
	if acquired.class == "" || len(acquired.calls) > maxOrderDepth {
		return
	}
	for _, existing := range locks.acquires {
		if existing.class == acquired.class && existing.read == acquired.read && existing.resource == acquired.resource {
			return
		}
	}
	locks.acquires = append(locks.acquires, acquired)
}
