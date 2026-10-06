package heapmodel

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

func TestSortedSlotsPreservesEveryIdentityField(t *testing.T) {
	set := map[HeapSlot]bool{}
	var want []HeapSlot
	for kind := HeapParameter; kind <= HeapFreeVar; kind++ {
		for index := range 3 {
			for _, pkg := range []string{"", "a", "b"} {
				for _, name := range []string{"", "A", "B"} {
					for _, path := range []string{"", "field:10", "field:2"} {
						at := HeapSlot{Root: HeapRoot{Kind: kind, Index: index, Package: pkg, Name: name}, Path: path}
						set[at] = index == 0
						want = append(want, at)
					}
				}
			}
		}
	}
	before := maps.Clone(set)
	if got := SortedSlots(set); !slices.Equal(got, want) {
		t.Fatal("summary ordering changed")
	}
	if !maps.Equal(set, before) {
		t.Fatal("sorting changed input evidence")
	}
	if got := SortedSlots(nil); !reflect.DeepEqual(got, []HeapSlot{}) {
		t.Fatal("empty summary ordering must remain a nonnil empty slice")
	}
}

func BenchmarkSortedSlots(b *testing.B) {
	for _, size := range []int{0, 1, 8, 32, 128} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			set := make(map[HeapSlot]bool, size)
			for index := range size {
				set[HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: size - index}, Path: "field:0"}] = false
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				SortedSlots(set)
			}
		})
	}
}
