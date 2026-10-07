package heapmodel

import (
	"fmt"
	"maps"
	"strconv"
	"testing"
)

func TestPointeesAddPreservesUnknownAndStale(t *testing.T) {
	first := slot{region: &region{kind: regionSite}}
	second := slot{region: &region{kind: regionSite}}
	unknown := slot{region: &region{kind: regionUnknown}}
	for _, test := range []struct {
		name    string
		initial pointees
		target  slot
		stale   bool
		want    pointees
	}{
		{"empty", pointees{}, first, false, pointees{first: false}},
		{"same known", pointees{first: false}, first, true, pointees{first: true}},
		{"retain stale", pointees{first: true}, first, false, pointees{first: true}},
		{"different known", pointees{first: false}, second, true, pointees{first: false, second: true}},
		{"unknown absorbs", pointees{unknown: true}, first, true, pointees{unknown: true}},
		{"mixed unknown absorbs duplicate", pointees{first: false, unknown: true}, first, true, pointees{first: false, unknown: true}},
		{"replace known with unknown", pointees{first: true, second: false}, unknown, true, pointees{unknown: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.initial.add(test.target, test.stale)
			if !maps.Equal(test.initial, test.want) {
				t.Fatalf("add changed evidence: got=%v want=%v", test.initial, test.want)
			}
		})
	}
}

func BenchmarkPointeesAdd(b *testing.B) {
	target := slot{region: &region{kind: regionSite}}
	other := slot{region: &region{kind: regionSite}}
	unknown := slot{region: &region{kind: regionUnknown}}
	for _, name := range []string{"empty", "duplicate", "different", "unknown", "mixed"} {
		b.Run(name, func(b *testing.B) {
			set := pointees{}
			switch name {
			case "duplicate":
				set[target] = false
			case "different":
				set[other] = false
			case "unknown":
				set[unknown] = true
			case "mixed":
				set[unknown], set[target] = true, false
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				switch name {
				case "empty":
					clear(set)
				case "different":
					delete(set, target)
				}
				set.add(target, true)
			}
		})
	}
}

func TestPointeesCloneIsIndependentAndWritable(t *testing.T) {
	first := slot{region: &region{kind: regionSite}}
	second := slot{region: &region{kind: regionSite}}
	original := pointees{first: true}
	copied := original.clone()
	copied[first] = false
	copied[second] = true
	if !original[first] || len(original) != 1 {
		t.Fatal("changing the copy changed the original pointee set")
	}
	var empty pointees
	copied = empty.clone()
	copied[first] = false
	if len(copied) != 1 {
		t.Fatal("cloning a nil set must return a writable set")
	}
}

func BenchmarkPointeesClone(b *testing.B) {
	for _, size := range []int{0, 1, 16, 256} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			set := pointees{}
			for range size {
				set[slot{region: &region{kind: regionSite}}] = false
			}
			b.ReportAllocs()
			for b.Loop() {
				copy := set.clone()
				if len(copy) != len(set) {
					b.Fatal("clone lost a pointee")
				}
			}
		})
	}
}

func BenchmarkRegionStateClone(b *testing.B) {
	for _, size := range []int{0, 1, 16, 256} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			state := newRegionState()
			for index := range size {
				owner := &region{kind: regionSite}
				target := slot{region: owner}
				state.backing[target] = owner
				state.clobbered[target] = index
				state.escaped[owner] = true
				state.escapes[target] = HeapEscape(1)
				state.stepEpochs[strconv.Itoa(index)] = index
			}
			b.ReportAllocs()
			for b.Loop() {
				copy := state.clone()
				if len(copy.backing) != size {
					b.Fatal("clone lost backing evidence")
				}
			}
		})
	}
}

func TestRegionStateScalarCloneIsIndependentAndWritable(t *testing.T) {
	owner := &region{kind: regionSite}
	target := slot{region: owner}
	state := newRegionState()
	state.backing[target] = owner
	state.stepEpochs["field"] = 1
	state.clobbered[target] = 2
	state.escaped[owner] = true
	state.escapes[target] = HeapEscape(1)
	copy := state.clone()
	delete(copy.backing, target)
	copy.stepEpochs["field"] = 3
	copy.clobbered[target] = 4
	copy.escaped[owner] = false
	copy.escapes[target] = 0
	if state.backing[target] != owner || state.stepEpochs["field"] != 1 || state.clobbered[target] != 2 ||
		!state.escaped[owner] || state.escapes[target] != HeapEscape(1) {
		t.Fatal("changing scalar snapshot evidence changed the original state")
	}
	copy = (&regionState{}).clone()
	copy.backing[target] = owner
	copy.stepEpochs["field"] = 1
	copy.clobbered[target] = 2
	copy.escaped[owner] = true
	copy.escapes[target] = HeapEscape(1)
}

func BenchmarkRegionStateContentsClone(b *testing.B) {
	for _, size := range []int{0, 1, 16, 32} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			state := newRegionState()
			for range 16 {
				set := pointees{}
				for range size {
					set[slot{region: &region{kind: regionSite}}] = false
				}
				state.contents[slot{region: &region{kind: regionSite}}] = set
			}
			b.ReportAllocs()
			for b.Loop() {
				copy := state.clone()
				if len(copy.contents) != 16 {
					b.Fatal("clone lost stored contents")
				}
			}
		})
	}
}

func TestRegionStateContentsCloneIsIndependentAndWritable(t *testing.T) {
	owner := slot{region: &region{kind: regionSite}}
	first := slot{region: &region{kind: regionSite}}
	second := slot{region: &region{kind: regionSite}}
	empty := slot{region: &region{kind: regionSite}}
	state := newRegionState()
	state.contents[owner] = pointees{first: true}
	state.contents[empty] = nil
	state.deferred = pointees{first: false}
	copy := state.clone()
	copy.contents[owner][first] = false
	copy.contents[owner][second] = true
	copy.contents[empty][second] = false
	copy.deferred[first] = true
	copy.deferred[second] = false
	if !state.contents[owner][first] || len(state.contents[owner]) != 1 || state.contents[empty] != nil ||
		state.deferred[first] || len(state.deferred) != 1 {
		t.Fatal("changing stored or deferred snapshot evidence changed the original state")
	}
	copy = newRegionState().clone()
	copy.deferred[first] = false
}

func TestPointeesUnion(t *testing.T) {
	first := slot{region: &region{kind: regionSite}}
	second := slot{region: &region{kind: regionSite}}
	unknown := slot{region: &region{kind: regionUnknown}}
	for _, test := range []struct {
		name        string
		destination pointees
		source      pointees
		want        pointees
	}{
		{"empty", nil, nil, nil},
		{"known stale overlap", pointees{first: false}, pointees{first: true, second: false}, pointees{first: true, second: false}},
		{"retain stale", pointees{first: true}, pointees{first: false}, pointees{first: true}},
		{"unknown destination", pointees{unknown: false}, pointees{first: true}, pointees{unknown: false}},
		{"unknown source", pointees{first: true}, pointees{second: false, unknown: true}, pointees{unknown: true}},
		{"retain unknown stale", pointees{unknown: true}, pointees{unknown: false, first: true}, pointees{unknown: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sourceBefore := test.source.clone()
			test.destination.union(test.source)
			if !maps.Equal(test.destination, test.want) {
				t.Fatalf("union = %v, want %v", test.destination, test.want)
			}
			if !maps.Equal(test.source, sourceBefore) {
				t.Fatal("union changed independent source evidence")
			}
		})
	}
}

func TestPointeesUnionWithSelf(t *testing.T) {
	for _, set := range []pointees{
		{{region: &region{kind: regionSite}}: true, {region: &region{kind: regionSite}}: false},
		{{region: &region{kind: regionUnknown}}: true},
	} {
		before := set.clone()
		set.union(set)
		if !maps.Equal(set, before) {
			t.Fatal("self union changed evidence")
		}
	}
}

func BenchmarkPointeesUnion(b *testing.B) {
	for _, size := range []int{0, 1, 8, 32} {
		b.Run(fmt.Sprintf("known/%d", size), func(b *testing.B) {
			source := make(pointees, size)
			for range size {
				source[slot{region: &region{kind: regionSite}}] = false
			}
			destination := source.clone()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				destination.union(source)
			}
		})
	}
	b.Run("unknown", func(b *testing.B) {
		destination := pointees{{region: &region{kind: regionUnknown}}: false}
		source := pointees{{region: &region{kind: regionSite}}: false}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			destination.union(source)
		}
	})
}

func TestMergeContentsReadsBeforeUpdates(t *testing.T) {
	for _, test := range []struct {
		name    string
		backing bool
	}{
		{"backing copy", true},
		{"wildcard element", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
			source := slot{region: &region{kind: regionSite}}
			copied := slot{region: &region{kind: regionSite}}
			old := slot{region: &region{kind: regionSite}}
			updated := slot{region: &region{kind: regionSite}}
			incoming := slot{region: &region{kind: regionSite}}
			state, other := newRegionState(), newRegionState()
			wanted := pointees{old: false, incoming: true}
			if test.backing {
				state.backing[copied] = source.region
			} else {
				source.path = "index:0"
				copied = slot{region: source.region, path: pathStar}
				wanted[slot{region: graph.nilR}] = false
			}
			state.contents[source] = pointees{old: false}
			other.contents[source] = pointees{updated: false}
			other.contents[copied] = pointees{incoming: true}
			graph.mergeContents(state, other, false, nil)
			if !maps.Equal(state.contents[copied], wanted) {
				t.Fatalf("merged contents = %v, want original read plus incoming evidence %v", state.contents[copied], wanted)
			}
			if !maps.Equal(other.contents[source], pointees{updated: false}) || !maps.Equal(other.contents[copied], pointees{incoming: true}) {
				t.Fatal("merge changed incoming evidence")
			}
		})
	}
}

func BenchmarkRegionMergeContents(b *testing.B) {
	for _, test := range []struct {
		name    string
		size    int
		missing int
	}{
		{"shared/16", 16, 0},
		{"shared/128", 128, 0},
		{"disjoint/16", 16, 16},
		{"disjoint/128", 128, 128},
		{"mixed/16", 16, 1},
		{"mixed/128", 128, 1},
	} {
		b.Run(test.name, func(b *testing.B) {
			size := test.size
			wanted := size + test.missing
			graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
			base, other := newRegionState(), newRegionState()
			for index := range size {
				owner := &region{kind: regionSite}
				target := slot{region: owner}
				base.contents[target] = pointees{{region: &region{kind: regionSite}}: false}
				base.backing[target] = owner
				base.escaped[owner] = true
				base.clobbered[target] = index
				incoming := target
				if index < test.missing {
					incoming = slot{region: &region{kind: regionSite}}
				}
				other.contents[incoming] = pointees{{region: &region{kind: regionSite}}: false}
			}
			b.ReportAllocs()
			for b.Loop() {
				state := base.clone()
				graph.mergeContents(state, other, false, nil)
				if len(state.contents) != wanted {
					b.Fatal("merge lost a slot")
				}
			}
		})
	}
}

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

func TestContentIndexQueriesReturnIndependentSets(t *testing.T) {
	graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
	owner := &region{kind: regionSite}
	value := slot{region: &region{kind: regionSite}}
	zero := slot{region: graph.nilR}
	unknown := slot{region: graph.unkR}
	for _, test := range []struct {
		name   string
		path   string
		stored map[slot]pointees
		want   pointees
	}{
		{"constant from wildcard", "index:0", map[slot]pointees{{region: owner, path: pathStar}: {value: true}}, pointees{value: true}},
		{"dynamic includes unwritten", pathStar, map[slot]pointees{{region: owner, path: "index:0"}: {value: false}}, pointees{value: false, zero: false}},
		{"dynamic absorbs unknown", pathStar, map[slot]pointees{{region: owner, path: "index:0"}: {unknown: true}}, pointees{unknown: true}},
		{"empty stored set", "index:0", map[slot]pointees{{region: owner, path: pathStar}: {}}, pointees{zero: false}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newRegionState()
			state.contents = test.stored
			got := graph.content(state, slot{region: owner, path: test.path})
			if !maps.Equal(got, test.want) {
				t.Fatalf("content = %v, want %v", got, test.want)
			}
			clear(got)
			got[value] = true
			if next := graph.content(state, slot{region: owner, path: test.path}); !maps.Equal(next, test.want) {
				t.Fatal("mutating query result changed later content")
			}
		})
	}
}

func BenchmarkRegionContent(b *testing.B) {
	for _, written := range []bool{false, true} {
		name := "unwritten"
		if written {
			name = "written"
		}
		b.Run(name, func(b *testing.B) {
			graph := &regionGraph{nilR: &region{kind: regionNil}, unkR: &region{kind: regionUnknown}}
			state := newRegionState()
			owner := slot{region: &region{kind: regionSite}}
			if written {
				state.contents[owner] = pointees{{region: &region{kind: regionSite}}: false}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if got := graph.content(state, owner); len(got) != 1 {
					b.Fatal("content query lost its single target")
				}
			}
		})
	}
}
