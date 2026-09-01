package testpolicy

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This file proves when a testing handle is captured only by closures returned
// from a factory. Analysis stops at any invocation, storage, argument use, or
// other operational escape because the factory may then affect attribution.

func testingHandleCapturedOnlyByReturnedClosures(function *ssa.Function, handle *ssa.Parameter) bool {
	references := handle.Referrers()
	if references == nil || len(*references) == 0 {
		return false
	}
	closures := map[*ssa.MakeClosure]bool{}
	bindings := map[ssa.Value]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			closure, ok := instruction.(*ssa.MakeClosure)
			if !ok || !closureCapturesTestingHandle(closure, handle) || !closureUsedOnlyByReturns(closure) {
				continue
			}
			closures[closure] = true
			for _, binding := range closure.Bindings {
				if ssaflow.CapturedBindingMatches(binding, handle) {
					bindings[binding] = true
				}
			}
		}
	}
	if len(closures) == 0 {
		return false
	}
	return testingHandleReferencesOnlyClosures(*references, handle, closures, bindings) &&
		capturedBindingsReferenceOnlyClosures(bindings, handle, closures)
}

func testingHandleReferencesOnlyClosures(
	references []ssa.Instruction,
	handle *ssa.Parameter,
	closures map[*ssa.MakeClosure]bool,
	bindings map[ssa.Value]bool,
) bool {
	for _, reference := range references {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.MakeClosure:
			if closures[typed] {
				continue
			}
		case *ssa.Store:
			if ssaflow.SameValue(typed.Val, handle) && bindings[typed.Addr] {
				continue
			}
		}
		return false
	}
	return true
}

func capturedBindingsReferenceOnlyClosures(
	bindings map[ssa.Value]bool,
	handle *ssa.Parameter,
	closures map[*ssa.MakeClosure]bool,
) bool {
	for binding := range bindings {
		if binding.Referrers() == nil {
			return false
		}
		for _, reference := range *binding.Referrers() {
			switch typed := reference.(type) {
			case *ssa.DebugRef:
				continue
			case *ssa.MakeClosure:
				if closures[typed] {
					continue
				}
			case *ssa.Store:
				if typed.Addr == binding && ssaflow.SameValue(typed.Val, handle) {
					continue
				}
			}
			return false
		}
	}
	return true
}

func closureCapturesTestingHandle(closure *ssa.MakeClosure, handle *ssa.Parameter) bool {
	for _, binding := range closure.Bindings {
		if ssaflow.CapturedBindingMatches(binding, handle) {
			return true
		}
	}
	return false
}

func closureUsedOnlyByReturns(closure *ssa.MakeClosure) bool {
	onlyReturned, reachesReturn := valueUsedOnlyByReturns(closure, map[ssa.Value]bool{})
	return onlyReturned && reachesReturn
}

// Returned callbacks may pass through compiler-created interface, conversion,
// or phi values before the Return instruction. Follow only those transparent
// wrappers and reject every operational use: invoking, storing, or passing the
// callback would make the outer helper part of the eventual failure path.
func valueUsedOnlyByReturns(value ssa.Value, seen map[ssa.Value]bool) (bool, bool) {
	if value == nil || value.Referrers() == nil {
		return false, false
	}
	if seen[value] {
		return true, false
	}
	seen[value] = true
	reachesReturn := false
	for _, reference := range *value.Referrers() {
		if _, debug := reference.(*ssa.DebugRef); debug {
			continue
		}
		if returned, ok := reference.(*ssa.Return); ok && ssaflow.ReturnSameValue(returned, value) {
			reachesReturn = true
			continue
		}
		wrapped, ok := transparentClosureWrapper(reference, value)
		if !ok {
			return false, false
		}
		onlyReturned, wrappedReturns := valueUsedOnlyByReturns(wrapped, seen)
		if !onlyReturned {
			return false, false
		}
		reachesReturn = reachesReturn || wrappedReturns
	}
	return true, reachesReturn
}

func transparentClosureWrapper(reference ssa.Instruction, value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA wrappers have distinct concrete types.
	switch typed := reference.(type) {
	case *ssa.ChangeInterface:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.ChangeType:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.Convert:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.MakeInterface:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if ssaflow.SameValue(edge, value) {
				return typed, true
			}
		}
	}
	return nil, false
}

func emitReturnedTestingClosureDecision(pass *analysis.Pass, function *ssa.Function) {
	checkID := string(check.TestHelperMarker)
	if !analysisTrace.Enabled("testpolicy", checkID) {
		return
	}
	// Calling Helper in an outer factory cannot affect a failure raised later by
	// its returned callback. Civitai uses this shape for an HTTP test handler:
	// https://github.com/civitai/cli/blob/bc830b105867ae4234ddd7dd23f3f7680a2cbe3c/internal/cmd/app_listing_test.go#L321-L348
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "testpolicy",
		Check:    checkID,
		Phase:    "decision",
		Reason:   "returned-testing-callback",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      function.Pos(),
		Function: function.String(),
	})
}
