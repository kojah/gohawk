package testpolicy

import (
	"github.com/kojah/gohawk/internal/ssaflow"

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
	onlyReturned, reachesReturn := valueUsedOnlyByReturns(ssaflow.NewReachingWalk(0), closure)
	return onlyReturned && reachesReturn
}

// Returned callbacks may pass through compiler-created interface, conversion,
// or phi values before the Return instruction. Follow only those transparent
// wrappers and reject every operational use: invoking, storing, or passing the
// callback would make the outer helper part of the eventual failure path.
func valueUsedOnlyByReturns(walk ssaflow.ReachingWalk, value ssa.Value) (bool, bool) {
	if value == nil || value.Referrers() == nil {
		return false, false
	}
	if !walk.Mark(value) {
		return true, false
	}
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
		onlyReturned, wrappedReturns := valueUsedOnlyByReturns(walk, wrapped)
		if !onlyReturned {
			return false, false
		}
		reachesReturn = reachesReturn || wrappedReturns
	}
	return true, reachesReturn
}

func transparentClosureWrapper(reference ssa.Instruction, value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA wrappers have distinct concrete types.
	wrapped, valueWrapper := reference.(ssa.Value)
	if valueWrapper {
		inner, transparent := ssaflow.UnwrapTransparentValue(
			wrapped,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		)
		if transparent {
			return wrapped, ssaflow.SameValue(inner, value)
		}
	}
	if typed, ok := reference.(*ssa.Phi); ok && ssaflow.PhiMergesValue(typed, value) {
		return typed, true
	}
	return nil, false
}

func emitReturnedTestingClosureDecision(pass *analysis.Pass, function *ssa.Function) {
	// Calling Helper in an outer factory cannot affect a failure raised later by
	// its returned callback. Civitai uses this shape for an HTTP test handler:
	// https://github.com/civitai/cli/blob/bc830b105867ae4234ddd7dd23f3f7680a2cbe3c/internal/cmd/app_listing_test.go#L321-L348
	traceHelperMarkerDecision(pass, function, "returned-testing-callback")
}
