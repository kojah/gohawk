package ssaflow

import "testing"

func TestRegionHistoryWidensConservatively(t *testing.T) {
	owner := &region{kind: regionSite}
	unknown := &region{kind: regionUnknown}
	held := slot{region: owner, path: "field:0"}
	graph := &regionGraph{history: map[slot]pointees{}, unkR: unknown}
	objects := make([]slot, pointeeLimit+1)
	for index := range objects {
		objects[index] = slot{region: &region{kind: regionSite, serial: index}}
		graph.remember(held, pointees{objects[index]: false})
		if index == pointeeLimit-1 && len(graph.history[held]) != pointeeLimit {
			t.Fatalf("history below limit has %d objects, want %d", len(graph.history[held]), pointeeLimit)
		}
	}
	if got := graph.history[held]; len(got) != 1 || !got.unknown() {
		t.Fatalf("history above limit = %v, want unknown only", got)
	}
	graph.remember(held, pointees{objects[0]: false})
	if got := graph.history[held]; len(got) != 1 || !got.unknown() {
		t.Fatalf("widened history grew again: %v", got)
	}
	if !graph.everContainedUnlocked(slot{region: owner}, pointees{objects[0]: false}) {
		t.Fatal("unknown history must admit possible containment")
	}
}
