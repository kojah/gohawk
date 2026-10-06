package heapmodel

import (
	"maps"
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
