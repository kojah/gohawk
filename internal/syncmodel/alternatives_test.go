package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"golang.org/x/tools/go/ssa"
)

func TestExpandRequiresEveryAlternativeToBeComplete(t *testing.T) {
	spawn := &ssa.Go{}
	arms := [][]concurrencyfacts.Operation{
		{{Kind: concurrencyfacts.Receive}},
		{{Kind: concurrencyfacts.Send}, {Kind: concurrencyfacts.Receive}},
	}
	selection := concurrencyfacts.Summary{
		Reason: "protocol-select-alternatives", AlternativesComplete: true,
		Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Send}},
		Choices: []concurrencyfacts.SelectChoice{{Worker: spawn, Arms: []concurrencyfacts.SelectArm{
			{Sequence: arms[0], Complete: true}, {Sequence: arms[1], Complete: true},
		}}},
		Workers: []concurrencyfacts.WorkerSummary{{
			Spawn: spawn, Prefix: 0, Alternatives: arms,
		}},
	}
	graphs, reason := Expand(selection)
	if reason != "" || len(graphs) != 2 || !graphs[0].Complete() || !graphs[1].Complete() ||
		len(graphs[0].Children[0].Events) != 1 || len(graphs[1].Children[0].Events) != 2 {
		t.Fatalf("expanded select = %+v (%s)", graphs, reason)
	}
	selection.AlternativesComplete = false
	if graphs, reason := Expand(selection); len(graphs) != 0 || reason == "" {
		t.Errorf("partial continuation was expanded: %+v (%s)", graphs, reason)
	}
	selection.AlternativesComplete = true
	selection.Workers[0].Alternatives = make([][]concurrencyfacts.Operation, 9)
	selection.Choices[0].Arms = make([]concurrencyfacts.SelectArm, 9)
	for index := range selection.Choices[0].Arms {
		selection.Choices[0].Arms[index].Complete = true
	}
	if graphs, reason := Expand(selection); len(graphs) != 0 || reason != "syncgraph-alternative-limit" {
		t.Errorf("unbounded alternatives were expanded: %+v (%s)", graphs, reason)
	}
}
