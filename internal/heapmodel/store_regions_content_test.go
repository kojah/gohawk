package heapmodel

import "testing"

func TestContentBackingCyclesAreUnknown(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*regionState, *region)
	}{
		{
			name: "self backing",
			setup: func(state *regionState, first *region) {
				state.backing[slot{region: first}] = first
			},
		},
		{
			name: "two backing regions",
			setup: func(state *regionState, first *region) {
				second := &region{kind: regionSite}
				state.backing[slot{region: first}] = second
				state.backing[slot{region: second}] = first
			},
		},
		{
			name: "snapshot source",
			setup: func(_ *regionState, first *region) {
				first.kind = regionSnapshot
				first.source = slot{region: first}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
			state := newRegionState()
			first := &region{kind: regionSite}
			test.setup(state, first)
			got := graph.content(state, slot{region: first})
			if !got.unknown() {
				t.Errorf("cyclic content = %v, want unknown", got)
			}
		})
	}
}

func TestContentBackingKnownAndDeepPaths(t *testing.T) {
	graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
	state := newRegionState()
	first := &region{kind: regionSite}
	second := &region{kind: regionSite}
	state.backing[slot{region: first}] = second
	got := graph.content(state, slot{region: first})
	if _, ok := got[slot{region: graph.nilR}]; !ok || len(got) != 1 {
		t.Errorf("acyclic content = %v, want nil region", got)
	}

	known := &region{kind: regionExternal}
	state.contents[slot{region: first}] = pointees{{region: known}: false}
	state.backing[slot{region: first}] = first
	if got := graph.content(state, slot{region: first}); len(got) != 1 {
		t.Errorf("written content = %v, want exact content", got)
	} else if _, ok := got[slot{region: known}]; !ok {
		t.Errorf("written content = %v, want exact content", got)
	}

	deep := newRegionState()
	chain := make([]*region, maxContentHops+2)
	for index := range chain {
		chain[index] = &region{kind: regionSite}
		if index > 0 {
			deep.backing[slot{region: chain[index-1]}] = chain[index]
		}
	}
	if got := graph.content(deep, slot{region: chain[0]}); !got.unknown() {
		t.Errorf("deep content = %v, want unknown", got)
	}
}

func TestContentQueriesReturnIndependentWritableSets(t *testing.T) {
	graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
	state := newRegionState()
	owner := slot{region: &region{kind: regionSite}}
	first := slot{region: &region{kind: regionSite}}
	second := slot{region: &region{kind: regionSite}}
	state.contents[owner] = pointees{first: true}
	read := graph.content(state, owner)
	read[first] = false
	read[second] = false
	if len(state.contents[owner]) != 1 || !state.contents[owner][first] {
		t.Fatal("changing query results changed stored evidence")
	}
	another := graph.content(state, owner)
	if len(another) != 1 || !another[first] {
		t.Fatal("a later query reused the mutated result")
	}
	unwritten := slot{region: &region{kind: regionSite}}
	read = graph.content(state, unwritten)
	read[second] = true
	if next := graph.content(state, unwritten); len(next) != 1 || next[slot{region: graph.nilR}] {
		t.Fatal("a later implicit query reused a mutated result")
	}
}
