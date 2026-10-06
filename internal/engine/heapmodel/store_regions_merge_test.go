package heapmodel

import (
	"maps"
	"testing"
)

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
