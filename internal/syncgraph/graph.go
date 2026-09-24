// Package syncgraph turns complete compositional synchronization effects into
// bounded event-order fragments. Edges express order, not a deadlock verdict;
// consumers must prove every blocking dependency they add.
package syncgraph

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"golang.org/x/tools/go/ssa"
)

// EventID identifies one event within a graph, not across summary instances.
type EventID int

// GoroutineID distinguishes the root from its sole summarized worker.
type GoroutineID uint8

const (
	Root GoroutineID = iota
	Worker
)

// SyncEvent is one ordered effect after its symbolic resource has been bound
// to the caller. It is an operation milestone, not a modeled start/end pair:
// consumers must establish which milestone a blocking edge needs. Source is
// the operation's origin; Site is its call site.
type SyncEvent struct {
	ID        EventID
	Goroutine GoroutineID
	Kind      concurrencyfacts.Kind
	Resource  concurrencyfacts.Reference
	Source    token.Pos
	Site      token.Pos
}

// EdgeKind distinguishes established execution order from a dependency that
// a consumer has separately proved. A cycle alone is never a diagnostic.
type EdgeKind uint8

const (
	ProgramOrder EdgeKind = iota
	SpawnOrder
	BlockingDependency
)

// SyncEdge says Before is a prerequisite for completing After. A channel
// handshake can satisfy both operations together; this is not a general
// happens-before claim.
type SyncEdge struct {
	Before EventID
	After  EventID
	Kind   EdgeKind
}

// SyncGraph contains at most the events of one complete root/sole-worker
// summary. Parent and Child preserve their distinct ordered sequences; Prefix
// counts parent events before the launch. Reason is nonempty if the underlying
// summary was incomplete, in which case no event or edge is usable as proof.
type SyncGraph struct {
	Parent []SyncEvent
	Child  []SyncEvent
	Edges  []SyncEdge
	Spawn  *ssa.Go
	Prefix int
	Reason string
}

// FromSummary constructs an event fragment without interpreting missing
// effects. It takes linear time and space in the bounded summary size.
func FromSummary(summary concurrencyfacts.Summary) SyncGraph {
	if !summary.Complete() {
		return SyncGraph{Reason: summary.Reason}
	}
	if summary.Prefix < 0 || summary.Prefix > len(summary.Operations) {
		return SyncGraph{Reason: "syncgraph-invalid-spawn-prefix"}
	}
	graph := SyncGraph{Spawn: summary.Spawn, Prefix: summary.Prefix}
	for _, operation := range summary.Operations {
		graph.Parent = append(graph.Parent, graph.event(Root, operation))
	}
	for _, operation := range summary.Worker {
		graph.Child = append(graph.Child, graph.event(Worker, operation))
	}
	graph.addOrder(graph.Parent)
	graph.addOrder(graph.Child)
	if graph.Spawn != nil && graph.Prefix > 0 && len(graph.Child) > 0 {
		graph.Edges = append(graph.Edges, SyncEdge{
			Before: graph.Parent[graph.Prefix-1].ID, After: graph.Child[0].ID, Kind: SpawnOrder,
		})
	}
	// Separate proof consumers may add different dependency edges. Clipping
	// prevents an append on one graph value from mutating another's backing
	// array even when both start from the same event fragment.
	graph.Edges = slices.Clip(graph.Edges)
	return graph
}

func (graph *SyncGraph) event(goroutine GoroutineID, operation concurrencyfacts.Operation) SyncEvent {
	id := EventID(len(graph.Parent) + len(graph.Child))
	return SyncEvent{
		ID: id, Goroutine: goroutine, Kind: operation.Kind,
		Resource: operation.Resource, Source: operation.Source, Site: operation.Site,
	}
}

func (graph *SyncGraph) addOrder(events []SyncEvent) {
	for index := 1; index < len(events); index++ {
		graph.Edges = append(graph.Edges, SyncEdge{
			Before: events[index-1].ID, After: events[index].ID, Kind: ProgramOrder,
		})
	}
}

// Complete reports only whether the input event sequence was complete.
func (graph *SyncGraph) Complete() bool { return graph.Reason == "" }

// AddDependency records an independently established prerequisite between
// events. It refuses invalid IDs and incomplete graphs; it does not infer
// locking or channel semantics from matching names or resource types.
func (graph *SyncGraph) AddDependency(before, after EventID) bool {
	count := EventID(len(graph.Parent) + len(graph.Child))
	if !graph.Complete() || before < 0 || after < 0 || before >= count || after >= count {
		return false
	}
	graph.Edges = append(graph.Edges, SyncEdge{Before: before, After: after, Kind: BlockingDependency})
	return true
}

// HasCycle finds an ordering contradiction in linear time in the graph size.
// A consumer must still establish that its dependency edges are feasible and
// unavoidable before using this candidate to report a bug.
func (graph *SyncGraph) HasCycle() bool {
	count := len(graph.Parent) + len(graph.Child)
	if !graph.Complete() || count == 0 {
		return false
	}
	degree := make([]int, count)
	adjacent := make([][]EventID, count)
	for _, edge := range graph.Edges {
		if edge.Before < 0 || edge.After < 0 || int(edge.Before) >= count || int(edge.After) >= count {
			return false
		}
		degree[edge.After]++
		adjacent[edge.Before] = append(adjacent[edge.Before], edge.After)
	}
	queue := make([]EventID, 0, count)
	for id, pending := range degree {
		if pending == 0 {
			queue = append(queue, EventID(id))
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adjacent[id] {
			degree[next]--
			if degree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	return visited != count
}
