package heapmodel

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Heap call binding preserves caller values and the original instruction.
// Dispatch resolution establishes a target, not its effects. Captured cells
// must have exact reaching contents; deferred mutations remain unknown.
// Moby's queue waiter accounting captures its receiver without rebinding it:
// https://github.com/moby/moby/blob/3f673306102e01c16b5e0ab2343588bfab2fc4e7/daemon/logger/loggerutils/queue.go#L75
type heapCallBinding struct {
	callee         *ssa.Function
	args, captures []ssa.Value
}

func resolveHeapCall(common *ssa.CallCommon, instruction ssa.Instruction) (heapCallBinding, CallApplicationReason) {
	// Keep the concrete instantiation: heapSummaryOf owns origin fallback.
	// Resolving to an origin here can bypass its already-published summary.
	callee := common.StaticCallee()
	closure, _ := common.Value.(*ssa.MakeClosure)
	arguments := common.Args
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	if common.IsInvoke() {
		dispatch := ssaflow.ResolveInterfaceDispatch(common, instruction.Parent().Prog, budget)
		if !dispatch.Proven() {
			return heapCallBinding{}, CallInterface
		}
		callee = dispatch.Function
		arguments = append([]ssa.Value{dispatch.Receiver}, common.Args...)
	}
	if callee == nil {
		return heapCallBinding{}, CallDynamic
	}
	// Imported declarations can have a signature and published summary but
	// no SSA body or Params slice. Their signature still defines the binding.
	parameters := callee.Signature.Params().Len()
	if callee.Signature.Recv() != nil {
		parameters++
	}
	if len(arguments) != parameters {
		return heapCallBinding{callee: callee}, CallDynamic
	}
	captures, ok := bindHeapCaptures(common, callee, closure, instruction, budget)
	if !ok {
		return heapCallBinding{callee: callee}, CallClosure
	}
	return heapCallBinding{callee, arguments, captures}, CallSummaryApplied
}

func bindHeapCaptures(common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure,
	instruction ssa.Instruction, budget *ssaflow.SearchBudget,
) ([]ssa.Value, bool) {
	if len(callee.FreeVars) == 0 {
		return nil, true
	}
	if closure == nil || len(closure.Bindings) != len(callee.FreeVars) {
		return nil, false
	}
	var captures []ssa.Value
	storage := NewStorage(budget)
	for _, binding := range ssaflow.CallBindings(common, callee, closure) {
		if !binding.Captured {
			continue
		}
		value := binding.Supplied
		content := storage.ContentFromWrites(value, instruction)
		if !content.Proven() || !captureOnlyLoaded(binding.Local, budget) {
			return nil, false
		}
		// The current projection does not name successive dereferences.
		// Do not collapse a captured pointer-to-pointer onto its final object.
		if pointer, ok := content.Value.Type().Underlying().(*types.Pointer); ok {
			if _, nested := pointer.Elem().Underlying().(*types.Pointer); nested {
				return nil, false
			}
		}
		// Projection names the contents of a captured cell as F[n].
		// This substitution is valid only when the closure reads the cell,
		// never exposes its address or replaces its contents.
		value = content.Value
		captures = append(captures, value)
	}
	return captures, true
}

func captureOnlyLoaded(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	if value.Referrers() == nil {
		return false
	}
	for _, use := range *value.Referrers() {
		load, ok := use.(*ssa.UnOp)
		if !budget.Spend() || !ok || load.Op != token.MUL || load.X != value {
			return false
		}
	}
	return true
}
