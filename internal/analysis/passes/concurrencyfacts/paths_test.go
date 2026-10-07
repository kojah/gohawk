package concurrencyfacts

import (
	"go/constant"
	"go/token"
	"go/types"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBoundedBranchPaths(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "paths", `package paths
func branch(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func forward(a, b chan int, flag bool) { branch(a, b, flag) }
func launch(a, b chan int, flag bool) { go forward(a, b, flag) }
func selectAndLaunch(a, b chan int) { go func(){ close(a) }(); select { case <-b: default: } }
func selectCaller(a, b chan int) { selectAndLaunch(a, b) }
func nested(a, b chan int) { go selectAndLaunch(a, b) }
func armLaunch(a, b chan int) { select { case <-b: go func(){ close(a) }(); default: close(a) } }
func launchAndClose(a, b chan int) { go func(){ close(a) }(); close(b) }
func deferredLaunch(a, b chan int) { defer launchAndClose(a, b) }
func optional(a chan int, flag bool) { if flag { close(a) } }
func opaque(a chan int, flag bool, f func()) { if flag { close(a) } else { f() } }
func loop(a chan int, flag bool) { for flag { close(a) } }
func many(a chan int, x, y, z, w bool) {
 if x { close(a) }; if y { close(a) }; if z { close(a) }; if w { close(a) }
}
`)
	engine := NewEngine()
	linear := engine.linear.Function(pkg.Func("branch"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if linear.Reason != ReasonBranchEffectsDiffer || len(linear.Paths) != 0 {
		t.Fatalf("linear export built unpublishable paths: %+v", linear)
	}
	launch := engine.Root(pkg.Func("launch"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if len(launch.Workers) != 1 || !launch.Workers[0].Branches || len(launch.Workers[0].Alternatives) != 2 {
		t.Fatalf("forwarded worker alternatives = %+v", launch)
	}
	for _, name := range []string{"branch", "forward", "optional"} {
		got := engine.Function(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
		if got.Complete() || len(got.Paths) != 2 || got.Reason != ReasonBranchAlternatives {
			t.Errorf("%s = %+v, want two non-linear paths", name, got)
		}
		for _, path := range got.Paths {
			if !path.Complete() || len(path.Paths) != 0 {
				t.Errorf("%s has incomplete or nested path: %+v", name, path)
			}
		}
	}
	optional := engine.Function(pkg.Func("optional"), proofs.NewSearchBudget(proofs.SummaryBudget))
	if len(optional.Paths) == 2 && len(optional.Paths[0].Operations)+len(optional.Paths[1].Operations) != 1 {
		t.Error("optional cleanup lost its empty escape path")
	}
	for _, name := range []string{"opaque", "loop", "many", "selectCaller", "nested", "armLaunch", "deferredLaunch"} {
		got := engine.Function(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
		if got.Complete() || len(got.Paths) != 0 {
			t.Errorf("%s must remain unavailable, got %+v", name, got)
		}
	}
}

func TestParameterConditionNegationBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "negated", `package negated
func direct(flag bool) bool { return flag }
func odd(flag bool) bool { a := !flag; return a }
func even(flag bool) bool { a := !flag; b := !a; return b }
func loaded(flag *bool) bool { return !*flag }
func caller(supplied bool) {}
`)
	supplied := pkg.Func("caller").Params[0]
	literal := ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])
	for _, test := range []struct {
		name     string
		context  bool
		compared bool
		captured bool
		wantOK   bool
		wantHold bool
	}{
		{"direct", false, false, false, true, true},
		{"odd", false, false, false, true, false},
		{"even", false, false, false, true, true},
		{"loaded", false, false, false, false, false},
		{"odd", true, false, false, false, false},
		{"odd", false, true, false, false, false},
		{"direct", false, true, false, true, true},
		{"odd", false, false, true, false, false},
	} {
		fn := pkg.Func(test.name)
		condition := Condition{Value: ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0], Holds: true}
		if test.context {
			condition.Context = []token.Pos{1}
		}
		if test.compared {
			condition.Compared = literal
		}
		bindings := []ssacall.CallBinding{{Local: fn.Params[0], Supplied: supplied, Captured: test.captured}}
		value, holds, ok := parameterCondition(condition, bindings)
		if ok != test.wantOK || holds != test.wantHold || ok && value != supplied || !ok && value != nil {
			t.Errorf("%+v: bound %v, holds=%t ok=%t", test, value, holds, ok)
		}
	}
}

func TestEquivalentBranches(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branches", `package branches
func different(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func optional(a chan int, flag bool) { if flag { close(a) } }
func reordered(a, b chan int, flag bool) {
 if flag { close(a); close(b) } else { close(b); close(a) }
}
func loop(a chan int, flag bool) { for flag { a <- 1 } }
func opaque(a chan int, flag bool, f func()) { if flag { close(a) } else { f(); close(a) } }
func deferredDifferent(a chan int, flag bool) { if flag { defer close(a) } else { close(a) } }
func same(a chan int, flag bool) { if flag { a <- 1 } else { a <- 2 }; close(a) }
func early(a chan int, flag bool) { if flag { close(a); return }; close(a) }
func deferred(a chan int, flag bool) { defer close(a); if flag { a <- 1 } else { a <- 2 } }
func scalar(a chan int, flag bool) int { n := 1; if flag { n = 2 }; close(a); return n }
func diamonds(a chan int, x, y bool) {
 if x { a <- 1 } else { a <- 2 }; if y { close(a) } else { close(a) }
}
`)
	for _, name := range []string{"different", "optional", "reordered", "loop", "opaque", "deferredDifferent"} {
		t.Run(name, func(t *testing.T) {
			result := NewEngine().Function(pkg.Func(name), proofs.NewSearchBudget(2000))
			if result.Reason == ReasonNone || len(result.Operations) != 0 {
				t.Fatalf("incomplete branches produced effects: %+v", result)
			}
		})
	}
	for name, count := range map[string]int{"same": 2, "early": 1, "deferred": 2, "scalar": 1, "diamonds": 2} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			result := NewEngine().Function(function, proofs.NewSearchBudget(2000))
			if result.Reason != ReasonNone || len(result.Operations) != count {
				t.Fatalf("equivalent effects lost: %+v", result)
			}
			for _, operation := range result.Operations {
				if operation.Resource.Value != function.Params[0] {
					t.Errorf("wrong resource: %+v", operation)
				}
			}
		})
	}
	engine := NewEngine()
	if result := engine.Function(pkg.Func("diamonds"), proofs.NewSearchBudget(1)); result.Reason != ReasonBudgetExhausted {
		t.Fatalf("small branch budget: %+v", result)
	}
	if result := engine.Function(pkg.Func("diamonds"), proofs.NewSearchBudget(2000)); result.Reason != ReasonNone {
		t.Fatalf("budget-shortened branch summary poisoned cache: %+v", result)
	}
}

// Folding equal branches keeps each branch's source for attribution.
func TestFoldedBranchesKeepEverySource(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "foldedsources", `package foldedsources
func Send(ch chan int, fail bool) {
	if fail {
		ch <- 1
		return
	}
	ch <- 2
}
`)
	got := NewEngine().Function(pkg.Func("Send"), proofs.NewSearchBudget(2000))
	if got.Completeness() != CompleteWithEffects || len(got.Operations) != 1 {
		t.Fatalf("folded summary = %+v", got)
	}
	operation := got.Operations[0]
	if len(operation.Alternates) != 1 || operation.Alternates[0] == operation.Source || !operation.Alternates[0].IsValid() {
		t.Errorf("folded send = %+v, want one distinct alternate source", operation)
	}
}

// Each path alternative records the branch choices that select it. A helper's
// test of its own parameter binds to the caller's argument.
func TestPathConditions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "conditions", `package conditions
func branch(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func twice(a, b chan int, x, y bool) { branch(a, b, x); branch(a, b, y) }
func worker(a, b chan int, flag bool) { go branch(a, b, flag) }
`)
	engine := NewEngine()
	budget := func() *proofs.SearchBudget { return proofs.NewSearchBudget(4000) }
	function := pkg.Func("branch")
	got := engine.Function(function, budget())
	if len(got.Paths) != 2 {
		t.Fatalf("branch = %+v, want two paths", got)
	}
	for _, path := range got.Paths {
		if len(path.Conditions) != 1 || path.Conditions[0].Value != function.Params[2] || len(path.Conditions[0].Context) != 0 {
			t.Fatalf("path conditions = %+v, want one local condition on flag", path.Conditions)
		}
		closesA := path.Operations[0].Resource.Value == function.Params[0]
		if path.Conditions[0].Holds != closesA {
			t.Errorf("path %+v: condition polarity does not select its effects", path)
		}
	}
	// A helper's test of its own parameter becomes a test of the caller's
	// argument, so the two calls test the caller's x and y.
	twiceFunction := pkg.Func("twice")
	twice := engine.Function(twiceFunction, budget())
	if len(twice.Paths) != 4 {
		t.Fatalf("twice = %+v, want four combined paths", twice)
	}
	for _, path := range twice.Paths {
		if len(path.Conditions) != 2 || path.Conditions[0].Value != twiceFunction.Params[2] ||
			path.Conditions[1].Value != twiceFunction.Params[3] || len(path.Conditions[0].Context) != 0 {
			t.Errorf("twice path conditions = %+v, want the caller's x then y", path.Conditions)
		}
	}
	workerFunction := pkg.Func("worker")
	launched := engine.Root(workerFunction, budget())
	if len(launched.Workers) != 1 || len(launched.Workers[0].AlternativeConditions) != 2 {
		t.Fatalf("worker = %+v, want two conditioned alternatives", launched)
	}
	for _, conditions := range launched.Workers[0].AlternativeConditions {
		if len(conditions) != 1 || conditions[0].Value != workerFunction.Params[2] || len(conditions[0].Context) != 0 {
			t.Errorf("worker alternative conditions = %+v, want the parent's flag", conditions)
		}
	}
}
