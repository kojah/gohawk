package lockorder

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
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
		}
	}
	return result
}

func (locks *calleeLocks) observe(search *calleeLockSearch, instruction ssa.Instruction, budget *ssaflow.SearchBudget) {
	if operation, _, receiver, ok := mutexAction(instruction); ok {
		// A mutex selected by a map or slice index may be a different lock on
		// every iteration, which is why the acquisition walk declines it. The
		// same uncertainty applies when the acquisition is a callee's.
		if class := lockClassOf(receiver); operation == mutexAcquire && class != "" && !dynamicIndexedMutex(receiver) {
			locks.add(acquisitionAt(instruction, class))
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

func lockResourcePath(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
	return ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, func(root ssa.Value) bool {
		switch root.(type) {
		case *ssa.Alloc, *ssa.Parameter, *ssa.FreeVar, *ssa.Global, *ssa.UnOp, *ssa.Call, *ssa.Extract:
			return true
		default:
			return false
		}
	})
}

func bindLockAcquisition(acquired lockAcquisition, call *ssa.Call) lockAcquisition {
	// Preserve the receiver while composing helper witnesses. Class-only
	// summaries cannot distinguish established from newly created participants.
	// Exact constructor-backed slots establish uncertainty, not private owners:
	// https://github.com/gopcua/opcua/blob/2b3714ccc8425190dbff8f7b2fa8c1bcb5152c2c/uasc/secure_channel.go#L625-L670
	path := acquired.resource
	if path.Root == nil {
		return acquired
	}
	callee, closure := ssaflow.DirectCallee(call.Common())
	for _, binding := range ssaflow.CallBindings(call.Common(), callee, closure) {
		if binding.Local != path.Root {
			continue
		}
		root, known := lockResourcePath(binding.Supplied)
		if known {
			root, known = root.Append(path.Fields[:path.Depth]...)
		}
		if !known {
			acquired.resource = ssaflow.EmbeddedFieldPath{}
			return acquired
		}
		acquired.resource = root
		acquired.instance = mutexPathInstanceIdentity(root)
		if _, fresh := root.Root.(*ssa.Alloc); fresh {
			acquired.class = localMutexPathIdentity(root)
			acquired.widened = false
		} else if possibleFreshBoundMutex(root).possible {
			acquired.class = ""
		}
		return acquired
	}
	return acquired
}

// A constructor stored into the exact observed slot can make a declaration
// class uncertain without establishing publication safety. The constructor may
// publish its result, and opaque code may replace the slot; neither is a claim
// that this mutex is private. Pointer-valued fields do not inherit their
// container's freshness: they may point at an established shared mutex.
func possibleFreshBoundMutex(path ssaflow.EmbeddedFieldPath) freshMutexFieldProof {
	unknown := freshMutexFieldProof{reason: "no-fresh-bound-owner"}
	load, ok := path.Root.(*ssa.UnOp)
	if !ok || load.Op != token.MUL || !embeddedValueFields(path) {
		return unknown
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return unknown
	}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	storage := heapmodel.NewStorage(budget)
	fresh := false
	for _, block := range load.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return unknown
			}
			if !ssaflow.InstructionMayFollow(instruction, load) {
				continue
			}
			// Only an initializer of this observed slot is positive evidence.
			// A whole-owner assignment or any visible shared value defeats it;
			// a branch-local constructor cannot cover an observation it does
			// not dominate. Storage compares receiver snapshots, not cells.
			if store, ok := instruction.(*ssa.Store); ok {
				if storage.Same(store.Addr, field.X).Proven() {
					return unknown
				}
				if sameBoundSlot(store.Addr, field, storage) {
					if !freshOwnerResult(store.Val, budget) {
						return unknown
					}
					fresh = fresh || ssaflow.InstructionDominates(store, load)
				}
			}
			if boundSlotMutation(instruction, field, storage, budget) {
				return unknown
			}
		}
	}
	if fresh && !budget.Exhausted() {
		return freshMutexFieldProof{possible: true, reason: "fresh-bound-owner-identity-unknown"}
	}
	return unknown
}

func embeddedValueFields(path ssaflow.EmbeddedFieldPath) bool {
	if path.Depth == 0 {
		return false
	}
	valueType := path.Root.Type()
	for _, index := range path.Fields[:path.Depth] {
		field := structField(valueType, index)
		if field == nil {
			return false
		}
		if _, pointer := field.Type().Underlying().(*types.Pointer); pointer {
			return false
		}
		valueType = types.NewPointer(field.Type())
	}
	return true
}

func freshOwnerResult(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	if _, fresh := value.(*ssa.Alloc); fresh {
		return true
	}
	call, ok := value.(*ssa.Call)
	if !ok {
		return false
	}
	callee, _ := ssaflow.DirectCallee(call.Common())
	if callee == nil || len(callee.Blocks) == 0 {
		return false
	}
	returned := false
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			result, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			allocation, fresh := ssainfer.ReturnedResult(result, 0).(*ssa.Alloc)
			if !fresh || allocation.Parent() != callee {
				return false
			}
			returned = true
		}
	}
	return returned
}

func boundSlotMutation(instruction ssa.Instruction, field *ssa.FieldAddr, storage *heapmodel.Storage, budget *ssaflow.SearchBudget) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	callee, closure := ssaflow.DirectCallee(call.Common())
	for _, binding := range ssaflow.CallBindings(call.Common(), callee, closure) {
		if storage.Same(binding.Supplied, field.X).Proven() &&
			visibleMutexSlotReplacement(ssaflow.NewReachingWalk(ssaflow.TransparentNone), binding.Local, field.Field, call, budget) {
			return true
		}
		if sameBoundSlot(binding.Supplied, field, storage) &&
			ssaflow.NewCallEffects(budget).Call(call, binding.Supplied).Effects&ssaflow.EffectMutate != 0 {
			return true
		}
	}
	return false
}

func sameBoundSlot(value ssa.Value, field *ssa.FieldAddr, storage *heapmodel.Storage) bool {
	target, ok := value.(*ssa.FieldAddr)
	return ok && target.Field == field.Field && storage.Same(target.X, field.X).Proven()
}

// A helper's embedded mutex can keep the identity of a fresh caller allocation
// without an invented FieldAddr instruction. Never carry that instruction's
// identity across loop allocations or treat a loaded owner as a fresh value.
func localMutexPathIdentity(path ssaflow.EmbeddedFieldPath) string {
	allocation, ok := path.Root.(*ssa.Alloc)
	if !ok || ssaflow.BlockInCycle(allocation.Block()) {
		return ""
	}
	return mutexPathInstanceIdentity(path)
}

func mutexPathInstanceIdentity(path ssaflow.EmbeddedFieldPath) string {
	root := heapmodel.NewStorage(nil).Resolve(path.Root)
	if !root.Proven() {
		return ""
	}
	if instruction, ok := root.Value.(ssa.Instruction); ok && ssaflow.BlockInCycle(instruction.Block()) {
		return ""
	}
	name := lockIdentityOf(root.Value)
	if name == "" {
		return ""
	}
	var identity strings.Builder
	identity.WriteString(name)
	valueType := root.Value.Type()
	for _, index := range path.Fields[:path.Depth] {
		field := structField(valueType, index)
		if field == nil {
			return ""
		}
		identity.WriteByte('.')
		identity.WriteString(field.Name())
		valueType = types.NewPointer(field.Type())
	}
	return identity.String()
}
