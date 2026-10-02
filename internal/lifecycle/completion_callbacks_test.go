package lifecycle

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestExactCallbackAlternatives(t *testing.T) {
	callback := &ssa.MakeClosure{}
	shared := &ssa.ChangeType{X: callback}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{callback, cycle}
	for _, test := range []struct {
		name  string
		value ssa.Value
		count int
	}{
		{"literal", callback, 1},
		{"shared branches", &ssa.Phi{Edges: []ssa.Value{shared, shared}}, 2},
		{"nested branches", &ssa.Phi{Edges: []ssa.Value{shared, &ssa.Phi{Edges: []ssa.Value{callback, shared}}}}, 3},
		{"opaque alternative", &ssa.Phi{Edges: []ssa.Value{callback, &ssa.Parameter{}}}, 0},
		{"cyclic alternative", cycle, 0},
		{"empty merge", &ssa.Phi{}, 0},
		{"missing value", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			closures, ok := exactCallbacks(test.value, nil, false, nil)
			if ok != (test.count > 0) || len(closures) != test.count {
				t.Fatalf("exactCallbacks() returned %d callbacks, resolved=%t; want %d", len(closures), ok, test.count)
			}
			for _, closure := range closures {
				if closure != callback {
					t.Error("resolved a different closure")
				}
			}
		})
	}
}
