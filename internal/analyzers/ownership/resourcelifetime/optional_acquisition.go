package resourcelifetime

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

const optionalAcquisitionSuccessPhi ssaflow.EvidenceReason = "optional-acquisition-success-phi"

// Optional-acquisition evidence connects one guarded acquisition to the
// resource and error phis at its merge. The proof deliberately stops at one
// acyclic diamond and one exact repeated guard; it does not infer arbitrary
// correlations between branch conditions.
type optionalAcquisitionProof struct {
	proof             ssaflow.Proof
	resourcePhi       *ssa.Phi
	merge             *ssa.BasicBlock
	acquisitionBlock  *ssa.BasicBlock
	acquiredSuccessor *ssa.BasicBlock
}

func proveOptionalAcquisition(call *ssa.Call, resource, errorValue ssa.Value) optionalAcquisitionProof {
	if call == nil || resource == nil || errorValue == nil || len(call.Block().Succs) != 1 {
		return optionalAcquisitionProof{}
	}
	// Starting flow at the call proves that its guard selected the acquisition
	// arm, but an immediate merge otherwise forgets that fact and invents the
	// inverse no-acquisition path. Recover only a direct diamond whose resource
	// and error phis have one exact acquired edge and nil everywhere else. This
	// is the shape used by CortexDB's optional keyword-search arm:
	// https://github.com/liliang-cn/cortexdb/blob/2486ab7a7d560f5351b626ba813afba5442d1b3d/pkg/core/advanced_search.go#L136-L156
	acquisitionBlock := call.Block()
	merge := acquisitionBlock.Succs[0]
	if len(merge.Preds) != 2 || len(acquisitionBlock.Preds) != 1 || ssaflow.BlockReachable(merge, acquisitionBlock) {
		return optionalAcquisitionProof{}
	}
	guard := acquisitionBlock.Preds[0]
	if len(guard.Succs) != 2 || !blockHasSuccessor(guard, acquisitionBlock) || !blockHasSuccessor(guard, merge) {
		return optionalAcquisitionProof{}
	}
	resourcePhi := exactOptionalPhi(merge, acquisitionBlock, resource)
	errorPhi := exactOptionalPhi(merge, acquisitionBlock, errorValue)
	if resourcePhi == nil || errorPhi == nil {
		return optionalAcquisitionProof{}
	}
	// The companion error phi ties both call results to the same edge. Requiring
	// the merge to repeat only the exact equality test (or its inverse) avoids
	// turning names, algebraic equivalence, or unrelated branch conditions into
	// resource-presence evidence.
	guardBranch := finalBranch(guard)
	mergeBranch := finalBranch(merge)
	if guardBranch == nil || mergeBranch == nil {
		return optionalAcquisitionProof{}
	}
	guardComparison, guardOK := guardBranch.Cond.(*ssa.BinOp)
	mergeComparison, mergeOK := mergeBranch.Cond.(*ssa.BinOp)
	if !guardOK || !mergeOK || !sameEqualityOperands(guardComparison, mergeComparison) {
		return optionalAcquisitionProof{}
	}
	guardTrueAtAcquisition := guard.Succs[0] == acquisitionBlock
	mergeTrueAtAcquisition := guardTrueAtAcquisition
	if guardComparison.Op != mergeComparison.Op {
		mergeTrueAtAcquisition = !mergeTrueAtAcquisition
	}
	acquiredSuccessor := merge.Succs[1]
	if mergeTrueAtAcquisition {
		acquiredSuccessor = merge.Succs[0]
	}
	return optionalAcquisitionProof{
		proof: ssaflow.Proof{
			State:      ssaflow.EvidenceProven,
			Reason:     optionalAcquisitionSuccessPhi,
			Provenance: ssaflow.EvidenceFromLocalSSA,
		},
		resourcePhi:       resourcePhi,
		merge:             merge,
		acquisitionBlock:  acquisitionBlock,
		acquiredSuccessor: acquiredSuccessor,
	}
}

func (proof optionalAcquisitionProof) Proven() bool {
	return proof.proof.Proven()
}

func exactOptionalPhi(merge, acquisitionBlock *ssa.BasicBlock, acquired ssa.Value) *ssa.Phi {
	var matched *ssa.Phi
	for _, instruction := range merge.Instrs {
		phi, ok := instruction.(*ssa.Phi)
		if !ok {
			break
		}
		if ssaflow.PhiEdgeCount(phi) != len(merge.Preds) {
			continue
		}
		valid := true
		for from, edge := range ssaflow.PhiIncoming(phi) {
			if from == acquisitionBlock {
				valid = valid && edge == acquired
			} else {
				valid = valid && ssaflow.DefinitelyNil(edge)
			}
		}
		if valid {
			if matched != nil {
				return nil
			}
			matched = phi
		}
	}
	return matched
}

func blockHasSuccessor(block, successor *ssa.BasicBlock) bool {
	return slices.Contains(block.Succs, successor)
}

func finalBranch(block *ssa.BasicBlock) *ssa.If {
	if block == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return nil
	}
	branch, _ := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	return branch
}

func sameEqualityOperands(left, right *ssa.BinOp) bool {
	if !equalityOperator(left.Op) || !equalityOperator(right.Op) {
		return false
	}
	return sameExactOperand(left.X, right.X) && sameExactOperand(left.Y, right.Y) ||
		sameExactOperand(left.X, right.Y) && sameExactOperand(left.Y, right.X)
}

func equalityOperator(operator token.Token) bool {
	return operator == token.EQL || operator == token.NEQ
}

func sameExactOperand(left, right ssa.Value) bool {
	if left == right {
		return true
	}
	leftConstant, leftOK := left.(*ssa.Const)
	rightConstant, rightOK := right.(*ssa.Const)
	return leftOK && rightOK && leftConstant.Value != nil && rightConstant.Value != nil &&
		types.Identical(leftConstant.Type(), rightConstant.Type()) && constant.Compare(leftConstant.Value, token.EQL, rightConstant.Value)
}

func traceOptionalAcquisition(pass *analysis.Pass, proof optionalAcquisitionProof) {
	checkID := string(check.ResourceRelease)
	analysisTrace.EmitIfEnabled(pass, analysisTrace.Event{
		Analyzer: "resourcelifetime",
		Check:    checkID,
		Phase:    "evidence",
		Reason:   string(proof.proof.Reason),
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      proof.resourcePhi.Pos(),
		Function: proof.resourcePhi.Parent().String(),
	})
}

func optionalAcquisitionReleases(instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || !slices.Contains(methods, ssaflow.CallName(common)) {
		return false
	}
	receiver := ssaflow.CallReceiver(common)
	for receiver != resource {
		unwrapped, ok := ssaflow.UnwrapTransparentValue(
			receiver,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		)
		if !ok {
			return false
		}
		receiver = unwrapped
	}
	return true
}
