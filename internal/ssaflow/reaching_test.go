package ssaflow

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

const reachingSource = `
package ssaflowtest

import "io"

type sink struct{ w io.Writer }

func (s *sink) Write(p []byte) (int, error) { return s.w.Write(p) }

func merge(flag bool, a, b io.Writer) io.Writer {
	var w io.Writer
	if flag {
		w = a
	} else {
		w = b
	}
	return w
}

func same(flag bool, a *sink) *sink {
	var s *sink
	if flag {
		s = a
	} else {
		s = a
	}
	return s
}

func wrapped(flag bool, a, b *sink) io.Writer {
	var w io.Writer
	if flag {
		w = a
	} else {
		w = b
	}
	return w
}

func deferring(w io.Writer) {
	defer func() { _ = w }()
	defer func() { _ = w }()
}
`

func returnedValue(t *testing.T, function *ssa.Function) ssa.Value {
	t.Helper()
	for _, block := range function.Blocks {
		if returned, ok := block.Instrs[len(block.Instrs)-1].(*ssa.Return); ok {
			return returned.Results[0]
		}
	}
	t.Fatalf("%s has no return", function.Name())
	return nil
}

func isParameter(_ ReachingWalk, value ssa.Value) bool {
	_, ok := value.(*ssa.Parameter)
	return ok
}

func TestReachingWalkFoldsPhiEdges(t *testing.T) {
	pkg := buildTestSSA(t, reachingSource)
	merged := pkg.Func("merge")
	returned := returnedValue(t, merged)
	second := merged.Params[2]
	isSecond := func(_ ReachingWalk, value ssa.Value) bool { return value == second }

	if !NewReachingWalk(0).Any(returned, isSecond) {
		t.Fatal("Any should find the second parameter through the phi")
	}
	if NewReachingWalk(0).Every(returned, isSecond) {
		t.Fatal("Every should fail when only one edge is the second parameter")
	}
	if !NewReachingWalk(0).Every(returned, isParameter) {
		t.Fatal("Every should hold when both edges are parameters")
	}
}

func TestReachingWalkRevisitContributesNothing(t *testing.T) {
	pkg := buildTestSSA(t, reachingSource)
	merged := pkg.Func("merge")
	returned := returnedValue(t, merged)
	walk := NewReachingWalk(0)
	if !walk.Mark(returned) {
		t.Fatal("first mark should report a first visit")
	}
	if walk.Mark(returned) {
		t.Fatal("second mark should report a revisit")
	}
	if walk.Any(returned, isParameter) || walk.Every(returned, isParameter) {
		t.Fatal("a visited value must contribute no evidence")
	}
}

func TestReachingWalkEveryOfNeedsValues(t *testing.T) {
	pkg := buildTestSSA(t, reachingSource)
	merged := pkg.Func("merge")
	always := func(ReachingWalk, ssa.Value) bool { return true }
	if NewReachingWalk(0).EveryOf(nil, always) {
		t.Fatal("no values must prove nothing")
	}
	if !NewReachingWalk(0).EveryOf([]ssa.Value{merged.Params[1], merged.Params[2]}, isParameter) {
		t.Fatal("every parameter satisfies the leaf")
	}
}

func TestResolveReachingValueRequiresAgreement(t *testing.T) {
	pkg := buildTestSSA(t, reachingSource)
	typeName := func(_ ReachingWalk, value ssa.Value) (string, bool) {
		if _, ok := value.(*ssa.Parameter); ok {
			return value.Type().String(), true
		}
		return "", false
	}
	identity := func(name string) string { return name }

	resolved, ok := ResolveReachingValue(NewReachingWalk(0), returnedValue(t, pkg.Func("same")), typeName, identity)
	if !ok || !strings.HasSuffix(resolved, "ssaflowtest.sink") {
		t.Fatalf("edges that agree should resolve, got %q %t", resolved, ok)
	}
	// Both edges of wrapped are MakeInterface conversions of parameters. They
	// resolve only when the walk peels that form, and then they agree on type.
	returned := returnedValue(t, pkg.Func("wrapped"))
	if _, ok := ResolveReachingValue(NewReachingWalk(0), returned, typeName, identity); ok {
		t.Fatal("an opaque MakeInterface must leave the value unresolved")
	}
	if _, ok := ResolveReachingValue(NewReachingWalk(TransparentMakeInterface), returned, typeName, identity); !ok {
		t.Fatal("a transparent MakeInterface should expose agreeing parameters")
	}
	disagreeing := func(_ ReachingWalk, value ssa.Value) (string, bool) { return value.Name(), true }
	if _, ok := ResolveReachingValue(NewReachingWalk(0), returnedValue(t, pkg.Func("merge")), disagreeing, identity); ok {
		t.Fatal("edges with different keys must not resolve")
	}
}

func TestDeferredInstructionsAndClosureBindingPairs(t *testing.T) {
	pkg := buildTestSSA(t, reachingSource)
	defers := InstructionsOf[*ssa.Defer](pkg.Func("deferring"))
	if len(defers) != 2 {
		t.Fatalf("expected two defers, got %d", len(defers))
	}
	closure, ok := defers[0].Common().Value.(*ssa.MakeClosure)
	if !ok {
		t.Fatal("deferred closure expected")
	}
	function := closure.Fn.(*ssa.Function)
	pairs := ClosureBindingPairs(function, closure)
	if len(pairs) != 1 || pairs[0].Free != function.FreeVars[0] || pairs[0].Binding != closure.Bindings[0] {
		t.Fatalf("unexpected pairs %v", pairs)
	}
	if ClosureBindingPairs(function, nil) != nil || ClosureBindingPairs(nil, closure) != nil {
		t.Fatal("missing function or closure yields no pairs")
	}
}

func TestWalkStatesExpandsEachKeyOnce(t *testing.T) {
	type state struct {
		node  int
		count int
	}
	var expanded []state
	WalkStates([]state{{node: 0}}, func(s state) int { return s.node }, func(s state) ([]state, bool) {
		expanded = append(expanded, s)
		if s.node == 3 {
			return nil, false
		}
		return []state{{node: s.node + 1, count: s.count + 1}, {node: s.node, count: s.count + 1}}, true
	})
	if len(expanded) != 4 {
		t.Fatalf("expected four expansions, got %v", expanded)
	}
	for index, s := range expanded {
		if s.node != index || s.count != index {
			t.Fatalf("expansion %d should be the first state reaching node %d, got %v", index, index, s)
		}
	}
}
