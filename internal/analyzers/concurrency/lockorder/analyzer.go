// Package lockorder implements the lockorder gohawk analyzer.
package lockorder

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"

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
	constraints    []lockGuard
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
		Name:     "lockorder",
		Doc:      "checks contradictory mutex acquisition order and unreleased return paths",
		Requires: summaryKnowledge.Requires(),
		Run:      runLockOrder,
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
	callers := conditionalCallerSets(append([]*ssa.Function{ssaResult.Pkg.Func("init")}, ssaResult.SrcFuncs...))
	for _, function := range functions {
		var evidence ssaflow.LocalEvidence
		walkLockOrder(pass, function, relations, calleeLocks, &evidence, callers)
		// A discarded Try acquisition is decided per instruction and needs no
		// lock-flow state, so it stays outside the path-sensitive walk above,
		// which may visit a block more than once.
		reportDiscardedTryLocks(pass, function)
	}
	return nil, nil
}

func walkLockOrder(
	pass *analysis.Pass,
	function *ssa.Function,
	relations *lockOrders,
	calleeLocks *calleeLockSearch,
	evidence *ssaflow.LocalEvidence,
	callers map[*ssa.Function]conditionalCallerSet,
) {
	summaries := summarizedMutexEffects(pass, function)
	if !hasMutexAcquisition(function, summaries) {
		return
	}
	// A partial walk cannot establish an all-return contract. Keep diagnostics
	// and new order edges private until the bounded function walk completes.
	buffered, commit := check.BufferReports(pass)
	localRelations := newLockOrders()
	localRelations.collectOnly = true
	if !walkLockOrderBounded(buffered, function, localRelations, calleeLocks, evidence, callers, summaries) {
		traceLockStateBudget(pass, function)
		return
	}
	commit()
	for _, edge := range localRelations.staged {
		relations.record(pass, edge.held, edge.acquired, edge.guards...)
	}
}
