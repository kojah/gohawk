package lockorder

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// lockIdentityOf names the lock a receiver value denotes, or returns the
// empty string when its origin cannot be told apart from another lock's.
func lockIdentityOf(value ssa.Value) string {
	return lockIdentityWithin(value, nil)
}

// lockIdentityWithin shares reaching and observation-time storage with the
// enclosing request. Cutoff supplies no identity, never a fallback slot name.
func lockIdentityWithin(value ssa.Value, budget *proofs.SearchBudget) string {
	identity := lockIdentity(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget), value, budget)
	if budget.Exhausted() || budget.PoolExhausted() {
		return ""
	}
	return identity
}

func lockIdentity(walk ssaflow.ReachingWalk, value ssa.Value, budget *proofs.SearchBudget) string {
	if value == nil || !walk.Mark(value) {
		return ""
	}
	resolved := heapmodel.NewStorage(budget).Resolve(value)
	if budget.Exhausted() || budget.PoolExhausted() {
		return ""
	}
	if resolved.Proven() && resolved.Value != value {
		return lockIdentity(walk, resolved.Value, budget)
	}
	if source, ok := ssaflow.IdentitySource(value); ok {
		return lockIdentity(walk, source, budget)
	}
	return lockIdentityLeaf(walk, value, budget)
}

func lockIdentityLeaf(walk ssaflow.ReachingWalk, value ssa.Value, budget *proofs.SearchBudget) string {
	switch typed := value.(type) {
	case *ssa.Call:
		owner, field := mutexGetter(typed)
		if field == nil {
			// A call result is not a stable lock across executions. Without the
			// body, matching separate calls by name guesses at aliasing, while
			// keeping them separate invents held locks across loop iterations.
			return ""
		}
		if identity := lockIdentity(walk, owner, budget); identity != "" {
			return identity + "." + field.Name()
		}
		return ""
	case *ssa.Global:
		return typed.Name()
	case *ssa.FieldAddr, *ssa.Field:
		return projectedFieldLockIdentity(walk, typed, budget)
	case *ssa.IndexAddr:
		return indexedLockIdentity(walk, typed.X, typed.Index, budget)
	case *ssa.Index:
		return indexedLockIdentity(walk, typed.X, typed.Index, budget)
	case *ssa.Parameter:
		return typed.Parent().String() + "." + typed.Name()
	case *ssa.FreeVar:
		// Two captured owners of the same type are distinct locks inside the
		// closure, just as two parameters are; the closure's own name keeps the
		// identity from colliding with any other function's captures.
		// https://github.com/trickstercache/trickster/blob/7818ae3c39e725eb998f04fa31e5d315ede84b79/integration/alb_request_headers_test.go#L93-L99
		return typed.Parent().String() + ":free:" + typed.Name()
	case *ssa.Alloc:
		// SSA uses generic comments such as "complit" for distinct local
		// allocations of the same type. Include the stable SSA value name so two
		// local owners do not collapse into one apparent recursive lock.
		return typed.Parent().String() + ":local:" + typed.Comment + ":" + typed.Name()
	case *ssa.Const:
		if typed.Value != nil {
			return "constant:" + typed.Value.ExactString()
		}
	}
	if parent := value.Parent(); parent != nil && value.Name() != "" {
		// Dynamic values of the same type can still identify different lock
		// instances. Keep their SSA identities distinct instead of collapsing
		// them to the field type. Prometheus transfers state while holding locks
		// on two alertmanagerSet values of the same type:
		// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/notifier/manager.go#L165-L180
		return parent.String() + ":value:" + value.Name()
	}
	return ""
}

// mutexGetter recognizes only a pointer receiver returning the address of one
// direct field. Loads, branches, calls and side effects are intentionally opaque;
// in particular a pointer-valued field could change between calls. This is the
// body of Account.Mu, not a convention attached to its name:
// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/store.go#L553-L555
func mutexGetter(call *ssa.Call) (ssa.Value, *types.Var) { //nolint:ireturn // The owner is an SSA value.
	callee := call.Common().StaticCallee()
	if callee == nil || callee.Signature.Recv() == nil || len(callee.Params) != 1 ||
		len(call.Common().Args) != 1 || len(callee.Blocks) != 1 {
		return nil, nil
	}
	if _, ok := callee.Params[0].Type().Underlying().(*types.Pointer); !ok {
		return nil, nil
	}
	instructions := callee.Blocks[0].Instrs
	if len(instructions) != 2 {
		return nil, nil
	}
	field, ok := instructions[0].(*ssa.FieldAddr)
	if !ok || field.X != callee.Params[0] {
		return nil, nil
	}
	returned, ok := instructions[1].(*ssa.Return)
	if !ok || len(returned.Results) != 1 || returned.Results[0] != field {
		return nil, nil
	}
	return call.Common().Args[0], structField(field.X.Type(), field.Field)
}

func fieldLockIdentity(walk ssaflow.ReachingWalk, fieldAddress *ssa.FieldAddr, budget *proofs.SearchBudget) string {
	field := structField(fieldAddress.X.Type(), fieldAddress.Field)
	if field == nil {
		return ""
	}
	if owner := lockIdentity(walk, fieldAddress.X, budget); owner != "" {
		return owner + "." + field.Name()
	}
	// A declaration identifies a lock class, not an instance. When a call
	// returns an unknown owner, its field must remain unknown too: a pool can
	// return a different container on each loop iteration.
	// https://github.com/encodeous/nylon/blob/c4a96c804f7aa08512721dec7994907eab100bc8/polyamide/device/receive.go#L176-L180
	return ""
}

func projectedFieldLockIdentity(walk ssaflow.ReachingWalk, value ssa.Value, budget *proofs.SearchBudget) string {
	switch field := value.(type) {
	case *ssa.FieldAddr:
		return fieldLockIdentity(walk, field, budget)
	case *ssa.Field:
		return copiedFieldLockIdentity(walk, field, budget)
	}
	return ""
}

// A value receiver can be copied into a local SSA slot and reloaded for each
// field access. Only an exact aggregate read back to the immutable parameter
// or capture identifies those copies as the same field. Partial/conflicting
// writes leave the read unresolved, so they cannot equate different locks.
// https://github.com/tikv/client-go/blob/b9fc0b7719d3ea62bd9904cd31fbd27715ff08bc/txnkv/transaction/pessimistic.go#L489-L518
func copiedFieldLockIdentity(walk ssaflow.ReachingWalk, fieldValue *ssa.Field, budget *proofs.SearchBudget) string {
	field := structField(fieldValue.X.Type(), fieldValue.Field)
	if field == nil {
		return ""
	}
	source := heapmodel.NewStorage(budget).Resolve(fieldValue.X)
	if !source.Proven() {
		return ""
	}
	switch source.Value.(type) {
	case *ssa.Parameter, *ssa.FreeVar:
		if owner := lockIdentity(walk, source.Value, budget); owner != "" {
			return owner + "." + field.Name()
		}
	}
	return ""
}

// privateMutexOnly reports the deliberately narrow case where a local mutex
// allocation is used exclusively by direct synchronous mutex operations.
// No pointer, callback, or owner leaves these uses, so a held mutex cannot
// affect another caller after return. This says nothing about recursive
// acquisition before return, which is still checked.
// https://github.com/alajmo/sake/blob/86986df901293db0f7d1e548ef34c849bb1f709d/core/run/exec.go#L1070-L1084
func privateMutexOnly(value ssa.Value, budget *proofs.SearchBudget) bool {
	allocation, ok := value.(*ssa.Alloc)
	if !ok || allocation.Referrers() == nil {
		return false
	}
	for _, use := range *allocation.Referrers() {
		if !budget.Spend() {
			return false
		}
		if _, ok := use.(*ssa.DebugRef); ok {
			continue
		}
		if _, ok := use.(*ssa.Call); !ok {
			return false
		}
		_, _, receiver, ok := mutexActionWithin(use, budget)
		if !ok || receiver != allocation {
			return false
		}
	}
	return true
}

func indexedLockIdentity(walk ssaflow.ReachingWalk, ownerValue, indexValue ssa.Value, budget *proofs.SearchBudget) string {
	owner := lockIdentity(walk, ownerValue, budget)
	index := lockIdentity(walk, indexValue, budget)
	if owner == "" || index == "" {
		return ""
	}
	return owner + "[" + index + "]"
}

func structField(value types.Type, index int) *types.Var {
	if pointer, ok := value.Underlying().(*types.Pointer); ok {
		value = pointer.Elem()
	}
	structure, ok := value.Underlying().(*types.Struct)
	if !ok || index < 0 || index >= structure.NumFields() {
		return nil
	}
	return structure.Field(index)
}
