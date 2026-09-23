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

// projectHeap projects the function's heap, truncated at every parameter
// and result when the graph could not.
func projectHeap(function *ssa.Function) *ssaflow.HeapSummary {
	summary, ok := ssaflow.ProjectHeap(function)
	if !ok {
		for index := range function.Params {
			summary.Truncated = append(summary.Truncated, ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: index}})
		}
		for index := range function.Signature.Results().Len() {
			summary.Truncated = append(summary.Truncated, ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapResult, Index: index}})
		}
	}
	return &summary
}

// withReleases adds the release effects the discharge proofs established.
func withReleases(summary *ssaflow.HeapSummary, fact *Fact) *ssaflow.HeapSummary {
	for _, discharge := range fact.Discharges {
		summary.Effects = append(summary.Effects, ssaflow.HeapEffect{
			Slot:    ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: discharge.Parameter}, Path: discharge.Path},
			Release: discharge.Method,
			Every:   true,
		})
	}
	return summary
}

// receiverStores is the ReceiverStore claim as a query over the projection:
// on every normal return, some slot beneath the receiver's object holds the
// parameter's object and nothing else. A store of a wrapper around the
// parameter counts when the wrapper's slot holding it is in the projection,
// which a summarized wrapper constructor provides.
func receiverStores(summary *ssaflow.HeapSummary, index int) bool {
	parameter := ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: index}}
	for _, edge := range summary.Edges {
		if edge.Must && edge.From.Root.Kind == ssaflow.HeapParameter && edge.From.Root.Index == 0 && edge.From.Path != "" &&
			edge.To.Kind == ssaflow.HeapTargetSlot && edge.To.Slot == parameter {
			return true
		}
	}
	return false
}
