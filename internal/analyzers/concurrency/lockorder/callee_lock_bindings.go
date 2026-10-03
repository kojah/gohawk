package lockorder

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Call-site lock bindings preserve embedded field paths and caller snapshots
// while composing callee acquisitions. A constructor-backed observed slot can
// make declaration identity uncertain; it does not prove a private mutex or
// safe publication. Summary traversal and its branch contexts stay separate.

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
			// Instance resolution already rejects loop allocations. For this
			// exact fresh root it also supplies the local class; reuse the proof.
			acquired.class = acquired.instance
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
	unknown := freshMutexFieldProof{reason: lockReasonNoFreshBoundOwner}
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
		return freshMutexFieldProof{possible: true, reason: lockReasonFreshBoundOwnerIdentityUnknown}
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
			allocation, fresh := lifecycle.ReturnedResultWithin(result, 0, budget).(*ssa.Alloc)
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
