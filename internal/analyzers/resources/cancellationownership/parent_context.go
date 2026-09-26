package cancellationownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A fresh standard context child also follows its exact parent's cancellation.
// Reuse the existing classifier for that alternate owner, but only as unknown:
// an opaque handoff of the parent is not proof of when its child is canceled.
// Signal registrations still require their own stop function; cancellation of
// their parent does not unregister them. Wrapped and merged parents stay opaque.
// https://github.com/crazy-max/diun/blob/269cb27295944aeacfe549d24ab7ac483e600aa9/internal/notif/apprise/client.go#L91-L94
func parentCancellationClassifier(call *ssa.Call, observer ssaflow.Observer) *cancellationClassifier {
	contract, ok := cancellationContractFor(call.Common())
	if !ok || contract.packagePath != "context" || len(call.Common().Args) == 0 {
		return nil
	}
	// Contexts captured by workers are held in local cells. Resolve the load
	// where this child is created, not at the later cancellation or return: the
	// same source variable may subsequently hold the child instead of its parent.
	// https://github.com/werf/nelm/blob/6393382d695e65d8d8f744cf590337fe62a83eef/pkg/action/release_install.go#L179-L190
	parentValue := call.Common().Args[0]
	if resolved := heapmodel.NewStorage(ssaflow.NewSearchBudget(cancellationCompletionBudget).Observed(observer)).Resolve(parentValue); resolved.Proven() {
		parentValue = resolved.Value
	}
	parent, ok := parentValue.(*ssa.Extract)
	if !ok || parent.Index != 0 {
		return nil
	}
	constructor, ok := parent.Tuple.(*ssa.Call)
	if !ok || constructor.Parent() != call.Parent() {
		return nil
	}
	parentContract, ok := cancellationContractFor(constructor.Common())
	if !ok {
		return nil
	}
	cancel := ssaflow.CallResult(constructor, parentContract.result)
	if cancel == nil {
		return nil
	}
	return &cancellationClassifier{cancel: cancel, actions: make(map[ssa.Instruction]cancellationAction), observer: observer}
}
