package heapmodel

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestOrderedSlotsPreservesPublicationOrder(t *testing.T) {
	first := &region{serial: 1}
	second := &region{serial: 2}
	want := []slot{{region: first, path: "field:10"}, {region: first, path: "field:2"}, {region: second}}
	entries := map[slot]bool{want[2]: true, want[1]: false, want[0]: true}
	before := maps.Clone(entries)
	if got := orderedSlots(entries); !slices.Equal(got, want) {
		t.Fatalf("ordered slots = %v, want %v", got, want)
	}
	if !maps.Equal(entries, before) {
		t.Fatal("sorting changed input evidence")
	}
	if got := orderedSlots(map[slot]bool(nil)); got == nil || len(got) != 0 {
		t.Fatal("empty ordering must return a nonnil empty slice")
	}
}

func TestBoundedSlotPreservesRenderedPaths(t *testing.T) {
	root := &region{kind: regionExternal}
	placeholder := &region{kind: regionPlaceholder, source: slot{region: root, path: "field:0/field:1"}}
	projection := &heapProjection{roots: map[*region]HeapRoot{root: {Kind: HeapParameter, Index: 2}}}
	for _, test := range []struct {
		name   string
		target slot
		path   string
		ok     bool
	}{
		{"root", slot{region: root}, "", true},
		{"limit", slot{region: root, path: "field:0/field:1/field:2"}, "field:0/field:1/field:2", true},
		{"beyond", slot{region: root, path: "field:0/field:1/field:2/field:3"}, "", false},
		{"placeholder limit", slot{region: placeholder, path: "field:2"}, "field:0/field:1/field:2", true},
		{"placeholder beyond", slot{region: placeholder, path: "field:2/field:3"}, "", false},
		{"empty steps at limit", slot{region: root, path: "//"}, "//", true},
		{"empty steps beyond", slot{region: root, path: "///"}, "", false},
		{"unnamed", slot{region: &region{kind: regionSite}}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := projection.boundedSlot(test.target)
			if ok != test.ok || got.Path != test.path {
				t.Fatalf("bounded slot = %v, %t; want path %q, %t", got, ok, test.path, test.ok)
			}
			if !ok && got != (HeapSlot{}) {
				t.Fatal("rejected slot published partial evidence")
			}
			if ok && got.Root != projection.roots[root] {
				t.Fatal("accepted slot changed its named root")
			}
		})
	}
}

func BenchmarkOrderedSlots(b *testing.B) {
	for _, size := range []int{0, 1, 8, 32} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			entries := make(map[slot]bool, size)
			for index := range size {
				entries[slot{region: &region{serial: size - index}}] = false
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if got := orderedSlots(entries); len(got) != size {
					b.Fatal("sorting lost a slot")
				}
			}
		})
	}
}

func BenchmarkBoundedSlot(b *testing.B) {
	for _, depth := range []int{0, SummaryPaths, SummaryPaths + 1} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			root := &region{kind: regionExternal}
			projection := &heapProjection{roots: map[*region]HeapRoot{root: {Kind: HeapParameter}}}
			path := strings.TrimSuffix(strings.Repeat("field:0/", depth), "/")
			target := slot{region: root, path: path}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, ok := projection.boundedSlot(target); ok != (depth <= SummaryPaths) {
					b.Fatal("projection changed its depth boundary")
				}
			}
		})
	}
}
