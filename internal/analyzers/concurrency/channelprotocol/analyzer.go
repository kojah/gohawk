package channelprotocol

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

const instructionBudget = 2000

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

// Analyzer returns the opt-in pass for bounded, compositional channel cycles.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "channelprotocol", Doc: "checks for proven channel waiting cycles between a caller and worker",
		Requires: summaryKnowledge.Requires(), Run: run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	component, _ := summaryKnowledge.Provider(pass).Concurrency()
	engine := &summaryEngine{Engine: component}
	for _, function := range functions {
		position := candidatePosition(function)
		if !position.IsValid() {
			continue
		}
		for _, id := range []check.ID{check.ChannelProtocolBlocked, check.ChannelProtocolLockJoin, check.ChannelProtocolLockCycle} {
			probe := trace.For(pass, "channelprotocol", string(id), position)
			probe.Candidate(trace.Step{Reason: "protocol-launch", Outcome: trace.OutcomeObserved})
			proof := engine.proveCheck(function, instructionBudget, id)
			traceDecision(probe, position, proof)
			if proof.Proven() {
				reportCycle(pass, proof, id)
			}
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

func reportCycle(pass *analysis.Pass, proof cycleProof, id check.ID) {
	source := syntax.SourceRange(pass, proof.wait.Site)
	message := "channel wait prevents the worker's preceding send from completing"
	if proof.wait.Kind == groupWaitOperation {
		message = "WaitGroup wait prevents the worker's preceding send from completing"
	}
	related := []analysis.RelatedInformation{
		{Pos: proof.send.Source, Message: "worker must finish this unbuffered send before signaling completion"},
		{Pos: proof.signal.Source, Message: "completion is signaled only after the send"},
		{Pos: proof.receive.Source, Message: "the matching receive occurs only after the wait"},
	}
	if id != check.ChannelProtocolBlocked {
		message = "channel operation holds the mutex its worker needs to communicate"
		if id == check.ChannelProtocolLockJoin {
			message = "waiting for a worker while holding the mutex it needs to complete"
		}
		related = []analysis.RelatedInformation{
			{Pos: proof.send.Source, Message: "worker must acquire the caller's held mutex here"},
			{Pos: proof.signal.Source, Message: "the required worker operation follows that acquisition"},
			{Pos: proof.receive.Source, Message: "caller releases the mutex only after the blocking operation"},
		}
	}
	check.Report(pass, id, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(),
		Message: message,
		Related: related,
	})
}
