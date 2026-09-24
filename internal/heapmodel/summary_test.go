package heapmodel

import (
	"reflect"
	"testing"
)

func TestSortedSlots(t *testing.T) {
	parameter := HeapRoot{Kind: HeapParameter, Index: 0}
	result := HeapRoot{Kind: HeapResult, Index: 0}
	set := map[HeapSlot]bool{
		{Root: result}:                     true,
		{Root: parameter, Path: "field:2"}: true,
		{Root: parameter, Path: "field:1"}: true,
	}
	want := []HeapSlot{
		{Root: parameter, Path: "field:1"},
		{Root: parameter, Path: "field:2"},
		{Root: result},
	}
	if got := SortedSlots(set); !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedSlots() = %v, want %v", got, want)
	}
}

func TestSummaryString(t *testing.T) {
	parameter := HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 0}}
	summary := HeapSummary{
		Edges:    []HeapEdge{{From: parameter, To: HeapTarget{Kind: HeapTargetNil}, Must: true}},
		Requires: []HeapRequirement{{Slot: parameter, Kind: HeapRequiresNonNil}},
	}
	want := "edge P0 -> nil must\nrequires P0 non-nil every"
	if got := summary.String(); got != want {
		t.Fatalf("summary.String() = %q, want %q", got, want)
	}
}
