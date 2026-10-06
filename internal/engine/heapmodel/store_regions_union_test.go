package heapmodel

import (
	"fmt"
	"maps"
	"testing"
)

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
