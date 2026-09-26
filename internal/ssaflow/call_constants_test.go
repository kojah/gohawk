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
`

// effectReached reports whether some block reachable under constants calls
// effect.
func effectReached(function *ssa.Function, constants ssaflow.BooleanConstants) bool {
	for _, block := range ssaflow.ReachableBlocksAssuming(function, constants) {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() != nil && call.Common().StaticCallee().Name() == "effect" {
				return true
			}
		}
	}
	return false
}

func TestBooleanConstantsDecideBranches(t *testing.T) {
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
		constants := ssaflow.BooleanConstants{function.Params[0]: test.value}
		if got := effectReached(function, constants); got != test.reached {
			t.Errorf("%s(%t): effect reached = %t, want %t", test.name, test.value, got, test.reached)
		}
		_, decided := constants.DecidedSuccessor(function.Blocks[0])
		if decided != test.decides {
			t.Errorf("%s(%t): decided = %t, want %t", test.name, test.value, decided, test.decides)
		}
	}
}

func TestConstantBooleanArgumentsBindCallsAndCells(t *testing.T) {
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
	if constants := ssaflow.ConstantBooleanArguments(calls[0].Common(), nil, direct, nil); constants[direct.Params[0]] != true {
		t.Errorf("literal argument bindings = %v, want keep=true", constants)
	}
	if constants := ssaflow.ConstantBooleanArguments(calls[1].Common(), nil, direct, nil); len(constants) != 0 {
		t.Errorf("unbound parameter argument bindings = %v, want none", constants)
	}
	known := ssaflow.BooleanConstants{caller.Params[0]: false}
	if constants := ssaflow.ConstantBooleanArguments(calls[1].Common(), nil, direct, known); constants[direct.Params[0]] != false || len(constants) != 1 {
		t.Errorf("forwarded argument bindings = %v, want keep=false", constants)
	}
	if bound, values := ssaflow.ConstantBooleanArgumentBits(calls[0].Common(), nil); bound != 1 || values != 1 {
		t.Errorf("argument bits = %#x, %#x; want 0x1, 0x1", bound, values)
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
		known := ssaflow.BooleanConstants{function.Params[0]: false}
		constants := ssaflow.ConstantBooleanArguments(deferred.Common(), closure, body, known)
		if _, ok := constants[body.FreeVars[0]]; ok != test.bound {
			t.Errorf("%s: cell bound = %t, want %t", test.name, ok, test.bound)
		}
		if test.bound && effectReached(body, constants) {
			t.Errorf("%s: effect reached under keep=false", test.name)
		}
		if key := constants.Key(body); test.bound && key != "f0=false" {
			t.Errorf("%s: key = %q, want f0=false", test.name, key)
		}
	}
}
