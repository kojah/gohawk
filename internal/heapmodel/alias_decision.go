package heapmodel

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// AliasDecision records a graph disjointness answer for evidence dumps.
type AliasDecision struct {
	Value, Target ssa.Value
	Reason        proofs.EvidenceReason
}
