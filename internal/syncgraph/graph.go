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

// GoroutineID distinguishes the root from each summarized child.
type GoroutineID int

const Root GoroutineID = 0

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

// SyncChild preserves one child's ordered effects and launch point. Prefix
// counts parent events before the launch; different children never inherit
// program order merely because their launch sites are ordered.
type SyncChild struct {
	Events []SyncEvent
	Spawn  *ssa.Go
	Site   token.Pos
	Prefix int
}

// LaunchKnown reports whether this child was instantiated from an exact
// launch, either locally or from a parameter-relative imported fact.
func (child SyncChild) LaunchKnown() bool { return child.Spawn != nil || child.Site.IsValid() }

// SyncArm is a possible select communication, or a default path with no
// communication. Arms in one choice are mutually exclusive.
type SyncArm struct {
	Kind     concurrencyfacts.Kind
	Resource concurrencyfacts.Reference
	Source   token.Pos
	Default  bool
}

// SyncChoice is one unresolved select. The graph preserves its alternatives
// for evidence and tracing, but linear cycle proofs must decline the graph.
type SyncChoice struct {
	Arms   []SyncArm
	Prefix int
	Site   token.Pos
	Worker *ssa.Go
}

// SyncGraph contains the events of one complete, bounded root summary.
// Parent and each Child preserve distinct ordered sequences. Reason is
// nonempty if the underlying summary was incomplete or malformed, in which
// case no event or edge is usable as proof.
type SyncGraph struct {
	Parent   []SyncEvent
	Children []SyncChild
	Choices  []SyncChoice
	Edges    []SyncEdge
	Reason   string
}

// FromSummary constructs an event fragment without interpreting missing
// effects. It takes linear time and space in the bounded summary size.
func FromSummary(summary concurrencyfacts.Summary) SyncGraph {
	if !summary.Complete() {
		graph := SyncGraph{Reason: summary.Reason}
		if summary.Reason == "protocol-select-alternatives" {
			for _, choice := range summary.Choices {
				mapped := SyncChoice{Prefix: choice.Prefix, Site: choice.Site, Worker: choice.Worker}
				for _, arm := range choice.Arms {
					mapped.Arms = append(mapped.Arms, SyncArm{
						Kind: arm.Operation.Kind, Resource: arm.Operation.Resource,
						Source: arm.Operation.Source, Default: arm.Default,
					})
				}
				graph.Choices = append(graph.Choices, mapped)
			}
		}
		return graph
	}
	graph := SyncGraph{}
	nextID := EventID(0)
	for _, operation := range summary.Operations {
		graph.Parent = append(graph.Parent, event(nextID, Root, operation))
		nextID++
	}
	graph.addOrder(graph.Parent)
	for index, worker := range summary.Workers {
		if worker.Spawn == nil && !worker.Site.IsValid() || worker.Prefix < 0 || worker.Prefix > len(graph.Parent) {
			return SyncGraph{Reason: "syncgraph-invalid-spawn-prefix"}
		}
		child := SyncChild{Spawn: worker.Spawn, Site: worker.Site, Prefix: worker.Prefix}
		for _, operation := range worker.Operations {
			child.Events = append(child.Events, event(nextID, GoroutineID(index+1), operation))
			nextID++
		}
		graph.addOrder(child.Events)
		if child.Prefix > 0 && len(child.Events) > 0 {
			graph.Edges = append(graph.Edges, SyncEdge{
				Before: graph.Parent[child.Prefix-1].ID, After: child.Events[0].ID, Kind: SpawnOrder,
			})
		}
		graph.Children = append(graph.Children, child)
	}
	// Separate proof consumers may add different dependency edges. Clipping
	// prevents an append on one graph value from mutating another's backing
	// array even when both start from the same event fragment.
	graph.Edges = slices.Clip(graph.Edges)
	return graph
}

func event(id EventID, goroutine GoroutineID, operation concurrencyfacts.Operation) SyncEvent {
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
	count := EventID(graph.eventCount())
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
	count := graph.eventCount()
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

func (graph *SyncGraph) eventCount() int {
	count := len(graph.Parent)
	for _, child := range graph.Children {
		count += len(child.Events)
	}
	return count
}
