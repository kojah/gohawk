package syncgraph

import "github.com/kojah/gohawk/internal/passes/concurrencyfacts"

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

// Index observations once. Shared receive slices are immutable, keeping this
// relation linear in event count even with repeated requests and observers.
func (graph *SyncGraph) connectCancellation() {
	observations := make(map[concurrencyfacts.Reference][]EventID)
	var requests []SyncEvent
	collect := func(events []SyncEvent) {
		for _, event := range events {
			if !event.Resource.Cancellation || event.Resource.Indirect || event.Resource.Value == nil {
				continue
			}
			switch event.Kind {
			case concurrencyfacts.Cancel:
				requests = append(requests, event)
			case concurrencyfacts.Receive:
				observations[event.Resource] = append(observations[event.Resource], event.ID)
			default:
				// No other event observes a cancellation signal.
			}
		}
	}
	collect(graph.Parent)
	for _, child := range graph.Children {
		collect(child.Events)
	}
	for _, request := range requests {
		graph.Cancellations = append(graph.Cancellations, CancellationSignal{
			Request: request.ID, Receives: observations[request.Resource],
		})
	}
}
