package heapmodel

// GraphBuildReason classifies whether a points-to graph reached its fixpoint.
// The zero value is unavailable, not evidence of a complete empty graph.
type GraphBuildReason uint8

const (
	GraphBuildUnknown GraphBuildReason = iota
	GraphBuildComplete
	GraphBuildNoBody
	GraphBuildBudgetExhausted
	GraphBuildFixpointLimit
	graphBuildReasonCount
)

// String formats the stable reason code, not the optional explanatory detail.
func (reason GraphBuildReason) String() string {
	switch reason {
	case GraphBuildUnknown:
		return "graph-build-unknown"
	case GraphBuildComplete:
		return "graph-build-complete"
	case GraphBuildNoBody:
		return "graph-build-no-body"
	case GraphBuildBudgetExhausted:
		return "graph-build-budget-exhausted"
	case GraphBuildFixpointLimit:
		return "graph-build-fixpoint-limit"
	default:
		return "invalid-graph-build-reason"
	}
}

// Keep the human-readable graph dump stable without using prose as state.
func (graph *regionGraph) buildFailureText() string {
	switch graph.buildReason {
	case GraphBuildNoBody:
		return "no body"
	case GraphBuildBudgetExhausted:
		return "budget exhausted"
	case GraphBuildFixpointLimit:
		return "fixpoint did not settle " + graph.buildDetail
	default:
		return ""
	}
}
