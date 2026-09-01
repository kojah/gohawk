package ssautil

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestCompletionEvidenceReasons(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}

func (*closer) Close() {}

func closeHelper(value *closer) {
	value.Close()
}

func evidence(value *closer) {
	defer func() { value.Close() }()
	go func() { value.Close() }()
	closeHelper(value)
}
`)
	function := pkg.Func("evidence")
	target := function.Params[0]
	tests := []struct {
		name   string
		match  func(ssa.Instruction) bool
		modes  CompletionMode
		reason EvidenceReason
	}{
		{
			name: "deferred",
			match: func(instruction ssa.Instruction) bool {
				_, ok := instruction.(*ssa.Defer)
				return ok
			},
			modes:  CompletionDeferred,
			reason: EvidenceDeferredCompletion,
		},
		{
			name: "started",
			match: func(instruction ssa.Instruction) bool {
				_, ok := instruction.(*ssa.Go)
				return ok
			},
			modes:  CompletionInStartedClosure,
			reason: EvidenceStartedCompletion,
		},
		{
			name: "helper",
			match: func(instruction ssa.Instruction) bool {
				common := InstructionCall(instruction)
				return common != nil && CallName(common) == "closeHelper"
			},
			modes:  CompletionByHelper,
			reason: EvidenceHelperCompletion,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			instruction := findSSAInstruction(t, function, test.match)
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      target,
				Methods:     []string{"Close"},
				Modes:       test.modes,
			})
			if !proof.Proven() || proof.Reason != test.reason || proof.Method != "Close" {
				t.Fatalf("ProveCompletion() = %#v, want reason %q and method Close", proof, test.reason)
			}
		})
	}

	if proof := ProveCompletion(CompletionRequest{
		Instruction: findSSAInstruction(t, function, tests[0].match),
		Target:      target,
		Methods:     []string{"Close"},
	}); proof.Proven() || proof.State != EvidenceUnknown || proof.Reason != EvidenceUnavailable {
		t.Fatalf("ProveCompletion() with no accepted modes = %#v, want unavailable proof", proof)
	}
}

func TestOwnershipTransferEvidenceReason(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}
type owner struct { value *closer }

func store(target *owner, value *closer) {
	target.value = value
}
`)
	function := pkg.Func("store")
	target := function.Params[1]
	store := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.Store)
		return ok
	})
	proof := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: store,
		Value:       target,
		Modes:       TransferStoredInField,
	})
	if !proof.Proven() || proof.Reason != EvidenceStoredInField {
		t.Fatalf("ProveOwnershipTransfer() = %#v, want stored-in-field proof", proof)
	}
	if rejected := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: store,
		Value:       target,
		Modes:       TransferStoredInGlobal,
	}); rejected.Proven() || rejected.State != EvidenceDisproven {
		t.Fatalf("ProveOwnershipTransfer() = %#v, want disproven relationship", rejected)
	}
}

func TestIdentityEvidenceReasons(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}
type owner struct {
	value *closer
	other *closer
}

func consume(...*closer) {}

func fields(left, right *owner) {
	consume(left.value, right.value, right.other)
}

`)
	function := pkg.Func("fields")
	var fields []*ssa.FieldAddr
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if field, ok := instruction.(*ssa.FieldAddr); ok {
				fields = append(fields, field)
			}
		}
	}
	if len(fields) != 3 {
		t.Fatalf("field address count = %d, want 3", len(fields))
	}

	direct := ProveIdentity(AccessPath{Value: function.Params[0]}, AccessPath{Value: function.Params[0]})
	if !direct.Proven() || direct.Reason != EvidenceSameValue {
		t.Fatalf("direct identity = %#v, want same-value proof", direct)
	}
	mapped := ProveIdentity(
		AccessPath{Value: fields[0], Root: function.Params[0]},
		AccessPath{Value: fields[1], Root: function.Params[1]},
	)
	if !mapped.Proven() || mapped.Reason != EvidenceSameAccessPath {
		t.Fatalf("mapped identity = %#v, want same-access-path proof", mapped)
	}
	rejected := ProveIdentity(
		AccessPath{Value: fields[0], Root: function.Params[0]},
		AccessPath{Value: fields[2], Root: function.Params[1]},
	)
	if rejected.Proven() || rejected.State != EvidenceDisproven {
		t.Fatalf("identity = %#v, want different fields disproven", rejected)
	}
}

func TestEvidenceQueryMemoizesProofs(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}

func (*closer) Close() {}

func closeHelper(value *closer) {
	value.Close()
}

func caller(value *closer) {
	closeHelper(value)
}
`)
	function := pkg.Func("caller")
	instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		common := InstructionCall(instruction)
		return common != nil && CallName(common) == "closeHelper"
	})
	request := CompletionRequest{
		Instruction: instruction,
		Target:      function.Params[0],
		Methods:     []string{"Close"},
		Modes:       CompletionByHelper,
	}
	var query EvidenceQuery
	first := query.Completion(request)
	second := query.Completion(request)
	if !first.Proven() || first != second {
		t.Fatalf("memoized completion proofs differ: first=%#v second=%#v", first, second)
	}
	if len(query.completions) != 1 {
		t.Fatalf("completion cache entries = %d, want 1", len(query.completions))
	}
}
