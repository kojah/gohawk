package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

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
