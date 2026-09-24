package syncmodel

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
)

const maxAlternativeGraphs = 8

// Expand materializes every proven worker-select outcome as its own linear
// graph. A caller must prove its property on every returned graph; one graph
// alone never establishes an unavoidable deadlock. Unknown parent choices,
// unproven arms, and excessive products yield no usable graphs.
func Expand(summary concurrencyfacts.Summary) ([]SyncGraph, Failure) {
	if len(summary.Paths) != 0 {
		var graphs []SyncGraph
		for _, path := range summary.Paths {
			if len(path.Paths) != 0 {
				return nil, graphFailure(ReasonNestedAlternatives)
			}
			variants, reason := Expand(path)
			if !reason.Empty() {
				return nil, reason
			}
			graphs = append(graphs, variants...)
			if len(graphs) > maxAlternativeGraphs {
				return nil, graphFailure(ReasonAlternativeLimit)
			}
		}
		return graphs, Failure{}
	}
	if !summary.CancellationBound() {
		return nil, summaryFailure(concurrencyfacts.ReasonContextBindingRequired)
	}
	if !summary.CallbacksBound() {
		return nil, summaryFailure(concurrencyfacts.ReasonCallbackBindingRequired)
	}
	if summary.Complete() {
		graph := FromSummary(summary)
		return []SyncGraph{graph}, graph.Failure
	}
	if summary.Reason != concurrencyfacts.ReasonSelectAlternatives || !summary.AlternativesComplete || !workerChoicesComplete(summary) {
		return nil, summaryFailure(summary.Reason)
	}
	variants, failure := workerVariants(summary)
	if !failure.Empty() {
		return nil, failure
	}
	graphs := make([]SyncGraph, 0, len(variants))
	for _, variant := range variants {
		graph := FromSummary(variant)
		if !graph.Complete() {
			return nil, graph.Failure
		}
		graphs = append(graphs, graph)
	}
	return graphs, Failure{}
}

func workerChoicesComplete(summary concurrencyfacts.Summary) bool {
	count := 0
	branches := 0
	for _, worker := range summary.Workers {
		if len(worker.Alternatives) == 0 {
			continue
		}
		if worker.Branches {
			branches++
			continue
		}
		count++
		matched := false
		for _, choice := range summary.Choices {
			if choice.Worker != worker.Spawn || len(choice.Arms) != len(worker.Alternatives) {
				continue
			}
			matched = true
			for index, arm := range choice.Arms {
				if !arm.Complete || !slices.EqualFunc(arm.Sequence, worker.Alternatives[index], identicalOperation) {
					return false
				}
			}
		}
		if !matched {
			return false
		}
	}
	return count+branches != 0 && count == len(summary.Choices)
}

// identicalOperation compares operations including their positions.
func identicalOperation(a, b concurrencyfacts.Operation) bool {
	return a.Kind == b.Kind && a.Resource == b.Resource && a.Source == b.Source && a.Site == b.Site &&
		slices.Equal(a.Alternates, b.Alternates)
}

// workerVariants expands every worker's alternatives into one summary per
// combination, joining each chosen branch alternative's conditions.
func workerVariants(summary concurrencyfacts.Summary) ([]concurrencyfacts.Summary, Failure) {
	variants := []concurrencyfacts.Summary{{
		Operations: summary.Operations, Workers: slices.Clone(summary.Workers), Conditions: summary.Conditions,
	}}
	found := false
	for index, worker := range summary.Workers {
		if len(worker.Alternatives) == 0 {
			continue
		}
		found = true
		if len(variants)*len(worker.Alternatives) > maxAlternativeGraphs {
			return nil, graphFailure(ReasonAlternativeLimit)
		}
		next := make([]concurrencyfacts.Summary, 0, len(variants)*len(worker.Alternatives))
		for _, variant := range variants {
			for alternative, operations := range worker.Alternatives {
				branch := variant
				branch.Workers = slices.Clone(variant.Workers)
				branch.Workers[index].Operations = operations
				branch.Workers[index].Alternatives = nil
				branch.Workers[index].AlternativeConditions = nil
				if alternative < len(worker.AlternativeConditions) {
					// The worker's own branch choice joins the variant's.
					branch.Conditions = append(slices.Clone(variant.Conditions), worker.AlternativeConditions[alternative]...)
				}
				next = append(next, branch)
			}
		}
		variants = next
	}
	if !found {
		return nil, summaryFailure(summary.Reason)
	}
	return variants, Failure{}
}
