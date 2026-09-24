package ssaflow

import "golang.org/x/tools/go/ssa"

// CallbackCaptureReadOnly reports whether a closure only observes one captured cell.
func CallbackCaptureReadOnly(closure *ssa.MakeClosure, cell ssa.Value, budget *SearchBudget) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	query := NewCallEffects(budget)
	for _, pair := range ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return false
		}
		if pair.Binding == cell && !query.Value(pair.Free).PreservesStorage() {
			return false
		}
	}
	return true
}
