package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestDeferredCallbackCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

import "sync"

type locker struct{}
func (*locker) Unlock() {}

func discard(callback func()) func() { return func() {} }
func observe(*func()) {}

func accepted(value *locker) {
	callback := func() { value.Unlock() }
	defer callback()
}
func acceptedOnce(value *locker) {
	callback := sync.OnceFunc(func() { value.Unlock() })
	defer callback()
}
func acceptedNestedOnce(value *locker) {
	callback := sync.OnceFunc(func() { value.Unlock() })
	defer func() { callback() }()
}
func differentReceiver(value, other *locker) {
	callback := func() { other.Unlock() }
	defer callback()
}
func immediate(value *locker) {
	callback := func() { value.Unlock() }
	callback()
}
func discardedWrapper(value *locker) {
	defer discard(func() { value.Unlock() })()
}
func existentialPhi(value *locker, unlock bool) {
	callback := func() {}
	if unlock {
		callback = func() { value.Unlock() }
	}
	defer callback()
}
func ambiguousStores(value *locker, replace bool) {
	callback := func() { value.Unlock() }
	observe(&callback)
	if replace {
		callback = func() { value.Unlock() }
	}
	defer callback()
}
func nonDominatingStore(value *locker, assign bool) {
	var callback func()
	if assign {
		callback = func() { value.Unlock() }
	}
	observe(&callback)
	defer callback()
}
`)
	tests := []struct {
		name     string
		function string
		deferred bool
		want     bool
	}{
		{name: "deferred callback", function: "accepted", deferred: true, want: true},
		{name: "documented OnceFunc wrapper", function: "acceptedOnce", deferred: true, want: true},
		{name: "nested deferred OnceFunc callback", function: "acceptedNestedOnce", deferred: true, want: true},
		{name: "different receiver", function: "differentReceiver", deferred: true},
		{name: "non-deferred callback", function: "immediate"},
		{name: "discarding wrapper result", function: "discardedWrapper", deferred: true},
		{name: "unlock or noop phi", function: "existentialPhi", deferred: true},
		{name: "ambiguous stores", function: "ambiguousStores", deferred: true},
		{name: "non-dominating store", function: "nonDominatingStore", deferred: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.function)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				if test.deferred {
					_, ok := instruction.(*ssa.Defer)
					return ok
				}
				_, ok := instruction.(*ssa.Call)
				return ok
			})
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      function.Params[0],
				Methods:     []string{"Unlock"},
				Modes:       CompletionByDeferredCallback,
			})
			if proof.Proven() != test.want {
				t.Fatalf("ProveCompletion() = %#v, want proven %v", proof, test.want)
			}
			if test.want && proof.Reason != EvidenceDeferredCallback {
				t.Fatalf("ProveCompletion() reason = %q, want %q", proof.Reason, EvidenceDeferredCallback)
			}
		})
	}
}
