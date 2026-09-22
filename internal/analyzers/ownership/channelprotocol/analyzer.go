package channelprotocol

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const instructionBudget = 2000

// Analyzer returns the opt-in pass for bounded, compositional channel cycles.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "channelprotocol", Doc: "checks for proven channel waiting cycles between a caller and worker",
		Requires: []*analysis.Analyzer{buildssa.Analyzer}, Run: run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	engine := newSummaryEngine()
	for _, function := range functions {
		position := candidatePosition(function)
		if !position.IsValid() {
			continue
		}
		probe := trace.For(pass, "channelprotocol", string(check.ChannelProtocolBlocked), position)
		probe.Candidate(trace.Step{Reason: "protocol-launch", Outcome: trace.OutcomeObserved})
		proof := engine.prove(function, instructionBudget)
		traceDecision(probe, position, proof)
		if proof.Proven() {
			reportCycle(pass, proof)
		}
	}
	return nil, nil
}

// Locate a candidate cheaply before summary construction, so a trace records
// entry even when a callee walk consumes the entire budget.
func candidatePosition(function *ssa.Function) token.Pos {
	var launch token.Pos
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if _, ok := instruction.(*ssa.Go); ok && !launch.IsValid() {
				launch = instruction.Pos()
			}
			if !launch.IsValid() {
				continue
			}
			switch instruction := instruction.(type) {
			case *ssa.UnOp:
				if instruction.Op == token.ARROW {
					return instruction.Pos()
				}
			case *ssa.Call:
				return instruction.Pos()
			}
		}
	}
	return launch
}

func traceDecision(probe trace.Probe, position token.Pos, proof cycleProof) {
	outcome := trace.OutcomeUnknown
	switch proof.State {
	case ssaflow.EvidenceProven:
		outcome = trace.OutcomeRejected
	case ssaflow.EvidenceDisproven:
		outcome = trace.OutcomeAccepted
	case ssaflow.EvidenceUnknown:
		// Incomplete participation or ordering never establishes a cycle.
	}
	probe.Decision(trace.Step{Reason: string(proof.Reason), Outcome: outcome, Pos: position})
}

func reportCycle(pass *analysis.Pass, proof cycleProof) {
	source := syntax.SourceRange(pass, proof.wait.site)
	message := "channel wait prevents the worker's preceding send from completing"
	if proof.wait.kind == groupWaitOperation {
		message = "WaitGroup wait prevents the worker's preceding send from completing"
	}
	check.Report(pass, check.ChannelProtocolBlocked, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(),
		Message: message,
		Related: []analysis.RelatedInformation{
			{Pos: proof.send.source, Message: "worker must finish this unbuffered send before signaling completion"},
			{Pos: proof.signal.source, Message: "completion is signaled only after the send"},
			{Pos: proof.receive.source, Message: "the matching receive occurs only after the wait"},
		},
	})
}
