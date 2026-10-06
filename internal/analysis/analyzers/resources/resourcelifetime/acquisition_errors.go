package resourcelifetime

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analysis/summaries"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"github.com/kojah/gohawk/internal/reporting/check"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Acquisition-error guards establish when the exact error paired with a
// resource excludes ownership on one successor. Visible predicates and captured
// callbacks use bounded shared flow and storage evidence; opaque or mutable
// dispatch cannot establish this implication.

type acquisitionErrorResultProof struct {
	proof resourceProof
	value ssa.Value
}

func proveAcquisitionErrorResultWithin(call *ssa.Call, budget *proofs.SearchBudget) acquisitionErrorResultProof {
	results, ok := call.Type().(*types.Tuple)
	if !ok || results.Len() < 2 {
		return acquisitionErrorResultProof{proof: resourceProof{State: proofs.EvidenceDisproven}}
	}
	last := results.Len() - 1
	if !types.Identical(results.At(last).Type(), types.Universe.Lookup("error").Type()) {
		return acquisitionErrorResultProof{proof: resourceProof{State: proofs.EvidenceDisproven}}
	}
	// The success guard must track the error paired with the acquisition, not
	// assume it occupies slot one. termios.Pty returns (master, slave, err):
	// https://github.com/89luca89/lilipod/blob/872755a7cef33c238ea2d11b2310b3116944eb48/ptyagent/pty.go#L134-L146
	value := ssacall.CallResultWithin(call, last, budget)
	proof := carriedValueProof(value != nil, resourceReasonNone, budget)
	// An unavailable error lookup cannot activate ownership on the failed
	// acquisition edge. Only a completed search may supply an absent extract.
	if proof.State == proofs.EvidenceUnknown {
		value = nil
	}
	return acquisitionErrorResultProof{proof: proof, value: value}
}

type resourceBranchProof struct {
	resourceProof
	success bool
}

// The branch proof owns cutoff availability. A missing error guard after an
// interrupted query must not activate ownership or admit a partial edge list.
func proveResourceSuccessBranch(
	pass *analysis.Pass, knowledge *summaries.Provider, block, successor *ssa.BasicBlock,
	errorValue ssa.Value, candidate token.Pos, budget *proofs.SearchBudget,
) resourceBranchProof {
	if !budget.Spend() {
		return resourceBranchProof{resourceProof: carriedValueProof(false, resourceReasonNone, budget)}
	}
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return resourceBranchProof{resourceProof: carriedValueProof(false, resourceReasonNone, budget)}
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return resourceBranchProof{resourceProof: carriedValueProof(false, resourceReasonNone, budget)}
	}
	// A true check against a documented non-nil filesystem error proves that
	// the acquisition failed and produced no owned file. Callers commonly
	// inspect a specific error before the generic err != nil check. This covers
	// both skipped missing inputs and create-if-absent races without treating an
	// arbitrary package variable as non-nil.
	// https://github.com/heymaikol/network-doctor/blob/6d0df6eaba1de237077e0a1f8224fd8d5c3d083a/internal/simulation/evidence.go#L407-L415
	// https://github.com/codefly-dev/cli/blob/5d176b95c8e3ad721bdeb0d6c4c3a64dd261caa6/pkg/executionattestor/file.go#L122-L130
	// https://github.com/prometheus/node_exporter/blob/a4e08d1d9a152f67ef781469eade6b0bf431994d/collector/ethtool_linux_test.go#L62-L74
	// https://github.com/pocketbase/pocketbase/blob/bc8ffed4e7265a70a6e8de76c0b0b48b945e19ef/tools/filesystem/internal/fileblob/fileblob.go#L428-L436
	proof := resourceAbsentErrorCheck(knowledge, branch.Cond, errorValue, budget)
	if proof.State == proofs.EvidenceUnknown {
		return resourceBranchProof{resourceProof: proof}
	}
	if proof.Proven() && successor == block.Succs[0] {
		traceAcquisitionErrorProof(pass, branch, proof.Reason, candidate)
		return resourceBranchProof{resourceProof: proof}
	}
	if success, ok := testifyNoErrorSuccessBranch(branch, successor, errorValue, budget); ok && !resourceFlowExhausted(budget) {
		if success {
			traceAcquisitionErrorProof(pass, branch, resourceReasonTestifyNoErrorGuard, candidate)
		}
		return resourceBranchProof{resourceProof: carriedValueProof(true, resourceReasonTestifyNoErrorGuard, budget), success: success}
	}
	success, known := ssapath.SuccessBranchWithin(block, successor, errorValue, budget)
	proof = carriedValueProof(known, resourceReasonNone, budget)
	return resourceBranchProof{resourceProof: proof, success: proof.Proven() && success}
}

func testifyNoErrorSuccessBranch(branch *ssa.If, successor *ssa.BasicBlock, errorValue ssa.Value, budget *proofs.SearchBudget) (bool, bool) {
	call, ok := branch.Cond.(*ssa.Call)
	if !ok || !ssacall.HasLibraryContract(call.Common(), ssacall.ContractTestifyNoError) || len(call.Common().Args) < 2 ||
		!heapmodel.MayAliasAnyWithin(call.Common().Args[1], []ssa.Value{errorValue}, budget) {
		return false, false
	}
	// Testify's exact boolean contract is true precisely when the supplied
	// error is nil. Go's SSA canonicalizes both `if NoError` and
	// `if !NoError { return }` so the true successor is the success path. Kong
	// uses both forms around HTTP response cleanup:
	// https://github.com/Kong/kong-operator/blob/1adc910f31b5a6bf65d20dbf0698c85f3dbb87b1/ingress-controller/test/helpers/http.go#L173-L177
	return successor == branch.Block().Succs[0], true
}

func resourceAbsentErrorCheck(
	knowledge *summaries.Provider, condition, errorValue ssa.Value, budget *proofs.SearchBudget,
) resourceProof {
	// Equality to a documented non-nil sentinel excludes successful acquisition.
	// Require the exact error: an unrelated or derived error can compare equal
	// even when this acquisition succeeded. Arbitrary error variables may be nil.
	// https://github.com/life4/enc/blob/853bf70379f698b65e4415876c014378be90e021/cmd/helpers.go#L29-L38
	if comparison, ok := condition.(*ssa.BinOp); ok && comparison.Op == token.EQL {
		if comparison.X == errorValue && isNonNilFilesystemSentinel(comparison.Y, budget) ||
			comparison.Y == errorValue && isNonNilFilesystemSentinel(comparison.X, budget) {
			return carriedValueProof(true, resourceReasonExactErrorEqualsNonNilFilesystemSentinel, budget)
		}
	}
	if errorTypeAssertionSucceeded(condition, errorValue, budget) {
		return carriedValueProof(true, resourceReasonErrorTypeAssertionSucceeded, budget)
	}
	if reason, proven := errorsIsNonNilSentinel(condition, errorValue, budget); proven {
		return carriedValueProof(true, reason, budget)
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return carriedValueProof(false, resourceReasonNone, budget)
	}
	common := call.Common()
	// errors.As returns false for nil, including custom As implementations:
	// they are called only after that nil check. Require the exact acquisition
	// error; a joined or wrapped error can be non-nil even when acquisition succeeds.
	// https://github.com/bluesky-social/indigo/blob/41278964ec8e3253e70d4e919dfb8e34211c543d/atproto/identity/did.go#L108-L121
	if ssacall.CallMatchesSymbol(common, syntax.PackageFunction("errors", "As")) &&
		len(common.Args) == 2 && common.Args[0] == errorValue {
		return carriedValueProof(true, resourceReasonErrorsAsExactAcquisitionError, budget)
	}
	if proof := errorPredicateAcquisition(knowledge, call, errorValue, budget); proof.Proven() || proof.Reason == resourceReasonBudgetExhausted {
		return proof
	}
	// os.IsNotExist and os.IsExist are the legacy equivalents of errors.Is with
	// the corresponding filesystem sentinel. Their true branches prove that
	// the acquisition returned a non-nil error and no owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	if len(common.Args) != 1 || !heapmodel.ValueDerivesFromWithin(common.Args[0], errorValue, budget) {
		return carriedValueProof(false, resourceReasonNone, budget)
	}
	// os.IsPermission and os.IsTimeout are documented to report false for a
	// nil error, so their true branches carry the same proof.
	for _, predicate := range []struct {
		symbol syntax.Symbol
		reason resourceLifetimeReason
	}{
		{syntax.PackageFunction("os", "IsNotExist"), resourceReasonOSIsNotExist},
		{syntax.PackageFunction("os", "IsExist"), resourceReasonOSIsExist},
		{syntax.PackageFunction("os", "IsPermission"), resourceReasonOSIsPermission},
		{syntax.PackageFunction("os", "IsTimeout"), resourceReasonOSIsTimeout},
	} {
		if ssacall.CallMatchesSymbol(common, predicate.symbol) {
			return carriedValueProof(true, predicate.reason, budget)
		}
	}
	return carriedValueProof(false, resourceReasonNone, budget)
}

// A predicate may observe the error without changing its nil meaning. The
// implication needed here, that the result is false whenever the exact error
// is nil, is the parameter-nil result case the result summary proves for
// visible bodies and imports for other packages, so a local helper, a
// captured callback, and an exported helper all answer through one proof.
// Unknown other branches, rewritten errors, dynamic dispatch and deferred
// result mutation leave the relation unproven.
// https://github.com/norwoodj/helm-docs/blob/a5573af096a4b526dcbc3c896c220b1714a0765b/pkg/helm/chart_info.go#L94-L106
func errorPredicateAcquisition(knowledge *summaries.Provider, call *ssa.Call, errorValue ssa.Value, budget *proofs.SearchBudget) resourceProof {
	unknown := resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonEvidenceUnavailable}
	if !budget.Spend() {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	function, closure := ssacall.DirectCallee(call.Common())
	if function == nil {
		captureBudget := budget.Within(proofs.QueryBudget)
		function = capturedErrorPredicate(call, captureBudget)
		if resourceFlowExhausted(captureBudget) {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
	}
	if function == nil || knowledge == nil || errorValue == nil {
		if budget.Exhausted() {
			unknown.Reason = resourceReasonBudgetExhausted
		}
		return unknown
	}
	// Result cases index parameters by position, receiver included, which is the
	// argument position of a static call. An imported callee has no SSA
	// parameters to bind, so the position is matched directly.
	for index, argument := range call.Common().Args {
		if !budget.Spend() {
			break
		}
		if argument != errorValue || closure != nil {
			continue
		}
		summaryBudget := budget.Within(proofs.SummaryBudget)
		summary, available := knowledge.ForFunction(function).Results(summaryBudget)
		if resourceFlowExhausted(summaryBudget) {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if available == summaries.Available && !resourceFlowExhausted(budget) && summary.Implies(ssacall.ParameterNil(index), 0, ssacall.OutcomeFalse) {
			return resourceProof{State: proofs.EvidenceProven, Reason: resourceReasonErrorPredicateFalseForNil}
		}
	}
	if budget.Exhausted() {
		unknown.Reason = resourceReasonBudgetExhausted
	}
	return unknown
}

// Captured dispatch requires immutable lexical cells at every capture layer.
// Every construction must lead to the same visible function; later replacement,
// opaque cell escape, circular evidence and exhaustion remain unknown. The
// reaching fold and all storage/effect queries share one work budget.
// https://github.com/antonmedv/gitmal/blob/83be9ddcf82e8a90ea50a9d54c1ebfc3e22ace16/blob.go#L69-L110
func capturedErrorPredicate(call *ssa.Call, budget *proofs.SearchBudget) *ssa.Function {
	load, ok := call.Common().Value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	query := capturedPredicateQuery{budget: budget}
	if !ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget).Every(load.X, query.cell) {
		return nil
	}
	return query.result
}

type capturedPredicateQuery struct {
	budget *proofs.SearchBudget
	result *ssa.Function
}

func (query *capturedPredicateQuery) cell(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	free, ok := value.(*ssa.FreeVar)
	if !ok || free.Parent().Parent() == nil || !ssacall.NewCallEffects(query.budget).Value(free).PreservesStorage() {
		return false
	}
	var creations []ssa.Value
	for _, block := range free.Parent().Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !query.budget.Spend() {
				return false
			}
			creation, ok := instruction.(*ssa.MakeClosure)
			if ok && creation.Fn == free.Parent() {
				creations = append(creations, creation)
			}
		}
	}
	return walk.EveryOf(creations, func(branch ssaflow.ReachingWalk, value ssa.Value) bool {
		creation, ok := value.(*ssa.MakeClosure)
		return ok && query.creation(branch, free, creation)
	})
}

func (query *capturedPredicateQuery) creation(walk ssaflow.ReachingWalk, free *ssa.FreeVar, creation *ssa.MakeClosure) bool {
	for _, binding := range ssaflow.ClosureBindingPairs(free.Parent(), creation) {
		if !query.budget.Spend() {
			return false
		}
		if binding.Free != free {
			continue
		}
		if _, captured := binding.Binding.(*ssa.FreeVar); captured {
			return walk.Every(binding.Binding, query.cell)
		}
		stored := heapmodel.NewStorage(query.budget).StableContent(binding.Binding, creation)
		if !stored.Proven() {
			return false
		}
		value := stored.Value
		if closure, ok := value.(*ssa.MakeClosure); ok {
			value = closure.Fn
		}
		function, _ := value.(*ssa.Function)
		if function == nil || query.result != nil && query.result != function {
			return false
		}
		query.result = function
		return true
	}
	return false
}

func errorTypeAssertionSucceeded(condition, errorValue ssa.Value, budget *proofs.SearchBudget) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	return ok && assertion.CommaOk && heapmodel.ValueDerivesFromWithin(assertion.X, errorValue, budget)
}

// errors.Is(nil, target) is false for a documented non-nil target. Require
// the acquisition's exact error: errors.Join may include that error yet match
// another member when the acquisition succeeded. cute handles its HTTP timeout
// before the general error check:
// https://github.com/ozontech/cute/blob/9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3/roundtripper.go#L76-L91
func errorsIsNonNilSentinel(condition, errorValue ssa.Value, budget *proofs.SearchBudget) (resourceLifetimeReason, bool) {
	call, ok := condition.(*ssa.Call)
	if !ok {
		return resourceReasonNone, false
	}
	common := call.Common()
	if !ssacall.CallMatchesSymbol(common, syntax.PackageFunction("errors", "Is")) || len(common.Args) != 2 || common.Args[0] != errorValue {
		return resourceReasonNone, false
	}
	reason := nonNilErrorSentinelReason(common.Args[1], budget)
	return reason, reason != resourceReasonNone
}

func isNonNilFilesystemSentinel(value ssa.Value, budget *proofs.SearchBudget) bool {
	return nonNilErrorSentinelReason(value, budget) == resourceReasonErrorsIsNonNilFilesystemSentinel
}

// nonNilErrorSentinelReason recognizes documented standard-library sentinel
// contracts. A custom error variable, its initializer, and merged targets do
// not establish non-nilness. Keep the filesystem trace reason stable while
// giving the context contract its own reason.
func nonNilErrorSentinelReason(value ssa.Value, budget *proofs.SearchBudget) resourceLifetimeReason {
	for budget.Spend() {
		if inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			value = inner
			continue
		}
		switch typed := value.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return resourceReasonNone
			}
			value = typed.X
		case *ssa.Global:
			if ssacall.ValueMatchesAnySymbol(
				typed,
				syntax.PackageVariable("os", "ErrNotExist"),
				syntax.PackageVariable("os", "ErrExist"),
				syntax.PackageVariable("io/fs", "ErrNotExist"),
				syntax.PackageVariable("io/fs", "ErrExist"),
			) {
				return resourceReasonErrorsIsNonNilFilesystemSentinel
			}
			if ssacall.ValueMatchesAnySymbol(typed,
				syntax.PackageVariable("context", "Canceled"), syntax.PackageVariable("context", "DeadlineExceeded"),
			) {
				return resourceReasonErrorsIsNonNilContextSentinel
			}
			return resourceReasonNone
		default:
			return resourceReasonNone
		}
	}
	return resourceReasonNone
}

func traceAcquisitionErrorProof(pass *analysis.Pass, branch *ssa.If, proof resourceLifetimeReason, candidate token.Pos) {
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), candidate)
	if !probe.Enabled() {
		return
	}
	probe.Evidence(analysisTrace.Step{
		Reason:   resourceReasonAcquisitionErrorProven.String(),
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      branch.Cond.Pos(),
		Function: branch.Parent().String(),
		Details:  map[string]string{"proof": proof.String()},
	})
}
