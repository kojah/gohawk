// Package lockorder implements the lockorder gohawk analyzer.
package lockorder

import (
	"go/token"
	"reflect"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/summaries"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type lockRelation struct {
	from string
	to   string
}

type lockFlowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	held        []string
	origins     map[string]lockAcquisition
	// readHeld is the subset of held taken with RLock. A read lock grants
	// read access only, so a write while one is held is a race with any other
	// reader; tracking the mode separately keeps the rest of the walk, which
	// does not care how a lock was taken, unchanged.
	readHeld       []string
	deferred       []string
	guards         map[string]lockGuard
	condition      string
	conditionValue bool
	constants      []lockScalarConstant
	constraints    ssapath.PathGuards
}

type lockScalarConstant struct {
	value   *ssa.Phi
	literal *ssa.Const
}

type lockGuard struct {
	condition string
	value     bool
}

type mutexOperation uint8

const (
	mutexAcquire mutexOperation = iota + 1
	mutexRelease
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Concurrency: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:       "lockorder",
		Doc:        "checks contradictory mutex acquisition order and unreleased return paths",
		Requires:   summaryKnowledge.Requires(),
		Run:        runLockOrder,
		ResultType: reflect.TypeFor[*Graph](),
	}
}

func runLockOrder(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	relations := newLockOrders()
	// One search serves every function: a helper reached from many call sites
	// is summarized once rather than once per site.
	calleeLocks := newCalleeLockSearch()
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	packageFunctions := ssaflow.PackageFunctions(pass)
	inventory := collectLockCallers(ssaResult.Pkg.Func("init"), packageFunctions, proofs.NewSearchBudget(callerSetBudget))
	callers := inventory
	exclusive := newExclusiveCallers(pass, inventory)
	fields := collectReadLockFieldEvidence(functions, proofs.NewSearchBudget(lockStateWorkBudget))
	for _, function := range functions {
		var evidence lifecycle.LocalEvidence
		walkLockOrder(pass, function, relations, calleeLocks, &evidence, callers, exclusive, fields)
	}
	return relations.graph(pass), nil
}

func walkLockOrder(
	pass *analysis.Pass,
	function *ssa.Function,
	relations *lockOrders,
	calleeLocks *calleeLockSearch,
	evidence *lifecycle.LocalEvidence,
	callers map[*ssa.Function]conditionalCallerSet,
	exclusive *exclusiveCallers, fields readLockFieldEvidence,
) {
	probe := analysisTrace.For(pass, "lockorder", string(check.LockMissingRelease), function.Pos())
	walk := lockStateWalk{budget: proofs.NewSearchBudget(lockStateWorkBudget).Observed(probe.Observer()), fieldEvidence: fields}
	walk.analyze(pass, function, relations, calleeLocks, evidence, callers, exclusive)
}

func (walk *lockStateWalk) analyze(
	pass *analysis.Pass, function *ssa.Function, relations *lockOrders,
	calleeLocks *calleeLockSearch, evidence *lifecycle.LocalEvidence,
	callers map[*ssa.Function]conditionalCallerSet, exclusive *exclusiveCallers,
) bool {
	proof := buildLockSetup(pass, function, walk.budget)
	if !proof.Proven() {
		traceLockStateBudget(pass, function)
		return false
	}
	if !proof.setup.hasAcquisition {
		return true
	}
	// A partial walk cannot establish an all-return contract. Keep diagnostics
	// and new order edges private until the bounded function walk completes.
	buffered, commit := check.BufferReports(pass)
	localRelations := newLockOrders()
	localRelations.collectOnly = true
	walk.setup = proof.setup
	if !walk.run(buffered, function, localRelations, calleeLocks, evidence, callers, exclusive) {
		traceLockStateBudget(pass, function)
		return false
	}
	commit()
	for _, edge := range localRelations.staged {
		relations.record(pass, edge.held, edge.acquired, edge.guards...)
	}
	return true
}

// lockDiagnosticProof is the final policy result consumed by reporting and
// tracing. Proven permits a diagnostic, disproven excludes it, and unknown
// suppresses it without establishing release or protection of the written field.
// Each check owns its evidence rules; this file only presents their outcomes.
type lockDiagnosticProof struct {
	state  proofs.EvidenceState
	reason lockReason
}

func traceLockDiagnostic(pass *analysis.Pass, id check.ID, position token.Pos, proof lockDiagnosticProof) {
	// Instructions with no matching obligation or owner are not candidates.
	if proof.reason == lockReasonNone {
		return
	}
	outcome := analysisTrace.DiagnosticOutcome(proof.state)
	analysisTrace.For(pass, "lockorder", string(id), position).Decision(analysisTrace.Step{
		Reason: proof.reason.String(), Outcome: outcome, Pos: position,
	})
}
