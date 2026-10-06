package trace

import "github.com/kojah/gohawk/internal/engine/enumtext"

// Phase identifies the role of one trace event. Record holds its serialized label.
type Phase uint8

const (
	_ Phase = iota
	// PhaseEvidence records evidence for a candidate.
	PhaseEvidence
	// PhaseDecision records the final proof or reporting decision.
	PhaseDecision
	// PhaseLabel records an instruction classifier action.
	PhaseLabel
	// PhaseCandidate records a potentially reportable construct.
	PhaseCandidate
	// PhaseConsidered records an evaluated alternative that did not hold.
	PhaseConsidered
)

var phaseLabels = [...]string{
	0: "", PhaseEvidence: "evidence", PhaseDecision: "decision", PhaseLabel: "label",
	PhaseCandidate: "candidate", PhaseConsidered: "considered",
}

// String returns the stable presentation label.
func (phase Phase) String() string { return enumtext.Name(phase, phaseLabels[:]) }
