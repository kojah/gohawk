package cancellationownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Local-use resolution distinguishes private observation from possible opaque
// consumption. Broad alias evidence only supplies uncertainty; helper bodies
// must resolve every use, and recursive or exhausted summaries remain opaque.

func instructionReferencesCancellation(instruction ssa.Instruction, cancel ssa.Value) bool {
	for _, operand := range instruction.Operands(nil) {
		if operand == nil || *operand == nil {
			continue
		}
		if *operand == cancel || heapmodel.MayAlias(*operand, cancel) || lifecycle.MayContainValue(*operand, cancel) ||
			addressStoresCancellation(*operand, cancel) {
			return true
		}
	}
	return false
}

// Ambiguous-use detection deliberately follows more aliases than exact
// release proofs. A broad match here can only suppress a diagnostic; it can
// never establish that cancellation was released or transferred.

// cancellationForms are the wrappers a cancel function keeps its identity
// through in local-use resolution and stored-address uncertainty.
const cancellationForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

func addressStoresCancellation(value, cancel ssa.Value) bool {
	return ssaflow.NewReachingWalk(cancellationForms).Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return addressStoresCancellationLeaf(walk, value, cancel)
	})
}

func addressStoresCancellationLeaf(walk ssaflow.ReachingWalk, value, cancel ssa.Value) bool {
	if loaded, ok := value.(*ssa.UnOp); ok && walk.Any(loaded.X, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return addressStoresCancellationLeaf(walk, value, cancel)
	}) {
		return true
	}
	if value.Referrers() == nil {
		return false
	}
	for _, reference := range *value.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == value && (store.Val == cancel || heapmodel.MayAlias(store.Val, cancel)) {
			return true
		}
	}
	return false
}

func deferredClosureUseIsLocallyResolved(instruction ssa.Instruction, cancel ssa.Value, observer proofs.Observer) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	function := common.StaticCallee()
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	found := false
	for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
		if !heapmodel.CapturedBindingMatches(captured.Binding, cancel) {
			continue
		}
		found = true
		if !newCancellationUse(observer).parameterResolved(function, captured.Free) {
			return false
		}
	}
	return found
}

func localCallOnlyObserves(instruction ssa.Instruction, cancel ssa.Value, observer proofs.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	callee := common.StaticCallee()
	// Proven read-only use is not cancellation. Unknown effects still go
	// through the cancellation-specific invocation policy below.
	if ssaflow.NewCallEffects(proofs.NewSearchBudget(cancellationCompletionBudget).Observed(observer)).Call(instruction, cancel).PreservesStorage() {
		return true
	}
	found := false
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		argument := binding.Supplied
		closureContainsCancel := false
		if _, ok := argument.(*ssa.MakeClosure); ok {
			closureContainsCancel = lifecycle.MayContainValue(argument, cancel)
		}
		if argument != cancel && !closureContainsCancel {
			continue
		}
		found = true
		if !newCancellationUse(observer).parameterResolved(callee, binding.Local) {
			return false
		}
	}
	return found
}

// cancellationUse answers whether every use of a cancel value inside a callee
// is resolved. The memo owns the cycle guard and the rule that an answer cut
// short by it is not retained.
type cancellationUse struct {
	memo   *ssaflow.CallGraphMemo[cancellationUseKey, bool]
	budget *proofs.SearchBudget
}

type cancellationUseKey struct {
	function  *ssa.Function
	parameter ssa.Value
}

func newCancellationUse(observer proofs.Observer) *cancellationUse {
	return &cancellationUse{
		memo:   ssaflow.NewCallGraphMemo[cancellationUseKey, bool](),
		budget: proofs.NewSearchBudget(cancellationCompletionBudget).Observed(observer),
	}
}

func (search *cancellationUse) parameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	key := cancellationUseKey{function: function, parameter: parameter}
	return search.memo.Summarize(key, function, search.budget, func() bool {
		return search.searchParameterResolved(function, parameter)
	}, func(ssaflow.SummaryUnavailable, bool) bool {
		// Unresolved use stays an opaque consumption at the classifier. A
		// shortened search must not prove that the helper only observes cancel.
		return false
	})
}

func (search *cancellationUse) searchParameterResolved(function *ssa.Function, parameter ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return false
			}
			if instructionReferencesCancellation(instruction, parameter) &&
				!search.instructionResolved(instruction, parameter) {
				return false
			}
		}
	}
	return true
}

func (search *cancellationUse) instructionResolved(instruction ssa.Instruction, parameter ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		if _, ok := instruction.(*ssa.DebugRef); ok || localStorageOnly(instruction) {
			return true
		}
		value, ok := instruction.(ssa.Value)
		return ok && exactLocalValueUse(value, parameter)
	}
	if exactLocalValueUse(common.Value, parameter) {
		return true
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 {
		return false
	}
	matched := false
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		if !search.budget.Spend() {
			return false
		}
		if !exactLocalValueUse(binding.Supplied, parameter) {
			continue
		}
		matched = true
		if !search.parameterResolved(callee, binding.Local) {
			return false
		}
	}
	return matched
}

// Local resolution admits wrappers and loads of one formal, but not a phi
// that may select another value. Keep that boundary while the fold owns cycles.
func exactLocalValueUse(value, parameter ssa.Value) bool {
	if value == parameter {
		return true
	}
	return ssaflow.NewReachingWalk(cancellationForms).OpaquePhis().Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return exactLocalValueUseLeaf(walk, value, parameter)
	})
}

func exactLocalValueUseLeaf(walk ssaflow.ReachingWalk, value, parameter ssa.Value) bool {
	if value == parameter {
		return true
	}
	loaded, ok := value.(*ssa.UnOp)
	return ok && loaded.Op == token.MUL && walk.Any(loaded.X, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		return exactLocalValueUseLeaf(walk, value, parameter)
	})
}

// localStorageOnly accepts a store into a local that no other code can read.
// A local captured by a closure is not private: a deferred guard such as
// `if cancelWorker != nil { cancelWorker() }` may release the stored cancel
// on every return, and this proof does not follow that closure, so the store
// is an ambiguous handoff rather than plain retention. Safebucket's worker
// lock loop uses exactly that shape:
// https://github.com/safebucket/safebucket/blob/f35560194cb6ea01a4607c2fe36ead2c7db51b9d/internal/core/bootstrap.go#L256-L297
func localStorageOnly(instruction ssa.Instruction) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	local, ok := store.Addr.(*ssa.Alloc)
	return ok && heapmodel.QueryEscape(local, heapmodel.EscapeFunction).Outcome == heapmodel.EscapeLocal && !capturedByClosure(local)
}

func capturedByClosure(local *ssa.Alloc) bool {
	if local.Referrers() == nil {
		return false
	}
	for _, reference := range *local.Referrers() {
		if _, ok := reference.(*ssa.MakeClosure); ok {
			return true
		}
	}
	return false
}
