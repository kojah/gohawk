package lockorder

import (
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// A call made while a lock is held orders that lock before every lock the
// callee takes. Finding those locks needs no value mapping: a class names a
// declaration rather than an object, so the string a callee produces for its
// own mutex is the string the caller compares against. That is what keeps this
// search cheap enough to run at every call site, and it is why the analyzer
// gained classes before it gained this.
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
	acquires []string
}

// calleeLockSearch answers the question once per function rather than once per
// call path that reaches it. Mutually recursive helpers are common in the code
// this analyzer runs on, and the memo is what keeps their cost linear.
type calleeLockSearch struct {
	memo *ssaflow.CallGraphMemo[*ssa.Function, calleeLocks]
}

func newCalleeLockSearch() *calleeLockSearch {
	return &calleeLockSearch{memo: ssaflow.NewCallGraphMemo[*ssa.Function, calleeLocks]()}
}

// locks reports the lock classes function may acquire, following the static
// calls it makes.
func (search *calleeLockSearch) locks(function *ssa.Function) calleeLocks {
	if function == nil || len(function.Blocks) == 0 {
		return calleeLocks{}
	}
	return search.memo.Answer(function, func() calleeLocks {
		return search.searchLocks(function)
	})
}

func (search *calleeLockSearch) searchLocks(function *ssa.Function) calleeLocks {
	if !search.memo.Enter(function) {
		// A recursive call adds nothing the enclosing answer does not already
		// collect, and Enter has recorded that this answer must not be retained.
		return calleeLocks{}
	}
	defer search.memo.Leave(function)
	var result calleeLocks
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			result.observe(search, instruction)
		}
	}
	return result
}

func (locks *calleeLocks) observe(search *calleeLockSearch, instruction ssa.Instruction) {
	if operation, _, receiver, ok := mutexAction(instruction); ok {
		// A mutex selected by a map or slice index may be a different lock on
		// every iteration, which is why the acquisition walk declines it. The
		// same uncertainty applies when the acquisition is a callee's.
		if class := lockClassOf(receiver); operation == mutexAcquire && class != "" && !dynamicIndexedMutex(receiver) {
			locks.acquires = appendUniqueString(locks.acquires, class)
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
	nested := search.locks(call.Common().StaticCallee())
	for _, class := range nested.acquires {
		locks.acquires = appendUniqueString(locks.acquires, class)
	}
}
