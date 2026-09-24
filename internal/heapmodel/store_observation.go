package heapmodel

import "golang.org/x/tools/go/ssa"

// GraphEvidence is an observational snapshot, not proof of a relationship.
// An absent or in-progress cached graph is distinct from a completed graph
// with no losses. Call records describe the last transfer at each instruction.
type GraphEvidence struct {
	Cached, Building bool
	BuildReason      GraphBuildReason
	Calls            []CallApplication
	// Widenings counts distinct (slot, instruction) sites, not fixpoint visits.
	Widenings int
	// Escapes counts distinct (slot, escape kind) records, not leaked resources.
	Escapes int
}

// CachedGraphEvidence reads an existing graph without building, publishing, or
// refreshing one. Tracing must not change inference by warming the graph cache.
// Callers should skip this query entirely when observation is disabled.
func CachedGraphEvidence(function *ssa.Function) GraphEvidence {
	regionGraphs.Lock()
	element, found := regionGraphs.entries[function]
	if !found {
		regionGraphs.Unlock()
		return GraphEvidence{}
	}
	entry := element.Value.(*regionGraphEntry) //nolint:forcetypeassert // The list holds only graph entries.
	graph := entry.graph
	regionGraphs.Unlock()
	if graph == nil {
		return GraphEvidence{Cached: true, Building: true}
	}
	defer graph.lock()()
	type wideningSite struct {
		target slot
		at     ssa.Instruction
	}
	seen := make(map[wideningSite]struct{}, len(graph.widened))
	for _, widening := range graph.widened {
		seen[wideningSite{widening.target, widening.at}] = struct{}{}
	}
	return GraphEvidence{
		Cached: true, BuildReason: graph.buildReason, Calls: graph.callApplications(),
		Widenings: len(seen), Escapes: len(graph.escapeOrigins),
	}
}
