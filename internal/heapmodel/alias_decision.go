package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// AliasDecision records a graph disjointness answer for evidence dumps.
type AliasDecision struct {
	Value, Target ssa.Value
	Reason        ssaflow.EvidenceReason
}
