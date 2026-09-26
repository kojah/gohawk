package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const constantsSource = `package constants

func effect() {}

func direct(keep bool) {
	if keep {
		effect()
	}
}

func negated(keep bool) {
	if !keep {
		effect()
	}
}

func doubled(keep bool) {
	if !!keep {
		effect()
	}
}

func compared(keep bool) {
	if keep == true {
		effect()
	}
}

func captured(keep bool) {
	defer func() {
		if keep {
			effect()
		}
	}()
}

func rewritten(keep bool) {
	defer func() {
		if keep {
			effect()
		}
	}()
	keep = !keep
}

func call(keep bool) {
	direct(true)
	direct(keep)
}

type sink interface{ Put() }
type buffer struct{}

func (*buffer) Put() {}

func withoutSink(s sink) {
	if s == nil {
		effect()
	}
}

func untested(p *buffer) { p.Put() }

func nilCalls() {
	var typed *buffer
	withoutSink(nil)
	withoutSink(typed)
	untested(&buffer{})
}
`

// effectReached reports whether some block reachable under constants calls
// effect.
func effectReached(function *ssa.Function, constants ssaflow.FixedValues) bool {
	for _, block := range ssaflow.ReachableBlocksAssuming(function, constants) {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() != nil && call.Common().StaticCallee().Name() == "effect" {
				return true
			}
		}
	}
	return false
}

func TestFixedValuesDecideBranches(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constants", constantsSource)
	for _, test := range []struct {
		name    string
		value   bool
		reached bool
		decides bool
	}{
		{"direct", true, true, true},
		{"direct", false, false, true},
		{"negated", true, false, true},
		{"negated", false, true, true},
		{"doubled", false, false, true},
		// A comparison of the parameter is left to ordinary feasibility.
		{"compared", false, true, false},
	} {
		function := pkg.Func(test.name)
		outcome := ssaflow.OutcomeFalse
		if test.value {
			outcome = ssaflow.OutcomeTrue
		}
		constants := ssaflow.FixedValues{function.Params[0]: outcome}
		if got := effectReached(function, constants); got != test.reached {
			t.Errorf("%s(%t): effect reached = %t, want %t", test.name, test.value, got, test.reached)
		}
		_, decided := constants.DecidedSuccessor(function.Blocks[0])
		if decided != test.decides {
			t.Errorf("%s(%t): decided = %t, want %t", test.name, test.value, decided, test.decides)
		}
	}
}

func TestFixedArgumentsBindCallsAndCells(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constants", constantsSource)

	caller := pkg.Func("call")
	var calls []*ssa.Call
	for _, block := range caller.Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				calls = append(calls, call)
			}
		}
	}
	direct := pkg.Func("direct")
	if constants := ssaflow.FixedArguments(calls[0].Common(), nil, direct, nil); constants[direct.Params[0]] != ssaflow.OutcomeTrue {
		t.Errorf("literal argument bindings = %v, want keep=true", constants)
	}
	if constants := ssaflow.FixedArguments(calls[1].Common(), nil, direct, nil); len(constants) != 0 {
		t.Errorf("unbound parameter argument bindings = %v, want none", constants)
	}
	known := ssaflow.FixedValues{caller.Params[0]: ssaflow.OutcomeFalse}
	if constants := ssaflow.FixedArguments(calls[1].Common(), nil, direct, known); constants[direct.Params[0]] != ssaflow.OutcomeFalse || len(constants) != 1 {
		t.Errorf("forwarded argument bindings = %v, want keep=false", constants)
	}
	if supplied := ssaflow.SuppliedCondition(calls[0].Common(), nil).Arguments; supplied.Bound != 1 || supplied.Values != 1 {
		t.Errorf("supplied constants = %+v, want bound 0x1 values 0x1", supplied)
	}

	for _, test := range []struct {
		name  string
		bound bool
	}{
		{"captured", true},
		// A cell written again after capture holds no single value.
		{"rewritten", false},
	} {
		function := pkg.Func(test.name)
		var closure *ssa.MakeClosure
		var deferred *ssa.Defer
		for _, instruction := range function.Blocks[0].Instrs {
			switch typed := instruction.(type) {
			case *ssa.MakeClosure:
				closure = typed
			case *ssa.Defer:
				deferred = typed
			}
		}
		body := closure.Fn.(*ssa.Function)
		known := ssaflow.FixedValues{function.Params[0]: ssaflow.OutcomeFalse}
		constants := ssaflow.FixedArguments(deferred.Common(), closure, body, known)
		if _, ok := constants[body.FreeVars[0]]; ok != test.bound {
			t.Errorf("%s: cell bound = %t, want %t", test.name, ok, test.bound)
		}
		if test.bound && effectReached(body, constants) {
			t.Errorf("%s: effect reached under keep=false", test.name)
		}
		if key := constants.Key(body); test.bound && key != "f0=2" {
			t.Errorf("%s: key = %q, want f0=2", test.name, key)
		}
	}
}

func TestFixedArgumentsBindNilness(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "constants", constantsSource)
	withoutSink := pkg.Func("withoutSink")
	for _, test := range []struct {
		outcome ssaflow.Outcome
		reached bool
	}{
		{ssaflow.OutcomeNil, true},
		{ssaflow.OutcomeNonNil, false},
	} {
		if got := effectReached(withoutSink, ssaflow.FixedValues{withoutSink.Params[0]: test.outcome}); got != test.reached {
			t.Errorf("withoutSink under %d: effect reached = %t, want %t", test.outcome, got, test.reached)
		}
	}
	var calls []*ssa.Call
	for _, block := range pkg.Func("nilCalls").Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				calls = append(calls, call)
			}
		}
	}
	if fixed := ssaflow.FixedArguments(calls[0].Common(), nil, withoutSink, nil); fixed[withoutSink.Params[0]] != ssaflow.OutcomeNil {
		t.Errorf("nil argument bindings = %v, want nil", fixed)
	}
	// An interface holding a typed nil pointer is not a nil interface.
	if fixed := ssaflow.FixedArguments(calls[1].Common(), nil, withoutSink, nil); fixed[withoutSink.Params[0]] != ssaflow.OutcomeNonNil {
		t.Errorf("typed nil argument bindings = %v, want non-nil", fixed)
	}
	// A pointer the callee never compares with nil adds no binding.
	if fixed := ssaflow.FixedArguments(calls[2].Common(), nil, pkg.Func("untested"), nil); len(fixed) != 0 {
		t.Errorf("untested pointer bindings = %v, want none", fixed)
	}
}
