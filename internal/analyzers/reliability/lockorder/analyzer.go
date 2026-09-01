// Package lockorder implements the lockorder gohawk analyzer.
package lockorder

import (
	"go/token"

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
	block          *ssa.BasicBlock
	held           []string
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
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runLockOrder,
	}
}

func runLockOrder(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	relations := map[lockRelation]token.Pos{}
	for _, function := range functions {
		var evidence ssaflow.LocalEvidence
		walkLockOrder(pass, function, relations, &evidence)
	}
	return nil, nil
}
