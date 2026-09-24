package concurrencyfacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A closed interface receiver can reuse direct summaries without changing
// argument positions: invocation mode stores the receiver in Value, whereas
// direct mode uses Args[0]. Never guess a target from the method name, the set
// of implementations in a package, or one possible receiver at a phi.
func (engine *Engine) resolvedCommon(instruction ssa.CallInstruction) *ssa.CallCommon {
	common := instruction.Common()
	if !common.IsInvoke() {
		return common
	}
	dispatch := ssaflow.ResolveInterfaceDispatch(common, instruction.Parent().Prog, engine.budget)
	if !dispatch.Proven() {
		return common
	}
	bound := *common
	bound.Method, bound.Value = nil, dispatch.Function
	bound.Args = append([]ssa.Value{dispatch.Receiver}, common.Args...)
	return &bound
}
