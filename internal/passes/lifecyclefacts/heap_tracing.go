package lifecyclefacts

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"golang.org/x/tools/go/ssa"
)

// Heap trace details describe already-computed evidence. They never warm the
// graph cache or turn an absent graph into evidence of an effect-free function.

// tracedCallLimit bounds the unsummarized calls one summary event lists.
const tracedCallLimit = 8

// heapTraceDetails describes the projection a summary rests on: how many
// effects and edges it has, which roots it cut, and which calls inside the
// function the graph could not substitute a summary at. Two runs that
// summarize one function differently differ here first.
func heapTraceDetails(function *ssa.Function, heap *heapmodel.HeapSummary) map[string]string {
	details := map[string]string{}
	if heap != nil {
		details["heap-edges"] = strconv.Itoa(len(heap.Edges))
		details["heap-effects"] = strconv.Itoa(len(heap.Effects))
		details["heap-truncated-count"] = strconv.Itoa(len(heap.Truncated))
		var cut []string
		for _, at := range heap.Truncated {
			cut = append(cut, at.String())
		}
		details["heap-truncated"] = strings.Join(cut, ",")
	}
	var calls, self []string
	applied := 0
	// A missing cache entry cannot establish absence of heap effects. Emit
	// availability alongside counts so eviction never masquerades as precision.
	evidence := heapmodel.CachedGraphEvidence(function)
	details["heap-cached"] = strconv.FormatBool(evidence.Cached)
	details["heap-building"] = strconv.FormatBool(evidence.Building)
	details["heap-build-reason"] = evidence.BuildReason.String()
	details["heap-widening-sites"] = strconv.Itoa(evidence.Widenings)
	details["heap-escape-origins"] = strconv.Itoa(evidence.Escapes)
	counts := make(map[heapmodel.CallApplicationReason]int)
	truncated := 0
	for _, record := range evidence.Calls {
		// Count every authoritative call record, even after the human-readable
		// sample fills. Sampling must not hide the scale of a missing contract.
		counts[record.Reason]++
		if record.Reason == heapmodel.CallSummaryApplied {
			applied++
			if record.Truncated > 0 {
				truncated++
			}
			if record.Callee == function && len(self) < tracedCallLimit {
				self = append(self, fmt.Sprintf("%d effects/%d truncated", record.Effects, record.Truncated))
			}
			continue
		}
		if len(calls) < tracedCallLimit {
			calls = append(calls, record.Instruction.String()+" ["+record.Reason.String()+"]")
		}
	}
	for reason, count := range counts {
		details["calls-"+reason.String()] = strconv.Itoa(count)
	}
	details["calls-applied-truncated"] = strconv.Itoa(truncated)
	details["calls-applied"] = strconv.Itoa(applied)
	details["calls-unsummarized"] = strings.Join(calls, "; ")
	details["calls-self-applied"] = strings.Join(self, "; ")
	return details
}
