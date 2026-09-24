package syncmodel

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// CancellationSignal connects a cancellation request to receives from its
// exact Done signal in this graph variant. Receives may execute before the
// request (for example due to other cancellation), and a select may choose
// another arm. This is neither a happens-before edge nor a completion proof.
// In particular, CancelFunc does not wait for work to stop:
// https://pkg.go.dev/context#CancelFunc
type CancellationSignal struct {
	Request  EventID
	Receives []EventID
}

// ObservationSet identifies all matching cancellation receives in this graph
// variant. Proven describes exhaustive enumeration of the modeled events only;
// it does not establish external participant completeness or worker completion.
// Events is an immutable borrowed slice owned by the query snapshot.
type ObservationSet struct {
	ssaflow.Proof
	Events []EventID
}

// CancellationObservations matches an exact Cancel event to receives from the
// same Done identity. The receives need not execute, nor be ordered after the
// request. CancelCause values do not affect readiness identity.
func (query Query) CancellationObservations(request EventID) ObservationSet {
	if !query.ready {
		return ObservationSet{Proof: query.unavailable()}
	}
	location, ok := query.locations[request]
	if !ok {
		return ObservationSet{Proof: queryProof(ssaflow.EvidenceUnknown, "syncgraph-event-unavailable")}
	}
	event := query.sequences[location.sequence][location.index]
	if event.Kind != concurrencyfacts.Cancel || !event.Resource.Cancellation || !exactReference(event.Resource) {
		return ObservationSet{Proof: queryProof(ssaflow.EvidenceUnknown, "syncgraph-cancel-identity-unknown")}
	}
	return ObservationSet{
		Proof: queryProof(ssaflow.EvidenceProven, "syncgraph-cancellation-observations"), Events: query.observations[event.Resource],
	}
}

// Index observations once. Shared receive slices are immutable, keeping this
// relation linear in event count even with repeated requests and observers.
func (graph *SyncGraph) connectCancellation() {
	query := NewQuery(*graph)
	for _, request := range query.events {
		if request.Kind != concurrencyfacts.Cancel {
			continue
		}
		observations := query.CancellationObservations(request.ID)
		if !observations.Proven() {
			continue
		}
		graph.Cancellations = append(graph.Cancellations, CancellationSignal{
			Request: request.ID, Receives: observations.Events,
		})
	}
}
