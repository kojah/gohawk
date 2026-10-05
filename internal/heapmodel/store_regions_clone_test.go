package heapmodel

import (
	"strconv"
	"testing"
)

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
