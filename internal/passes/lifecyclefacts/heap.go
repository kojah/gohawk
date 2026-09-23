package lifecyclefacts

import (
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// The heap projection is the one encoding of what a function does to the
// objects a caller can name. The graph supplies the edges, escapes, reads,
// and truncation; the every-return release proofs already computed for the
// discharge claims supply the release effects, because release is a
// coverage question the flow-sensitive graph does not answer on its own. A
// function whose graph is unavailable is projected as truncated at every
// parameter, so an importer applies it as an unresolved call rather than as
// a function with no effects.

// summarizeHeap projects the function's heap and adds its release effects.
func summarizeHeap(function *ssa.Function, fact *Fact) *ssaflow.HeapSummary {
	summary, ok := ssaflow.ProjectHeap(function)
	if !ok {
		for index := range function.Params {
			summary.Truncated = append(summary.Truncated, ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: index}})
		}
		for index := range function.Signature.Results().Len() {
			summary.Truncated = append(summary.Truncated, ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapResult, Index: index}})
		}
	}
	for _, discharge := range fact.Discharges {
		summary.Effects = append(summary.Effects, ssaflow.HeapEffect{
			Slot:    ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: discharge.Parameter}, Path: discharge.Path},
			Release: discharge.Method,
			Every:   true,
		})
	}
	return &summary
}
