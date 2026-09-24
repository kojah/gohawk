package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestQueryOrderDoesNotInventSynchronization(t *testing.T) {
	graph := FromSummary(concurrencyfacts.Summary{
		Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Receive}},
		Workers: []concurrencyfacts.WorkerSummary{
			{Site: 1, Prefix: 1, Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Close}}},
			{Site: 2, Prefix: 2, Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Send}}},
		},
	})
	// A consumer's prerequisite may even close a deadlock cycle. It must not
	// become evidence of strict execution order in the opposite direction.
	graph.AddDependency(3, 1)
	query := NewQuery(graph)
	for _, test := range []struct {
		name          string
		before, after EventID
		proven        bool
	}{
		{"parent order", 0, 1, true},
		{"worker order", 2, 3, true},
		{"spawn prefix", 0, 3, true},
		{"second spawn", 1, 4, true},
		{"later parent is not before earlier worker", 1, 2, false},
		{"workers are not serialized", 3, 4, false},
		{"dependency is not execution order", 3, 1, false},
		{"no self order", 0, 0, false},
		{"reverse order", 1, 0, false},
		{"invalid event", 99, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			proof := query.Before(test.before, test.after)
			if proof.Proven() != test.proven || !test.proven && proof.Known() {
				t.Fatalf("Before = %+v, want proven=%v and otherwise unknown", proof, test.proven)
			}
		})
	}
	graph.Children[0].Events[0].ID = 99
	graph.Children[0].Prefix = 0
	if !query.Before(0, 2).Proven() {
		t.Fatal("mutating the source graph changed a query snapshot")
	}
}

func TestQueriesRejectIncompleteAndMalformedGraphs(t *testing.T) {
	mutex := concurrencyfacts.Reference{Value: &ssa.Alloc{}}
	channel := concurrencyfacts.Reference{Value: &ssa.MakeChan{}}
	for name, query := range map[string]Query{
		"zero":       {},
		"incomplete": NewQuery(SyncGraph{Reason: "protocol-budget-exhausted"}),
		"choice":     NewQuery(SyncGraph{Choices: []SyncChoice{{}}}),
		"duplicate": NewQuery(SyncGraph{Parent: []SyncEvent{
			{ID: 0, Resource: channel}, {ID: 0, Resource: channel},
		}}),
		"wrong worker": NewQuery(SyncGraph{Parent: []SyncEvent{{ID: 0, Goroutine: 1}}}),
		"unknown spawn": NewQuery(SyncGraph{Children: []SyncChild{{
			Events: []SyncEvent{{ID: 0, Goroutine: 1}},
		}}}),
	} {
		t.Run(name, func(t *testing.T) {
			if query.Before(0, 1).Known() || query.FirstSignalAfterAcquire(0, mutex, channel, concurrencyfacts.Lock).Known() ||
				query.CancellationObservations(0).Known() {
				t.Fatal("unavailable query established evidence")
			}
		})
	}
}

func TestFirstSignalRequiresExactAcquisitionPrefix(t *testing.T) {
	mutex := concurrencyfacts.Reference{Value: &ssa.Alloc{}}
	other := concurrencyfacts.Reference{Value: &ssa.Alloc{}}
	channel := concurrencyfacts.Reference{Value: &ssa.MakeChan{}}
	indirect := concurrencyfacts.Reference{Value: mutex.Value, Indirect: true}
	op := func(kind concurrencyfacts.Kind, ref concurrencyfacts.Reference) concurrencyfacts.Operation {
		return concurrencyfacts.Operation{Kind: kind, Resource: ref}
	}
	lock, unlock := op(concurrencyfacts.Lock, mutex), op(concurrencyfacts.Unlock, mutex)
	closeChannel, send := op(concurrencyfacts.Close, channel), op(concurrencyfacts.Send, channel)
	for _, test := range []struct {
		name    string
		ops     []concurrencyfacts.Operation
		state   ssaflow.EvidenceState
		reason  string
		present bool
	}{
		{"no signal", []concurrencyfacts.Operation{lock}, ssaflow.EvidenceDisproven, "syncgraph-no-channel-signal", false},
		{"empty worker", nil, ssaflow.EvidenceDisproven, "syncgraph-no-channel-signal", false},
		{"signal before lock", []concurrencyfacts.Operation{send, lock, closeChannel}, ssaflow.EvidenceDisproven, "syncgraph-signal-before-acquire", true},
		{
			"other lock",
			[]concurrencyfacts.Operation{op(concurrencyfacts.Lock, other), closeChannel},
			ssaflow.EvidenceDisproven, "syncgraph-signal-before-acquire", true,
		},
		{"unlock before lock", []concurrencyfacts.Operation{unlock, lock, closeChannel}, ssaflow.EvidenceUnknown, "syncgraph-alternate-unlock", false},
		{
			"condition releases locker",
			[]concurrencyfacts.Operation{op(concurrencyfacts.CondWait, other), lock, closeChannel},
			ssaflow.EvidenceUnknown, "syncgraph-alternate-unlock", false,
		},
		{
			"indirect identity",
			[]concurrencyfacts.Operation{op(concurrencyfacts.Lock, indirect), closeChannel},
			ssaflow.EvidenceUnknown, "syncgraph-identity-unknown", false,
		},
		{"acquisition before close", []concurrencyfacts.Operation{lock, closeChannel}, ssaflow.EvidenceProven, "syncgraph-program-order", true},
		{"acquisition before send", []concurrencyfacts.Operation{lock, send}, ssaflow.EvidenceProven, "syncgraph-program-order", true},
		{"not a held-at-signal claim", []concurrencyfacts.Operation{lock, unlock, closeChannel}, ssaflow.EvidenceProven, "syncgraph-program-order", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := FromSummary(concurrencyfacts.Summary{Workers: []concurrencyfacts.WorkerSummary{{Site: 1, Operations: test.ops}}})
			answer := NewQuery(graph).FirstSignalAfterAcquire(1, mutex, channel, concurrencyfacts.Lock)
			if answer.State != test.state || string(answer.Reason) != test.reason || answer.Present != test.present {
				t.Fatalf("FirstSignalAfterAcquire = %+v", answer)
			}
			if answer.Present && answer.Signal.ID != EventID(firstSignalIndex(test.ops, channel)) {
				t.Fatal("query skipped the first signal")
			}
		})
	}
}

func firstSignalIndex(ops []concurrencyfacts.Operation, ref concurrencyfacts.Reference) int {
	for index, op := range ops {
		if op.Resource == ref && (op.Kind == concurrencyfacts.Send || op.Kind == concurrencyfacts.Close) {
			return index
		}
	}
	return -1
}

func TestCancellationQueryIsIdentityNotJoinOrOrder(t *testing.T) {
	first := concurrencyfacts.Reference{Value: &ssa.Call{}, Cancellation: true}
	second := concurrencyfacts.Reference{Value: &ssa.Call{}, Cancellation: true}
	graph := FromSummary(concurrencyfacts.Summary{
		Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Cancel, Resource: first}},
		Workers: []concurrencyfacts.WorkerSummary{{Site: 1, Operations: []concurrencyfacts.Operation{
			{Kind: concurrencyfacts.Receive, Resource: first}, {Kind: concurrencyfacts.Receive, Resource: second},
		}}},
	})
	query := NewQuery(graph)
	answer := query.CancellationObservations(0)
	if !answer.Proven() || len(answer.Events) != 1 || answer.Events[0] != 1 {
		t.Fatalf("observations = %+v", answer)
	}
	if query.Before(0, 1).Known() || graph.HasCycle() || query.CancellationObservations(1).Known() {
		t.Fatal("cancellation identity became order, a cycle, or a cancellation request")
	}
	// Another variant may have no observation at all. A receive in the first
	// variant cannot supply a dependency or completion witness for this one.
	graph.Children[0].Events = nil
	other := NewQuery(graph).CancellationObservations(0)
	if !other.Proven() || len(other.Events) != 0 || len(query.CancellationObservations(0).Events) != 1 {
		t.Fatalf("variant observations leaked: %+v", other)
	}
}
