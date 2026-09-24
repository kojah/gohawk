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
func Expand(summary concurrencyfacts.Summary) ([]SyncGraph, string) {
	if len(summary.Paths) != 0 {
		var graphs []SyncGraph
		for _, path := range summary.Paths {
			if len(path.Paths) != 0 {
				return nil, "syncgraph-nested-alternatives"
			}
			variants, reason := Expand(path)
			if reason != "" {
				return nil, reason
			}
			graphs = append(graphs, variants...)
			if len(graphs) > maxAlternativeGraphs {
				return nil, "syncgraph-alternative-limit"
			}
		}
		return graphs, ""
	}
	if !summary.CancellationBound() {
		return nil, "protocol-context-binding-required"
	}
	if summary.Complete() {
		graph := FromSummary(summary)
		return []SyncGraph{graph}, graph.Reason
	}
	if summary.Reason != "protocol-select-alternatives" || !summary.AlternativesComplete || !workerChoicesComplete(summary) {
		return nil, summary.Reason
	}
	variants := []concurrencyfacts.Summary{{Operations: summary.Operations, Workers: slices.Clone(summary.Workers)}}
	found := false
	for index, worker := range summary.Workers {
		if len(worker.Alternatives) == 0 {
			continue
		}
		found = true
		if len(variants)*len(worker.Alternatives) > maxAlternativeGraphs {
			return nil, "syncgraph-alternative-limit"
		}
		next := make([]concurrencyfacts.Summary, 0, len(variants)*len(worker.Alternatives))
		for _, variant := range variants {
			for _, operations := range worker.Alternatives {
				branch := variant
				branch.Workers = slices.Clone(variant.Workers)
				branch.Workers[index].Operations = operations
				branch.Workers[index].Alternatives = nil
				next = append(next, branch)
			}
		}
		variants = next
	}
	if !found {
		return nil, summary.Reason
	}
	graphs := make([]SyncGraph, 0, len(variants))
	for _, variant := range variants {
		graph := FromSummary(variant)
		if !graph.Complete() {
			return nil, graph.Reason
		}
		graphs = append(graphs, graph)
	}
	return graphs, ""
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
				if !arm.Complete || !slices.Equal(arm.Sequence, worker.Alternatives[index]) {
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
