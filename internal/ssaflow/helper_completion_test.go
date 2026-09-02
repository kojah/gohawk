package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestStaticHelperDeferredCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}

func (*closer) Close() {}

type owner struct { body *closer }

func deferred(value *closer) {
	defer func() { value.Close() }()
}

func deferredField(value *owner) {
	defer func() { value.body.Close() }()
}

func conditional(value *closer, enabled bool) {
	if enabled {
		defer func() { value.Close() }()
	}
}

func different(value, other *closer) {
	defer func() { other.Close() }()
}

func reassignedBeforeDefer(value, other *closer) {
	value = other
	defer func() { value.Close() }()
}

func reassignedAfterDefer(value, other *closer) {
	defer func() { value.Close() }()
	value = other
}

type consumer interface { Consume(*closer) }

func opaque(value *closer, target consumer) {
	target.Consume(value)
}

func accepted(value *closer) { deferred(value) }
func acceptedField(value *owner) { deferredField(value) }
func rejectedConditional(value *closer, enabled bool) { conditional(value, enabled) }
func rejectedDifferent(value, other *closer) { different(value, other) }
func rejectedReassignedBefore(value, other *closer) { reassignedBeforeDefer(value, other) }
func rejectedReassignedAfter(value, other *closer) { reassignedAfterDefer(value, other) }
func rejectedOpaque(value *closer, target consumer) { opaque(value, target) }
`)
	tests := []struct {
		name   string
		proven bool
	}{
		{name: "accepted", proven: true},
		{name: "acceptedField", proven: true},
		{name: "rejectedConditional"},
		{name: "rejectedDifferent"},
		{name: "rejectedReassignedBefore"},
		{name: "rejectedReassignedAfter"},
		{name: "rejectedOpaque"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				common := InstructionCall(instruction)
				return common != nil && common.StaticCallee() != nil
			})
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      function.Params[0],
				Methods:     []string{"Close"},
				Modes:       CompletionByHelper,
			})
			if proof.Proven() != test.proven {
				t.Fatalf("ProveCompletion() = %#v, proven = %t", proof, test.proven)
			}
			if test.proven && proof.Reason != EvidenceHelperCompletion {
				t.Fatalf("ProveCompletion() reason = %q, want %q", proof.Reason, EvidenceHelperCompletion)
			}
		})
	}
}

func TestDeferredArgumentCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}
func (*closer) Close() {}

type owner struct { body *closer }

func selectBody(value *owner) *closer { return value.body }

func accepted(value *owner) {
	defer func(body *closer) { body.Close() }(value.body)
}
func differentOwner(value, other *owner) {
	defer func(body *closer) { body.Close() }(other.body)
}
func differentParameter(value *owner) {
	defer func(other, body *closer) { other.Close() }(&closer{}, value.body)
}
func conditional(value *owner, enabled bool) {
	defer func(body *closer) {
		if enabled { body.Close() }
	}(value.body)
}
func immediate(value *owner) {
	func(body *closer) { body.Close() }(value.body)
}
func returnedProjection(value *owner) {
	defer func(body *closer) { body.Close() }(selectBody(value))
}
`)
	tests := []struct {
		name     string
		function string
		want     bool
	}{
		{name: "deferred projected argument", function: "accepted", want: true},
		{name: "different owner", function: "differentOwner"},
		{name: "different parameter", function: "differentParameter"},
		{name: "conditional close", function: "conditional"},
		{name: "nondeferred call", function: "immediate"},
		{name: "projection returned by helper", function: "returnedProjection"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.function)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				common := InstructionCall(instruction)
				return common != nil && common.StaticCallee() != nil && common.StaticCallee().Parent() == function
			})
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      function.Params[0],
				Methods:     []string{"Close"},
				Modes:       CompletionByDeferredArgument,
			})
			if proof.Proven() != test.want {
				t.Fatalf("ProveCompletion() = %#v, want proven %v", proof, test.want)
			}
			if test.want && proof.Reason != EvidenceDeferredArgumentCompletion {
				t.Fatalf("ProveCompletion() reason = %q, want %q", proof.Reason, EvidenceDeferredArgumentCompletion)
			}
		})
	}
}
