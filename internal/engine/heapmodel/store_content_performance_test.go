package heapmodel

import "testing"

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
