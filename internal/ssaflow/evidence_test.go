package ssaflow

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

func invoke(callback func()) {
	callback()
}

func evidence(value *closer) {
	defer func() { value.Close() }()
	defer invoke(value.Close)
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
			name: "deferred helper callback",
			match: func(instruction ssa.Instruction) bool {
				common := InstructionCall(instruction)
				return common != nil && CallName(common) == "invoke"
			},
			modes:  CompletionByDeferredHelperCallback,
			reason: EvidenceDeferredHelperCallback,
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

func TestDeferredHelperCallbackCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}

func (*closer) Close() {}

func invoke(callback func()) { callback() }

func maybeInvoke(callback func(), enabled bool) {
	if enabled {
		callback()
	}
}

func accepted(value *closer) { defer invoke(value.Close) }
func conditional(value *closer, enabled bool) { defer maybeInvoke(value.Close, enabled) }
func immediate(value *closer) { invoke(value.Close) }
func unrelated(value, other *closer) { defer invoke(other.Close) }
`)
	tests := []struct {
		name     string
		function string
		want     bool
	}{
		{name: "deferred unconditional helper", function: "accepted", want: true},
		{name: "conditional helper", function: "conditional"},
		{name: "nondeferred call", function: "immediate"},
		{name: "unrelated receiver", function: "unrelated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.function)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				common := InstructionCall(instruction)
				return common != nil && (CallName(common) == "invoke" || CallName(common) == "maybeInvoke")
			})
			got := DeferredHelperInvokesBoundMethodOnEveryReturn(instruction, "Close", function.Params[0])
			if got != test.want {
				t.Fatalf("DeferredHelperInvokesBoundMethodOnEveryReturn() = %v, want %v", got, test.want)
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

func TestCalledClosureCompletionBeforeBranch(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type closer struct{}

func (*closer) Close() {}

func nestedDeferred(value *closer) {
	func() { defer func() { value.Close() }() }()
}

func conditionalNestedDeferred(value *closer, enabled bool) {
	func() {
		if enabled {
			defer func() { value.Close() }()
		}
	}()
}
`)
	nested := pkg.Func("nestedDeferred")
	nestedCall := findSSAInstruction(t, nested, func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			return false
		}
		_, ok = call.Common().Value.(*ssa.MakeClosure)
		return ok
	})
	nestedProof := ProveCompletion(CompletionRequest{
		Instruction: nestedCall,
		Target:      nested.Params[0],
		Methods:     []string{"Close"},
		Modes:       CompletionInCalledClosureBeforeBranch,
	})
	if !nestedProof.Proven() || nestedProof.Reason != EvidenceCalledCompletionBeforeBranch {
		t.Fatalf("nested deferred completion = %#v, want called-closure proof", nestedProof)
	}

	conditional := pkg.Func("conditionalNestedDeferred")
	conditionalCall := findSSAInstruction(t, conditional, func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			return false
		}
		_, ok = call.Common().Value.(*ssa.MakeClosure)
		return ok
	})
	if proof := ProveCompletion(CompletionRequest{
		Instruction: conditionalCall,
		Target:      conditional.Params[0],
		Methods:     []string{"Close"},
		Modes:       CompletionInCalledClosureBeforeBranch,
	}); proof.Proven() {
		t.Fatalf("conditional nested deferred completion = %#v, want no proof", proof)
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

func TestLifecycleOwnerTransferRequiresStructuralEvidence(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type resource struct{}
type owner struct { value *resource }

func (*owner) Close() {}
func (*owner) Add(*resource) {}
func (current *owner) With(*resource) *owner { return current }
func (current *owner) Store(value *resource) { current.value = value }
func makeOwner(value *resource) *owner { return &owner{value: value} }

func noOpAdd(current *owner, value *resource) { current.Add(value) }
func noOpWith(current *owner, value *resource) { current.With(value) }
func visibleStore(current *owner, value *resource) { current.Store(value) }
func returnedOwner(value *resource) *owner { return makeOwner(value) }
`)
	callNamed := func(t *testing.T, function, name string) (*ssa.Call, ssa.Value) {
		t.Helper()
		current := pkg.Func(function)
		instruction := findSSAInstruction(t, current, func(instruction ssa.Instruction) bool {
			common := InstructionCall(instruction)
			return common != nil && CallName(common) == name
		})
		call, ok := instruction.(*ssa.Call)
		if !ok {
			t.Fatalf("%s instruction is %T, want *ssa.Call", function, instruction)
		}
		return call, current.Params[len(current.Params)-1]
	}
	for _, test := range []struct {
		function string
		name     string
	}{
		{function: "noOpAdd", name: "Add"},
		{function: "noOpWith", name: "With"},
	} {
		t.Run(test.function, func(t *testing.T) {
			call, value := callNamed(t, test.function, test.name)
			proof := ProveOwnershipTransfer(OwnershipTransferRequest{
				Instruction: call,
				Value:       value,
				Modes:       TransferToLifecycleOwner,
			})
			if proof.Proven() || proof.State != EvidenceDisproven || proof.Reason != EvidenceNotFound {
				t.Fatalf("ProveOwnershipTransfer() = %#v, want no lifecycle-owner proof", proof)
			}
		})
	}

	storeCall, stored := callNamed(t, "visibleStore", "Store")
	storeProof := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: storeCall,
		Value:       stored,
		Modes:       TransferToReceiver,
	})
	if !storeProof.Proven() || storeProof.Reason != EvidenceTransferredToReceiver {
		t.Fatalf("visible store proof = %#v, want receiver transfer", storeProof)
	}

	ownerCall, owned := callNamed(t, "returnedOwner", "makeOwner")
	ownerProof := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: ownerCall,
		Value:       owned,
		Modes:       TransferToReturnedOwner,
	})
	if !ownerProof.Proven() || ownerProof.Reason != EvidenceTransferredToReturnedOwner {
		t.Fatalf("returned owner proof = %#v, want returned-owner transfer", ownerProof)
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
