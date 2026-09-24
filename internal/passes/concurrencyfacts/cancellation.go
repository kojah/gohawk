package concurrencyfacts

// Cancellation is a request on an exact context signal, not a channel close or
// a worker join. Symbolic calls carry binding requirements: Context and
// CancelFunc types alone do not rule out custom implementations or callbacks.

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

var (
	withCancel      = syntax.PackageFunction("context", "WithCancel")
	withCancelCause = syntax.PackageFunction("context", "WithCancelCause")
	background      = syntax.PackageFunction("context", "Background")
	todoContext     = syntax.PackageFunction("context", "TODO")
	contextDone     = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Done"})
)

func isCancelConstructor(common *ssa.CallCommon) bool {
	return ssaflow.CallMatchesAnySymbol(common, withCancel, withCancelCause)
}

func cancellationType(value types.Type) bool {
	return syntax.NamedType(value, "context", "Context") || cancelFunctionType(value)
}

func cancelFunctionType(value types.Type) bool {
	return syntax.NamedType(value, "context", "CancelFunc") || syntax.NamedType(value, "context", "CancelCauseFunc")
}

func (engine *Engine) cancellationCall(call ssa.CallInstruction) (Summary, bool) {
	common := call.Common()
	switch {
	case ssaflow.CallMatchesAnySymbol(common, background, todoContext):
		return Summary{}, true
	case isCancelConstructor(common):
		// Restrict fresh signals to non-canceling, known parents for now.
		// Timers, parent propagation, custom Context hooks, and AfterFunc
		// callbacks must not disappear into an effect-free constructor.
		parent := engine.storage.Resolve(common.Args[0])
		value, ok := parent.Value.(*ssa.Call)
		if !parent.Proven() || !ok || !ssaflow.CallMatchesAnySymbol(value.Common(), background, todoContext) {
			return Summary{Reason: ReasonContextParentUnknown}, true
		}
		return Summary{}, true
	case ssaflow.CallMatchesSymbol(common, contextDone):
		return engine.cancellationOperation(call, ssaflow.CallReceiver(common), false), true
	case !common.IsInvoke() && common.Value != nil && cancelFunctionType(common.Value.Type()):
		return engine.cancellationOperation(call, common.Value, true), true
	default:
		return Summary{}, false
	}
}

func (engine *Engine) cancellationOperation(call ssa.CallInstruction, value ssa.Value, cancel bool) Summary {
	resource, ok := engine.reference(value)
	if !ok || !resource.Cancellation {
		return Summary{Reason: ReasonContextIdentityUnknown}
	}
	result := Summary{CancellationInputs: []Reference{resource}}
	if cancel {
		result.Operations = []Operation{{Kind: Cancel, Resource: resource, Source: call.Pos(), Site: call.Pos()}}
	}
	return finishCancellation(result)
}

func (engine *Engine) cancellationReference(value ssa.Value) (Reference, bool) {
	if call, ok := value.(*ssa.Call); ok && ssaflow.CallMatchesSymbol(call.Common(), contextDone) {
		return engine.reference(ssaflow.CallReceiver(call.Common()))
	}
	extract, ok := value.(*ssa.Extract)
	if !ok || extract.Index > 1 {
		return Reference{}, false
	}
	call, ok := extract.Tuple.(*ssa.Call)
	if !ok || !isCancelConstructor(call.Common()) {
		return Reference{}, false
	}
	if origin, _ := engine.cancellationCall(call); !origin.Complete() {
		return Reference{}, false
	}
	return Reference{Value: call, Cancellation: true}, true
}

func cancellationBound(reference Reference) bool {
	call, ok := reference.Value.(*ssa.Call)
	return reference.Cancellation && !reference.Indirect && ok && isCancelConstructor(call.Common())
}

// Requirements stay attached to conditional select summaries too. They must
// be discharged before graph expansion, not just before linear consumption.
func finishCancellation(summary Summary) Summary {
	if summary.Reason != ReasonNone && summary.Reason != ReasonContextBindingRequired && summary.Reason != ReasonSelectAlternatives {
		return summary
	}
	// Channel-valued helper arguments may become Done projections only at
	// binding time. Retain that requirement even without a local Done call.
	for _, op := range summary.Operations {
		if op.Resource.Cancellation {
			requireCancellation(&summary, []Reference{op.Resource})
		}
	}
	if summary.operationCount() > maxOperations {
		return Summary{Reason: ReasonSummaryLimit}
	}
	for _, input := range summary.CancellationInputs {
		if !cancellationBound(input) {
			if summary.Reason != ReasonSelectAlternatives {
				summary.Reason = ReasonContextBindingRequired
			}
			return summary
		}
	}
	if summary.Reason == ReasonContextBindingRequired {
		summary.Reason = ReasonNone
	}
	return summary
}

// CancellationBound reports whether every conditional cancellation contract
// has an exact standard-library origin. Expansion must check this separately
// because select summaries are intentionally not linearly complete.
func (summary Summary) CancellationBound() bool {
	for _, input := range summary.CancellationInputs {
		if !cancellationBound(input) {
			return false
		}
	}
	return true
}

func composableLinear(summary Summary) bool {
	return summary.Reason == ReasonNone || summary.Reason == ReasonContextBindingRequired
}

func requireCancellation(summary *Summary, inputs []Reference) {
	for _, input := range inputs {
		if !slices.Contains(summary.CancellationInputs, input) {
			summary.CancellationInputs = append(summary.CancellationInputs, input)
		}
	}
}

func (engine *Engine) bindCancellationInputs(
	inputs []Reference, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction,
) ([]Reference, Reason) {
	result := make([]Reference, 0, len(inputs))
	for _, input := range inputs {
		if !engine.budget.Spend() {
			return nil, ReasonBudgetExhausted
		}
		bound, ok := engine.bind(input, bindings, instruction)
		if !ok || !bound.Cancellation {
			return nil, ReasonContextBindingUnknown
		}
		result = append(result, bound)
	}
	return result, ReasonNone
}
