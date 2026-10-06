package heapmodel

import (
	"go/token"
	"go/types"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// A call application says how the points-to graph treated one call: it
// substituted the callee's heap summary, or it could not and forgot what
// the call could reach. The record is evidence for traces and the dump;
// no proof reads it, so it cannot change a diagnostic. A graph built while
// a callee's summary was not yet available shows the call as unsummarized,
// which is how a trace exposes a summary that arrived too late.

// CallApplicationReason names how the graph treated a call.
type CallApplicationReason uint8

const (
	// CallApplicationUnknown is an unset classification, never an applied summary.
	CallApplicationUnknown CallApplicationReason = iota
	// CallSummaryApplied: the callee's summary was substituted.
	CallSummaryApplied
	// CallNoSummary: the callee has no summary the graph could find.
	CallNoSummary
	// CallClosure: the callee captures variables a summary cannot bind.
	CallClosure
	// CallInterface: an interface method call has no static callee.
	CallInterface
	// CallDynamic: a call through a function value has no static callee.
	CallDynamic
	// CallStarted: work handed to a goroutine is never substituted.
	CallStarted
	// CallRecursive: the callee can call back into the caller, so its
	// summary depends on the caller's own and is never applied.
	CallRecursive
	// CallDeferredUncertain: registration is conditional or can repeat.
	CallDeferredUncertain
	callApplicationReasonCount
)

// String converts the internal classification to its stable trace/dump code.
// Unknown and invalid values must never look like successful substitution.
func (reason CallApplicationReason) String() string {
	switch reason {
	case CallApplicationUnknown:
		return "unknown"
	case CallSummaryApplied:
		return "summary-applied"
	case CallNoSummary:
		return "no-summary"
	case CallClosure:
		return "closure-callee"
	case CallInterface:
		return "interface-call"
	case CallDynamic:
		return "dynamic-call"
	case CallStarted:
		return "started"
	case CallRecursive:
		return "call-cycle"
	case CallDeferredUncertain:
		return "defer-registration-uncertain"
	default:
		return "invalid-call-application-reason"
	}
}

// CallApplication is the graph's record of one call.
// RegisteredNow says whether the registry holds the callee's summary when
// the records are read: a no-summary call whose callee is registered now
// means the summary arrived after the graph was built.
type CallApplication struct {
	Instruction   ssa.Instruction
	Callee        *ssa.Function
	Reason        CallApplicationReason
	RegisteredNow bool
	// Effects and Truncated size the summary that was applied.
	Effects, Truncated int
}

func (graph *regionGraph) callApplications() []CallApplication {
	records := make([]CallApplication, 0, len(graph.applied))
	for _, entry := range graph.applied {
		_, registered := RegisteredHeapSummary(entry.callee)
		records = append(records, CallApplication{
			Instruction: entry.instruction, Callee: entry.callee, Reason: entry.reason, RegisteredNow: registered,
			Effects: entry.effects, Truncated: entry.truncated,
		})
	}
	return records
}

// recordCall keeps the latest record of how a call was treated.
func (graph *regionGraph) recordCall(entry appliedSummary) {
	if index, ok := graph.recorded[entry.instruction]; ok {
		graph.applied[index] = entry
		return
	}
	graph.recorded[entry.instruction] = len(graph.applied)
	graph.applied = append(graph.applied, entry)
}

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
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	if common.IsInvoke() {
		dispatch := ssacall.ResolveInterfaceDispatch(common, instruction.Parent().Prog, budget)
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
	instruction ssa.Instruction, budget *proofs.SearchBudget,
) ([]ssa.Value, bool) {
	if len(callee.FreeVars) == 0 {
		return nil, true
	}
	if closure == nil || len(closure.Bindings) != len(callee.FreeVars) {
		return nil, false
	}
	var captures []ssa.Value
	storage := NewStorage(budget)
	for _, binding := range ssacall.CallBindings(common, callee, closure) {
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

func captureOnlyLoaded(value ssa.Value, budget *proofs.SearchBudget) bool {
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
