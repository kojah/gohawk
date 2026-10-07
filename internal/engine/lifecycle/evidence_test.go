package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

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
	if !proof.Proven() || proof.Reason != proofs.EvidenceStoredInField {
		t.Fatalf("ProveOwnershipTransfer() = %#v, want stored-in-field proof", proof)
	}
	if rejected := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: store,
		Value:       target,
		Modes:       TransferStoredInGlobal,
	}); rejected.Proven() || rejected.State != proofs.EvidenceDisproven {
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
			common := ssaflow.InstructionCall(instruction)
			return common != nil && ssaflow.CallName(common) == name
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
			if proof.Proven() || proof.State != proofs.EvidenceDisproven || proof.Reason != proofs.EvidenceNotFound {
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
	if !storeProof.Proven() || storeProof.Reason != proofs.EvidenceTransferredToReceiver {
		t.Fatalf("visible store proof = %#v, want receiver transfer", storeProof)
	}

	ownerCall, owned := callNamed(t, "returnedOwner", "makeOwner")
	ownerProof := ProveOwnershipTransfer(OwnershipTransferRequest{
		Instruction: ownerCall,
		Value:       owned,
		Modes:       TransferToReturnedOwner,
	})
	if !ownerProof.Proven() || ownerProof.Reason != proofs.EvidenceTransferredToReturnedOwner {
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

	direct := ssaflow.ProveIdentityWithin(ssaflow.AccessPath{Value: function.Params[0]}, ssaflow.AccessPath{Value: function.Params[0]}, nil)
	if !direct.Proven() || direct.Reason != proofs.EvidenceSameValue {
		t.Fatalf("direct identity = %#v, want same-value proof", direct)
	}
	mapped := ssaflow.ProveIdentityWithin(
		ssaflow.AccessPath{Value: fields[0], Root: function.Params[0]},
		ssaflow.AccessPath{Value: fields[1], Root: function.Params[1]}, nil,
	)
	if !mapped.Proven() || mapped.Reason != proofs.EvidenceSameAccessPath {
		t.Fatalf("mapped identity = %#v, want same-access-path proof", mapped)
	}
	rejected := ssaflow.ProveIdentityWithin(
		ssaflow.AccessPath{Value: fields[0], Root: function.Params[0]},
		ssaflow.AccessPath{Value: fields[2], Root: function.Params[1]}, nil,
	)
	if rejected.Proven() || rejected.State != proofs.EvidenceDisproven {
		t.Fatalf("identity = %#v, want different fields disproven", rejected)
	}
}

func TestLocalEvidenceMemoizesProofs(t *testing.T) {
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
		common := ssaflow.InstructionCall(instruction)
		return common != nil && ssaflow.CallName(common) == "closeHelper"
	})
	request := CompletionRequest{
		Instruction: instruction,
		Target:      function.Params[0],
		Methods:     []string{"Close"},
	}
	var evidence LocalEvidence
	first := evidence.Completion(request)
	second := evidence.Completion(request)
	if !first.Proven() || first != second {
		t.Fatalf("memoized completion proofs differ: first=%#v second=%#v", first, second)
	}
	if len(evidence.completions) != 1 {
		t.Fatalf("completion cache entries = %d, want 1", len(evidence.completions))
	}
}

func TestLocalCompletionCutoffIsNotCached(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type closer struct{}
 func (*closer) Close(){}
 func cleanup(value *closer){value.Close()}
 func caller(value *closer){cleanup(value)}
 `)
	fn := pkg.Func("caller")
	instruction := findSSAInstruction(t, fn, func(instruction ssa.Instruction) bool {
		return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "cleanup"
	})
	request := CompletionRequest{Instruction: instruction, Target: fn.Params[0], Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(0)}
	var evidence LocalEvidence
	cut := evidence.Completion(request)
	if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || len(evidence.completions) != 0 {
		t.Fatalf("interrupted completion cached: %+v, entries %d", cut, len(evidence.completions))
	}
	request.Budget = proofs.NewSearchBudget(proofs.SummaryBudget)
	fresh := evidence.Completion(request)
	if !fresh.Proven() || len(evidence.completions) != 1 {
		t.Fatalf("fresh completion = %+v, entries %d", fresh, len(evidence.completions))
	}
	if reused := evidence.Completion(request); reused != fresh {
		t.Fatalf("complete evidence changed on reuse: %+v", reused)
	}
}

func TestLocalCompletionContractPolicyChanges(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type closer struct{}
 func (*closer) Close(){}
 func contract(value *closer){}
 func forwarding(value *closer){contract(value)}
 func caller(value,other *closer){forwarding(value)}
 `)
	fn := pkg.Func("caller")
	root := findSSAInstruction(t, fn, func(instruction ssa.Instruction) bool {
		return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "forwarding"
	})
	// This trusted test contract affects only the exact argument of the visible
	// contract callee. The ordinary body contains no cleanup at all.
	accepts := func(instruction ssa.Instruction, target ssa.Value, method string, invoke bool, condition ssacall.CallCondition) bool {
		common := ssaflow.InstructionCall(instruction)
		return common != nil && common.StaticCallee() == pkg.Func("contract") && len(common.Args) == 1 && common.Args[0] == target &&
			method == "Close" && !invoke && condition.Unconditional()
	}
	rejects := func(ssa.Instruction, ssa.Value, string, bool, ssacall.CallCondition) bool { return false }
	policies := map[string]CompletionSummaryLookup{"none": nil, "accepts": accepts, "rejects": rejects}
	orders := [][]string{
		{"none", "accepts", "none"},
		{"accepts", "none", "accepts"},
		{"accepts", "rejects", "accepts"},
		{"rejects", "accepts", "rejects"},
	}
	for _, order := range orders {
		t.Run(order[0]+"/"+order[1], func(t *testing.T) {
			var evidence LocalEvidence
			for _, name := range order {
				request := CompletionRequest{Instruction: root, Target: fn.Params[0], Methods: []string{"Close"}, CallContract: policies[name]}
				got := evidence.Completion(request)
				if got.Proven() != (name == "accepts") {
					t.Fatalf("policy %s reused another contract's proof: %+v", name, got)
				}
			}
		})
	}
}

// Test-only conveniences over the memoized LocalEvidence proof methods.
// Production callers share one LocalEvidence per analyzer function, so these
// wrappers live with the tests instead of the package API.

func ProveOwnershipTransfer(request OwnershipTransferRequest) proofs.OwnershipTransferProof {
	var evidence LocalEvidence
	return evidence.OwnershipTransfer(request)
}
