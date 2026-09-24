package syncmodel

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Scope projects a complete graph onto exact resources. It never repairs an
// incomplete summary. All children remain present, including event-free ones;
// uncertain aliasing and implicit condition-variable releases prevent slicing.
// The result preserves necessary order, not reachability or termination of
// omitted operations. Consumers must still establish each blocking obligation.
func (graph *SyncGraph) Scope(resources ...concurrencyfacts.Reference) SyncGraph {
	if graph == nil || !NewQuery(*graph).ready || len(resources) == 0 {
		return SyncGraph{Failure: graphFailure(ReasonScopeIncomplete)}
	}
	for _, edge := range graph.Edges {
		if edge.Kind == BlockingDependency {
			return SyncGraph{Failure: graphFailure(ReasonScopeDependencyPresent)}
		}
	}
	summary := concurrencyfacts.Summary{Conditions: graph.Conditions}
	prefixes := make([]int, len(graph.Parent)+1)
	for index, event := range graph.Parent {
		keep, known := scopedEvent(event, resources)
		if !known {
			return SyncGraph{Failure: graphFailure(ReasonScopeAliasUnknown)}
		}
		if keep {
			summary.Operations = append(summary.Operations, scopedOperation(event))
		}
		prefixes[index+1] = len(summary.Operations)
	}
	for _, child := range graph.Children {
		if child.Prefix < 0 || child.Prefix >= len(prefixes) {
			return SyncGraph{Failure: graphFailure(ReasonInvalidSpawnPrefix)}
		}
		worker := concurrencyfacts.WorkerSummary{Spawn: child.Spawn, Site: child.Site, Prefix: prefixes[child.Prefix]}
		for _, event := range child.Events {
			keep, known := scopedEvent(event, resources)
			if !known {
				return SyncGraph{Failure: graphFailure(ReasonScopeAliasUnknown)}
			}
			if keep {
				worker.Operations = append(worker.Operations, scopedOperation(event))
			}
		}
		summary.Workers = append(summary.Workers, worker)
	}
	return FromSummary(summary)
}

func scopedOperation(event SyncEvent) concurrencyfacts.Operation {
	return concurrencyfacts.Operation{Kind: event.Kind, Resource: event.Resource, Source: event.Source, Site: event.Site}
}

func scopedEvent(event SyncEvent, resources []concurrencyfacts.Reference) (keep, known bool) {
	if !exactReference(event.Resource) || event.Kind == concurrencyfacts.CondWait {
		return false, false
	}
	for _, resource := range resources {
		if !exactReference(resource) {
			return false, false
		}
		if event.Resource == resource {
			return true, true
		}
	}
	for _, resource := range resources {
		if !disjointResources(event.Resource, resource) {
			return false, false
		}
	}
	return false, true
}

func disjointResources(left, right concurrencyfacts.Reference) bool {
	if left.Cancellation || right.Cancellation {
		// Bound context signals are constructor identities, never ordinary
		// mutex/channel storage. Distinct context constructors stay distinct.
		return left.Cancellation != right.Cancellation || left.Value != right.Value
	}
	a, aKnown := resourcePath(left.Value)
	b, bKnown := resourcePath(right.Value)
	if aKnown && bKnown && a != b {
		return true
	}
	// Distinct symbolic parameter nodes do not prove distinct runtime
	// objects: two arguments may alias. Require a local allocation anchor.
	if !aKnown && !bKnown {
		return false
	}
	proof := heapmodel.ProveMayAlias(left.Value, right.Value)
	return !proof.Aliases && (proof.Reason == ssaflow.EvidenceDisjointObjects || proof.Reason == ssaflow.EvidenceDisjointPaths)
}

func resourcePath(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
	return ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, func(root ssa.Value) bool {
		switch root.(type) {
		case *ssa.Alloc, *ssa.MakeChan:
			return true
		default:
			return false
		}
	})
}

// FreshResource proves local allocation identity, including embedded fields.
// This is not an escape proof: callers need a complete participant model too.
func FreshResource(resource concurrencyfacts.Reference) Proof {
	if !exactReference(resource) || resource.Cancellation {
		return queryProof(ssaflow.EvidenceUnknown, ReasonFreshnessUnknown)
	}
	if _, known := resourcePath(resource.Value); !known {
		return queryProof(ssaflow.EvidenceUnknown, ReasonFreshnessUnknown)
	}
	return queryProof(ssaflow.EvidenceProven, ReasonFreshResource)
}

// HeldMutex proves that one goroutine acquires and later releases the same
// exact mutex, so the mutex stays held between the two events. Freshness is
// not required. Another participant can still release a mutex it did not
// lock, but doing so during this window would make the holder's own later
// release fatal on the path where it is reached. A mutex pointer loaded from
// storage, a projection, or a different release identity stays unknown. This
// says nothing about the resource being waited on, which consumers must still
// prove fresh or otherwise closed to outside participants.
func HeldMutex(acquire, release concurrencyfacts.Reference) Proof {
	if !exactReference(acquire) || acquire.Cancellation || acquire != release {
		return queryProof(ssaflow.EvidenceUnknown, ReasonHeldMutexUnknown)
	}
	if !concurrencyfacts.MutexPointer(acquire.Value.Type()) {
		return queryProof(ssaflow.EvidenceUnknown, ReasonHeldMutexUnknown)
	}
	return queryProof(ssaflow.EvidenceProven, ReasonHeldMutex)
}
