package resourcelifetime

import (
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Acquisition-error guards establish when the exact error paired with a
// resource excludes ownership on one successor. Visible predicates and captured
// callbacks use bounded shared flow and storage evidence; opaque or mutable
// dispatch cannot establish this implication.

func resourceSuccessBranch(
	pass *analysis.Pass,
	knowledge *summaries.Provider,
	block, successor *ssa.BasicBlock,
	errorValue ssa.Value,
	candidate token.Pos,
) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
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
	if proof, ok := resourceAbsentErrorCheck(knowledge, branch.Cond, errorValue); ok && successor == block.Succs[0] {
		traceAcquisitionErrorProof(pass, branch, proof, candidate)
		return false, true
	}
	if success, ok := testifyNoErrorSuccessBranch(branch, successor, errorValue); ok {
		if success {
			traceAcquisitionErrorProof(pass, branch, "testify-no-error-guard", candidate)
		}
		return success, true
	}
	return ssaflow.SuccessBranch(block, successor, errorValue)
}

func testifyNoErrorSuccessBranch(branch *ssa.If, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	call, ok := branch.Cond.(*ssa.Call)
	if !ok || !ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestifyNoError) || len(call.Common().Args) < 2 ||
		!ssainfer.MayAlias(call.Common().Args[1], errorValue) {
		return false, false
	}
	// Testify's exact boolean contract is true precisely when the supplied
	// error is nil. Go's SSA canonicalizes both `if NoError` and
	// `if !NoError { return }` so the true successor is the success path. Kong
	// uses both forms around HTTP response cleanup:
	// https://github.com/Kong/kong-operator/blob/1adc910f31b5a6bf65d20dbf0698c85f3dbb87b1/ingress-controller/test/helpers/http.go#L173-L177
	return successor == branch.Block().Succs[0], true
}

func resourceAbsentErrorCheck(knowledge *summaries.Provider, condition, errorValue ssa.Value) (string, bool) {
	// Equality to a documented non-nil sentinel excludes successful acquisition.
	// Require the exact error: an unrelated or derived error can compare equal
	// even when this acquisition succeeded. Arbitrary error variables may be nil.
	// https://github.com/life4/enc/blob/853bf70379f698b65e4415876c014378be90e021/cmd/helpers.go#L29-L38
	if comparison, ok := condition.(*ssa.BinOp); ok && comparison.Op == token.EQL {
		if comparison.X == errorValue && isNonNilFilesystemSentinel(comparison.Y) ||
			comparison.Y == errorValue && isNonNilFilesystemSentinel(comparison.X) {
			return "exact-error-equals-non-nil-filesystem-sentinel", true
		}
	}
	if errorTypeAssertionSucceeded(condition, errorValue) {
		return "error-type-assertion-succeeded", true
	}
	if errorsIsNonNilFilesystemSentinel(condition, errorValue) {
		return "errors-is-non-nil-filesystem-sentinel", true
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return "", false
	}
	common := call.Common()
	// errors.As returns false for nil, including custom As implementations:
	// they are called only after that nil check. Require the exact acquisition
	// error; a joined or wrapped error can be non-nil even when acquisition succeeds.
	// https://github.com/bluesky-social/indigo/blob/41278964ec8e3253e70d4e919dfb8e34211c543d/atproto/identity/did.go#L108-L121
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("errors", "As")) &&
		len(common.Args) == 2 && common.Args[0] == errorValue {
		return "errors-as-exact-acquisition-error", true
	}
	if proof := errorPredicateAcquisition(knowledge, call, errorValue); proof.Proven() {
		return string(proof.Reason), true
	}
	// os.IsNotExist and os.IsExist are the legacy equivalents of errors.Is with
	// the corresponding filesystem sentinel. Their true branches prove that
	// the acquisition returned a non-nil error and no owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	if len(common.Args) != 1 || !ssainfer.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return "", false
	}
	// os.IsPermission and os.IsTimeout are documented to report false for a
	// nil error, so their true branches carry the same proof.
	for _, predicate := range []string{"IsNotExist", "IsExist", "IsPermission", "IsTimeout"} {
		if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", predicate)) {
			return "os-" + strings.ToLower(predicate), true
		}
	}
	return "", false
}

// A predicate may observe the error without changing its nil meaning. The
// implication needed here, that the result is false whenever the exact error
// is nil, is the FalseWhenParameterNil relation the result summary proves for
// visible bodies and imports for other packages, so a local helper, a
// captured callback, and an exported helper all answer through one proof.
// Unknown other branches, rewritten errors, dynamic dispatch and deferred
// result mutation leave the relation unproven.
// https://github.com/norwoodj/helm-docs/blob/a5573af096a4b526dcbc3c896c220b1714a0765b/pkg/helm/chart_info.go#L94-L106
func errorPredicateAcquisition(knowledge *summaries.Provider, call *ssa.Call, errorValue ssa.Value) ssaflow.Proof {
	unknown := ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	function, closure := ssaflow.DirectCallee(call.Common())
	if function == nil {
		function = capturedErrorPredicate(call, budget)
	}
	if function == nil || knowledge == nil || errorValue == nil {
		if budget.Exhausted() {
			unknown.Reason = ssaflow.EvidenceBudgetExhausted
		}
		return unknown
	}
	// Relations index parameters by position, receiver included, which is the
	// argument position of a static call. An imported callee has no SSA
	// parameters to bind, so the position is matched directly.
	for index, argument := range call.Common().Args {
		if argument != errorValue || closure != nil {
			continue
		}
		summary, available := knowledge.ForFunction(function).Results(ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if available == summaries.Available && summary.Holds(resultfacts.FalseWhenParameterNil, 0, index) {
			return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "error-predicate-false-for-nil"}
		}
	}
	if budget.Exhausted() {
		unknown.Reason = ssaflow.EvidenceBudgetExhausted
	}
	return unknown
}

// Captured dispatch requires immutable lexical cells at every capture layer.
// Every construction must lead to the same visible function; later replacement,
// opaque cell escape, circular evidence and exhaustion remain unknown. The
// reaching fold and all storage/effect queries share one work budget.
// https://github.com/antonmedv/gitmal/blob/83be9ddcf82e8a90ea50a9d54c1ebfc3e22ace16/blob.go#L69-L110
func capturedErrorPredicate(call *ssa.Call, budget *ssaflow.SearchBudget) *ssa.Function {
	load, ok := call.Common().Value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	query := capturedPredicateQuery{budget: budget}
	if !ssaflow.NewReachingWalk(ssaflow.TransparentNone).Every(load.X, query.cell) {
		return nil
	}
	return query.result
}

type capturedPredicateQuery struct {
	budget *ssaflow.SearchBudget
	result *ssa.Function
}

func (query *capturedPredicateQuery) cell(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	free, ok := value.(*ssa.FreeVar)
	if !ok || free.Parent().Parent() == nil || !ssaflow.NewCallEffects(query.budget).Value(free).PreservesStorage() {
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
		stored := ssainfer.NewStorage(query.budget).StableContent(binding.Binding, creation)
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

func errorTypeAssertionSucceeded(condition, errorValue ssa.Value) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	return ok && assertion.CommaOk && ssainfer.ValueDerivesFrom(assertion.X, errorValue, map[ssa.Value]bool{})
}

func errorsIsNonNilFilesystemSentinel(condition, errorValue ssa.Value) bool {
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if !ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("errors", "Is")) || len(common.Args) != 2 {
		return false
	}
	if !ssainfer.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return false
	}
	return isNonNilFilesystemSentinel(common.Args[1])
}

func isNonNilFilesystemSentinel(value ssa.Value) bool {
	for {
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
				return false
			}
			value = typed.X
		case *ssa.Global:
			return ssaflow.ValueMatchesAnySymbol(
				typed,
				syntax.PackageVariable("os", "ErrNotExist"),
				syntax.PackageVariable("os", "ErrExist"),
				syntax.PackageVariable("io/fs", "ErrNotExist"),
				syntax.PackageVariable("io/fs", "ErrExist"),
			)
		default:
			return false
		}
	}
}

func traceAcquisitionErrorProof(pass *analysis.Pass, branch *ssa.If, proof string, candidate token.Pos) {
	analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), candidate).Evidence(analysisTrace.Step{
		Reason:   "acquisition-error-proven",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      branch.Cond.Pos(),
		Function: branch.Parent().String(),
		Details:  map[string]string{"proof": proof},
	})
}
