package heapmodel

import "golang.org/x/tools/go/ssa"

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

// CallApplications lists how the function's points-to graph treated each
// call it reached, in the order the graph first reached them. A function
// whose graph is unavailable has no records.
func CallApplications(function *ssa.Function) []CallApplication {
	graph := regionsOfFunction(function)
	defer graph.lock()()
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
