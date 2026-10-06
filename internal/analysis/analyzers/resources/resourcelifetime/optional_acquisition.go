package resourcelifetime

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"github.com/kojah/gohawk/internal/reporting/check"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Optional-acquisition evidence connects one guarded acquisition to the
// resource and error phis at its merge. The proof deliberately stops at one
// acyclic diamond and one exact repeated guard; it does not infer arbitrary
// correlations between branch conditions.
type optionalAcquisitionProof struct {
	proof             resourceProof
	resourcePhi       *ssa.Phi
	merge             *ssa.BasicBlock
	acquisitionBlock  *ssa.BasicBlock
	acquiredSuccessor *ssa.BasicBlock
}

func proveOptionalAcquisitionWithin(call *ssa.Call, resource, errorValue ssa.Value, budget *proofs.SearchBudget) optionalAcquisitionProof {
	if !budget.Spend() {
		return optionalAcquisitionProof{proof: resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}}
	}
	proof := findOptionalAcquisitionWithin(call, resource, errorValue, budget)
	// Reachability and phi queries return no match at cutoff. Only the complete
	// finder may supply a correlation or completed decline; discard every field
	// of an interrupted candidate before the flow can bind its resource phi.
	if resourceFlowExhausted(budget) {
		return optionalAcquisitionProof{proof: resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}}
	}
	return proof
}

func findOptionalAcquisitionWithin(call *ssa.Call, resource, errorValue ssa.Value, budget *proofs.SearchBudget) optionalAcquisitionProof {
	unmatched := optionalAcquisitionProof{proof: resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}}
	if call == nil || resource == nil || errorValue == nil || len(call.Block().Succs) != 1 {
		return unmatched
	}
	// Starting flow at the call proves that its guard selected the acquisition
	// arm, but an immediate merge otherwise forgets that fact and invents the
	// inverse no-acquisition path. Recover only a direct diamond whose resource
	// and error phis have one exact acquired edge and nil everywhere else. This
	// is the shape used by CortexDB's optional keyword-search arm:
	// https://github.com/liliang-cn/cortexdb/blob/2486ab7a7d560f5351b626ba813afba5442d1b3d/pkg/core/advanced_search.go#L136-L156
	acquisitionBlock := call.Block()
	merge := acquisitionBlock.Succs[0]
	if len(merge.Preds) != 2 || len(acquisitionBlock.Preds) != 1 || cfg.BlockReachableWithin(merge, acquisitionBlock, budget) {
		return unmatched
	}
	guard := acquisitionBlock.Preds[0]
	if len(guard.Succs) != 2 || !blockHasSuccessor(guard, acquisitionBlock) || !blockHasSuccessor(guard, merge) {
		return unmatched
	}
	resourcePhi := exactOptionalPhiWithin(merge, acquisitionBlock, resource, budget)
	errorPhi := exactOptionalPhiWithin(merge, acquisitionBlock, errorValue, budget)
	if resourcePhi == nil || errorPhi == nil {
		return unmatched
	}
	// The companion error phi ties both call results to the same edge. Requiring
	// the merge to repeat only the exact equality test (or its inverse) avoids
	// turning names, algebraic equivalence, or unrelated branch conditions into
	// resource-presence evidence.
	guardBranch := finalBranch(guard)
	mergeBranch := finalBranch(merge)
	if guardBranch == nil || mergeBranch == nil {
		return unmatched
	}
	guardComparison, guardOK := guardBranch.Cond.(*ssa.BinOp)
	mergeComparison, mergeOK := mergeBranch.Cond.(*ssa.BinOp)
	if !guardOK || !mergeOK || !sameEqualityOperands(guardComparison, mergeComparison) {
		return unmatched
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
		proof: resourceProof{
			State:      proofs.EvidenceProven,
			Reason:     optionalAcquisitionSuccessPhi,
			Provenance: proofs.EvidenceFromLocalSSA,
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

func exactOptionalPhiWithin(merge, acquisitionBlock *ssa.BasicBlock, acquired ssa.Value, budget *proofs.SearchBudget) *ssa.Phi {
	var matched *ssa.Phi
	for _, instruction := range merge.Instrs {
		if !budget.Spend() {
			return nil
		}
		phi, ok := instruction.(*ssa.Phi)
		if !ok {
			break
		}
		if ssaflow.PhiEdgeCount(phi) != len(merge.Preds) {
			continue
		}
		valid := true
		for from, edge := range ssaflow.PhiIncoming(phi) {
			if !budget.Spend() {
				return nil
			}
			if from == acquisitionBlock {
				valid = valid && edge == acquired
			} else {
				valid = valid && ssaflow.DefinitelyNilWithin(edge, budget)
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

func traceOptionalAcquisition(pass *analysis.Pass, proof optionalAcquisitionProof, candidate token.Pos) {
	checkID := string(check.ResourceRelease)
	probe := analysisTrace.For(pass, "resourcelifetime", checkID, candidate)
	if !probe.Enabled() {
		return
	}
	probe.Evidence(analysisTrace.Step{
		Reason:   proof.proof.Reason.String(),
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

// Acquisition-error assertions supply the existing test-contract exclusion.
// Census, ordering and argument provenance share one allowance; partial
// evidence cannot establish that the successful resource path is unavailable.

func proveAcquisitionErrorWithin(acquisition *ssa.Call, resource, errorValue ssa.Value, httpResponse bool, budget *proofs.SearchBudget) resourceProof {
	// Test assertions can prove the owned-resource path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	errorAssertions, nilAssertions := acquisitionErrorAssertionsWithin(acquisition, resource, errorValue, budget)
	// A fatal Error assertion stops the test unless the acquisition failed,
	// which is the same evidence as an `if err != nil { return }` guard for any
	// acquisition. The non-fatal form is accepted only for net/http, whose
	// paired Nil assertion carries the extra fact that a response returned
	// together with an error has an already-closed body.
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, assertedError := range errorAssertions {
		if !budget.Spend() {
			break
		}
		if fatalErrorAssertion(assertedError) || httpResponse && errorAssertionDominatesNilWithin(assertedError, nilAssertions, budget) {
			return carriedValueProof(true, resourceReasonReleaseProven, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

func acquisitionErrorAssertionsWithin(
	acquisition *ssa.Call,
	resource, errorValue ssa.Value,
	budget *proofs.SearchBudget,
) ([]ssa.Instruction, []ssa.Instruction) {
	var errorAssertions, nilAssertions []ssa.Instruction
	for instruction := range ssaflow.InstructionsWithin(acquisition.Parent(), budget) {
		common := ssaflow.InstructionCall(instruction)
		errorClaim := ssacall.HasLibraryContract(common, ssacall.ContractTestifyErrorClaim)
		nilClaim := ssacall.HasLibraryContract(common, ssacall.ContractTestifyNilClaim)
		if !errorClaim && !nilClaim {
			continue
		}
		if !cfg.InstructionMayFollowWithin(acquisition, instruction, budget) {
			continue
		}
		if errorClaim {
			for _, argument := range common.Args {
				if !budget.Spend() {
					return nil, nil
				}
				if heapmodel.ValueDerivesFromWithin(argument, errorValue, budget) {
					errorAssertions = append(errorAssertions, instruction)
				}
			}
		}
		if nilClaim {
			for _, argument := range common.Args {
				if !budget.Spend() {
					return nil, nil
				}
				if budget.Spend() && heapmodel.MayAlias(argument, resource) {
					nilAssertions = append(nilAssertions, instruction)
				}
			}
		}
	}
	// An interrupted census publishes neither assertion list. Alias dispatch is
	// charged here; graph construction and alias-query internals are independent.
	if resourceFlowExhausted(budget) {
		return nil, nil
	}
	return errorAssertions, nilAssertions
}

func errorAssertionDominatesNilWithin(assertedError ssa.Instruction, nilAssertions []ssa.Instruction, budget *proofs.SearchBudget) bool {
	for _, assertedNil := range nilAssertions {
		if cfg.InstructionDominatesWithin(assertedError, assertedNil, budget) {
			return true
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return ssacall.HasLibraryContract(common, ssacall.ContractTestifyFatalError)
}

// Context cancellation pairs exact standard constructor results with their
// cancel invocation. Pre-acquisition DB failure and later transaction cleanup
// uncertainty share pairing mechanics but retain different lifecycle meanings.

// These DB methods obtain a connection through DB.conn, which checks ctx.Done
// before giving the driver any work. Conn, Tx, and Stmt methods do not share
// that entry check. Only an ordinary invocation of the exact paired cancel
// dominating acquisition counts; deadline expiry, deferred calls, and sleeps
// do not establish cancellation before acquisition.
// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L2794-L2803
func proveAcquisitionContextCanceledWithin(acquisition *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	common := acquisition.Common()
	if !sqlDatabaseCall(common, "PrepareContext", "QueryContext", "BeginTx") || len(common.Args) < 2 {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	constructor := contextCancelConstructor(common.Args[1])
	if constructor == nil {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	for instruction := range ssaflow.InstructionsWithin(acquisition.Parent(), budget) {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			continue
		}
		if invokesContextCancel(call.Common(), constructor) && cfg.InstructionDominates(call, acquisition) {
			return carriedValueProof(true, resourceReasonCanceledAcquisition, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// BeginTx binds the transaction lifetime to its context. Cancellation triggers
// database/sql's rollback watcher; it does not prove synchronous rollback or
// successful commit. Only the exact factory's paired cancel is recognized.
// https://pkg.go.dev/database/sql#DB.BeginTx
// https://github.com/OdyseeTeam/odysee-api/blob/6cb1fd36ef7d25a038e3ddf572e3ddb3bbbb3d79/apps/watchman/olapdb/olapdb.go#L112-L139
func cancelsTransactionContext(acquisition *ssa.Call, instruction ssa.Instruction) bool {
	common := acquisition.Common()
	if !sqlReceiverCall(common, "DB", "BeginTx") && !sqlReceiverCall(common, "Conn", "BeginTx") || len(common.Args) < 2 {
		return false
	}
	switch instruction.(type) {
	case *ssa.Call, *ssa.Defer:
		return invokesContextCancel(ssaflow.InstructionCall(instruction), contextCancelConstructor(common.Args[1]))
	}
	return false
}

// Pairing stays structural: context result zero and cancel result one from one
// standard constructor. A deadline alone, a replaced context, or another
// factory's cancel supplies no cancellation witness.
func contextCancelConstructor(value ssa.Value) *ssa.Call {
	ctx, ok := value.(*ssa.Extract)
	if !ok || ctx.Index != 0 {
		return nil
	}
	constructor, ok := ctx.Tuple.(*ssa.Call)
	if !ok || !ssacall.CallMatchesAnySymbol(constructor.Common(),
		syntax.PackageFunction("context", "WithCancel"),
		syntax.PackageFunction("context", "WithCancelCause"),
		syntax.PackageFunction("context", "WithDeadline"),
		syntax.PackageFunction("context", "WithDeadlineCause"),
		syntax.PackageFunction("context", "WithTimeout"),
		syntax.PackageFunction("context", "WithTimeoutCause")) {
		return nil
	}
	return constructor
}

func invokesContextCancel(common *ssa.CallCommon, constructor *ssa.Call) bool {
	if common == nil || constructor == nil {
		return false
	}
	cancel, ok := common.Value.(*ssa.Extract)
	return ok && cancel.Tuple == constructor && cancel.Index == 1
}
