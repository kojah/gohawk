package ssaflow

import "golang.org/x/tools/go/ssa"

// A call application says how the points-to graph treated one call: it
// substituted the callee's heap summary, or it could not and forgot what
// the call could reach. The record is evidence for traces and the dump;
// no proof reads it, so it cannot change a diagnostic. A graph built while
// a callee's summary was not yet available shows the call as unsummarized,
// which is how a trace exposes a summary that arrived too late.

// CallApplicationReason names how the graph treated a call.
type CallApplicationReason string

const (
	// CallSummaryApplied: the callee's summary was substituted.
	CallSummaryApplied CallApplicationReason = "summary-applied"
	// CallNoSummary: the callee has no summary the graph could find.
	CallNoSummary CallApplicationReason = "no-summary"
	// CallClosure: the callee captures variables a summary cannot bind.
	CallClosure CallApplicationReason = "closure-callee"
	// CallInterface: an interface method call has no static callee.
	CallInterface CallApplicationReason = "interface-call"
	// CallDynamic: a call through a function value has no static callee.
	CallDynamic CallApplicationReason = "dynamic-call"
	// CallStarted: work handed to a goroutine is never substituted.
	CallStarted CallApplicationReason = "started"
)

// CallApplication is the graph's record of one call.
type CallApplication struct {
	Instruction ssa.Instruction
	Callee      *ssa.Function
	Reason      CallApplicationReason
}

// CallApplications lists how the function's points-to graph treated each
// call it reached, in the order the graph first reached them. A function
// whose graph is unavailable has no records.
func CallApplications(function *ssa.Function) []CallApplication {
	graph := regionsOfFunction(function)
	defer graph.lock()()
	records := make([]CallApplication, 0, len(graph.applied))
	for _, entry := range graph.applied {
		records = append(records, CallApplication{Instruction: entry.instruction, Callee: entry.callee, Reason: entry.reason})
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
