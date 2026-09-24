package syncgraph

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// Query is a read-only snapshot of one complete graph variant. Its evidence
// concerns only that variant, not all schedules or all external participants.
// A zero query or an incomplete graph cannot establish an absence claim.
type Query struct {
	events       []SyncEvent
	locations    map[EventID]eventLocation
	sequences    [][]SyncEvent
	prefixes     []int
	observations map[concurrencyfacts.Reference][]EventID
	reason       string
	ready        bool
}

type eventLocation struct {
	sequence int
	index    int
}

// NewQuery snapshots the event sequences and launch positions in linear time.
// Consumer-added blocking dependencies are deliberately not execution order.
// Queries never mutate a graph or combine mutually exclusive select variants.
func NewQuery(graph SyncGraph) Query {
	query := Query{reason: graph.Reason}
	if !graph.Complete() || len(graph.Choices) != 0 {
		return query
	}
	query.locations = make(map[EventID]eventLocation)
	query.observations = make(map[concurrencyfacts.Reference][]EventID)
	query.sequences = append(query.sequences, slices.Clone(graph.Parent))
	for _, child := range graph.Children {
		if !child.LaunchKnown() || child.Prefix < 0 || child.Prefix > len(graph.Parent) {
			query.reason = "syncgraph-invalid-spawn-prefix"
			return query
		}
		query.sequences = append(query.sequences, slices.Clone(child.Events))
		query.prefixes = append(query.prefixes, child.Prefix)
	}
	for sequence, events := range query.sequences {
		for index, event := range events {
			if _, exists := query.locations[event.ID]; exists || event.ID < 0 || event.Goroutine != GoroutineID(sequence) {
				query.reason = "syncgraph-invalid-event"
				return query
			}
			query.locations[event.ID] = eventLocation{sequence: sequence, index: index}
			query.events = append(query.events, event)
			if exactReference(event.Resource) && event.Resource.Cancellation && event.Kind == concurrencyfacts.Receive {
				query.observations[event.Resource] = append(query.observations[event.Resource], event.ID)
			}
		}
	}
	query.ready = true
	return query
}

// Before proves strict program/spawn order, conditional on the later event
// being reached. No path means unknown, not concurrent or reversed. A blocking
// dependency or matching channel identity alone never establishes this order.
// With the current root/children model this query is constant-time.
func (query Query) Before(before, after EventID) ssaflow.Proof {
	if !query.ready {
		return query.unavailable()
	}
	left, leftOK := query.locations[before]
	right, rightOK := query.locations[after]
	if !leftOK || !rightOK {
		return queryProof(ssaflow.EvidenceUnknown, "syncgraph-event-unavailable")
	}
	ordered := left.sequence == right.sequence && left.index < right.index
	if left.sequence == 0 && right.sequence > 0 {
		ordered = left.index < query.prefixes[right.sequence-1]
	}
	if ordered {
		return queryProof(ssaflow.EvidenceProven, "syncgraph-program-order")
	}
	return queryProof(ssaflow.EvidenceUnknown, "syncgraph-order-unproven")
}

func (query Query) unavailable() ssaflow.Proof {
	reason := query.reason
	if reason == "" {
		reason = "syncgraph-query-unavailable"
	}
	return queryProof(ssaflow.EvidenceUnknown, reason)
}

func queryProof(state ssaflow.EvidenceState, reason string) ssaflow.Proof {
	// Sources may include imported effects. Do not label a graph query as
	// exclusively local SSA evidence; the witness retains its origin and site.
	return ssaflow.Proof{State: state, Reason: ssaflow.EvidenceReason(reason)}
}

func exactReference(reference concurrencyfacts.Reference) bool {
	return reference.Value != nil && !reference.Indirect
}
