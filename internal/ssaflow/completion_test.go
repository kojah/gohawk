package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

const completionFixture = `
package ssaflowtest

import (
	"sync"
	"testing"
)

type closer struct{}

func (*closer) Close() {}

type owner struct{ body *closer }

func closeHelper(value *closer)                  { value.Close() }
func closeOwner(value *owner)                    { value.body.Close() }
func closeOther(other, value *closer)            { other.Close() }
func closeMaybe(value *closer, enabled bool)     { if enabled { value.Close() } }
func closeChain(value *closer)                   { closeHelper(value) }
func closeLater(value *closer)                   { defer closeHelper(value) }
func invoke(callback func())                     { callback() }
func maybeInvoke(callback func(), enabled bool)  { if enabled { callback() } }
func startWaiter(value *closer)                  { go func() { value.Close() }() }
func register(t *testing.T, value *closer)       { t.Cleanup(func() { value.Close() }) }
func opaque(value *closer, callback func(*closer)) { callback(value) }

func deferredLiteral(value *closer)            { defer func() { value.Close() }() }
func deferredHelper(value *closer)             { defer closeHelper(value) }
func deferredConditional(value *closer, ok bool) { defer func() { if ok { value.Close() } }() }
func deferredOtherReceiver(value, other *closer) { defer func() { other.Close() }() }
func deferredReassignedAfter(value, other *closer) {
	defer func() { value.Close() }()
	value = other
}
func deferredStoredCallback(value *closer) {
	callback := func() { value.Close() }
	defer callback()
}
func deferredOnceCallback(value *closer) {
	callback := sync.OnceFunc(func() { value.Close() })
	defer callback()
}
func deferredPhiCallback(value *closer, unlock bool) {
	callback := func() {}
	if unlock {
		callback = func() { value.Close() }
	}
	defer callback()
}
func deferredProjection(value *owner)          { defer closeHelper(value.body) }
func deferredProjectionLiteral(value *owner)   { defer func(body *closer) { body.Close() }(value.body) }
func deferredProjectionOther(value, other *owner) { defer closeOther(other.body, value.body) }
func deferredOwnerCapture(value *owner)        { defer func() { value.body.Close() }() }
func deferredBoundCallback(value *closer)      { defer invoke(value.Close) }
func deferredBoundConditional(value *closer, ok bool) { defer maybeInvoke(value.Close, ok) }
func calledHelper(value *closer)               { closeHelper(value) }
func calledOwner(value *closer)                { closeOwner(&owner{body: value}) }
func calledConditional(value *closer, ok bool) { closeMaybe(value, ok) }
func calledChain(value *closer)                { closeChain(value) }
func calledDeferringHelper(value *closer)      { closeLater(value) }
func calledLiteral(value *closer)              { func() { value.Close() }() }
func calledStarter(value *closer)              { startWaiter(value) }
func calledRegistrar(t *testing.T, value *closer) { register(t, value) }
func calledOpaque(value *closer, callback func(*closer)) { opaque(value, callback) }
func startedLiteral(value *closer)             { go func() { value.Close() }() }
func startedGroup(value *closer) {
	var group sync.WaitGroup
	group.Go(func() { value.Close() })
	group.Wait()
}
func registeredCleanup(t *testing.T, value *closer) { t.Cleanup(func() { value.Close() }) }
func recursiveLiteral(value *closer) {
	var walk func(int)
	walk = func(depth int) {
		if depth > 0 {
			walk(depth - 1)
		}
		value.Close()
	}
	defer walk(3)
}
func selfCapturingCallback(value *closer) {
	var callback func()
	callback = func() { _ = callback; value.Close() }
	defer callback()
}
func onceCalledNow(value *closer) {
	callback := sync.OnceFunc(func() { value.Close() })
	callback()
}
`

var completionCases = []struct {
	function string
	proven   bool
	reason   EvidenceReason
	coverage CompletionCoverage
	unknown  bool
}{
	{function: "deferredLiteral", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredHelper", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredConditional"},
	{function: "deferredConditional", proven: true, reason: EvidenceDeferredCompletion, coverage: CoverageAnywhere},
	{function: "deferredOtherReceiver"},
	{function: "deferredReassignedAfter"},
	{function: "deferredStoredCallback", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredOnceCallback", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredPhiCallback"},
	{function: "deferredProjection", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredProjectionLiteral", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredProjectionOther"},
	{function: "deferredOwnerCapture", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredBoundCallback", proven: true, reason: EvidenceDeferredCompletion},
	{function: "deferredBoundConditional"},
	{function: "calledHelper", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledOwner", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledConditional"},
	{function: "calledChain", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledDeferringHelper", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledLiteral", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledStarter", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledRegistrar", proven: true, reason: EvidenceCalledCompletion},
	{function: "calledOpaque"},
	{function: "startedLiteral", proven: true, reason: EvidenceStartedCompletion},
	{function: "startedGroup", proven: true, reason: EvidenceStartedCompletion},
	{function: "registeredCleanup", proven: true, reason: EvidenceDeferredCompletion},
	// A literal that captures the variable holding itself must terminate the
	// search; the deferred call still completes the target on every return.
	{function: "recursiveLiteral", proven: true, reason: EvidenceDeferredCompletion},
	{function: "selfCapturingCallback", proven: true, reason: EvidenceDeferredCompletion},
	{function: "onceCalledNow", unknown: true},
}

// The completion engine has one search; these cases pin its boundaries by
// launch form (deferred, called, started, registered), by how the target is
// mapped into the callee (exact, projection, owner, callback), and by the
// coverage the caller asks for.
func TestCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, completionFixture)
	for _, test := range completionCases {
		name := test.function
		if test.coverage == CoverageAnywhere {
			name += "/anywhere"
		}
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(test.function)
			target := function.Params[0]
			if len(function.Params) > 1 && function.Params[0].Name() == "t" {
				target = function.Params[1]
			}
			instruction := findLaunch(t, function)
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      target,
				Methods:     []string{"Close"},
				Coverage:    test.coverage,
			})
			if test.unknown {
				if proof.State != EvidenceUnknown {
					t.Fatalf("ProveCompletion() = %#v, want unknown", proof)
				}
				return
			}
			if proof.Proven() != test.proven {
				t.Fatalf("ProveCompletion() = %#v, want proven %v", proof, test.proven)
			}
			if test.proven && (proof.Reason != test.reason || proof.Method != "Close") {
				t.Fatalf("ProveCompletion() reason = %q, want %q with method Close", proof.Reason, test.reason)
			}
		})
	}
}

// findLaunch returns the function's defer or go statement, or else its first
// call to a non-builtin function, which is the launch under test.
func findLaunch(t *testing.T, function *ssa.Function) ssa.Instruction { //nolint:ireturn // SSA launches have several instruction forms.
	t.Helper()
	var call ssa.Instruction
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch typed := instruction.(type) {
			case *ssa.Defer, *ssa.Go:
				return instruction
			case *ssa.Call:
				_, builtin := typed.Common().Value.(*ssa.Builtin)
				if call == nil && !builtin && CallName(typed.Common()) != "Wait" {
					call = instruction
				}
			}
		}
	}
	if call == nil {
		t.Fatalf("no launch found in %s", function.Name())
	}
	return call
}
