// Package lockorder implements the lockorder gohawk analyzer.
package lockorder

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type lockRelation struct {
	from string
	to   string
}

type lockFlowState struct {
	block   *ssa.BasicBlock
	held    []string
	origins map[string]lockAcquisition
	// readHeld is the subset of held taken with RLock. A read lock grants
	// read access only, so a write while one is held is a race with any other
	// reader; tracking the mode separately keeps the rest of the walk, which
	// does not care how a lock was taken, unchanged.
	readHeld       []string
	deferred       []string
	guards         map[string]lockGuard
	condition      string
	conditionValue bool
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

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "lockorder",
		Doc:      "checks contradictory mutex acquisition order and unreleased return paths",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, concurrencyfacts.Analyzer},
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
	for _, function := range functions {
		var evidence ssaflow.LocalEvidence
		walkLockOrder(pass, function, relations, calleeLocks, &evidence)
		// A discarded Try acquisition is decided per instruction and needs no
		// lock-flow state, so it stays outside the path-sensitive walk above,
		// which may visit a block more than once.
		reportDiscardedTryLocks(pass, function)
	}
	return nil, nil
}
