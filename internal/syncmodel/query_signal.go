package syncmodel

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
)

// SignalOrder records the first send or close on a channel in one goroutine,
// and an exact mutex acquisition that must precede it. Proven means the worker
// must pass that acquisition to reach the signal, not that it completes or
// holds the mutex at the signal. Absence is scoped to this complete variant.
type SignalOrder struct {
	ssaflow.Proof
	Acquire SyncEvent
	Signal  SyncEvent
	Present bool
}

// FirstSignalAfterAcquire queries the first channel signal, never a convenient
// later one. Before acquisition, an unlock or condition wait might release a
// lock held by another goroutine; such a prefix remains unknown. Callers still
// prove parent lock ownership, launch order, channel capacity, all participants,
// and every alternative before using this evidence in a deadlock proof.
func (query Query) FirstSignalAfterAcquire(
	worker GoroutineID, mutex, channel concurrencyfacts.Reference,
) SignalOrder {
	if !query.ready {
		return SignalOrder{Proof: query.unavailable()}
	}
	if worker < 0 || int(worker) >= len(query.sequences) || !exactReference(mutex) || !exactReference(channel) {
		return SignalOrder{Proof: queryProof(ssaflow.EvidenceUnknown, "syncgraph-identity-unknown")}
	}
	var acquire SyncEvent
	acquired := false
	for _, event := range query.sequences[worker] {
		if !exactReference(event.Resource) {
			return SignalOrder{Proof: queryProof(ssaflow.EvidenceUnknown, "syncgraph-identity-unknown")}
		}
		if !acquired && (event.Kind == concurrencyfacts.CondWait || event.Resource == mutex && event.Kind == concurrencyfacts.Unlock) {
			return SignalOrder{Proof: queryProof(ssaflow.EvidenceUnknown, "syncgraph-alternate-unlock")}
		}
		if !acquired && event.Resource == mutex && event.Kind == concurrencyfacts.Lock {
			acquire, acquired = event, true
		}
		if event.Resource != channel || event.Kind != concurrencyfacts.Send && event.Kind != concurrencyfacts.Close {
			continue
		}
		answer := SignalOrder{Acquire: acquire, Signal: event, Present: true}
		answer.Proof = queryProof(ssaflow.EvidenceDisproven, "syncgraph-signal-before-acquire")
		if acquired {
			answer.Proof = query.Before(acquire.ID, event.ID)
		}
		return answer
	}
	return SignalOrder{Proof: queryProof(ssaflow.EvidenceDisproven, "syncgraph-no-channel-signal")}
}
