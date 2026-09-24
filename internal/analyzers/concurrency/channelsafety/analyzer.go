// Package channelsafety implements the channelsafety gohawk analyzer.

package channelsafety

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "channelsafety", Doc: "checks channel operations for use after close and bounded dependency cycles",
		Requires: summaryKnowledge.Requires(), Run: runChannelSafety,
	}
}

func runChannelSafety(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		effects := channelEffects(pass, function)
		reportSendsAfterClose(pass, function, effects)
		reportChannelCycle(pass, function)
	}
	return nil, nil
}

func reportSendsAfterClose(pass *analysis.Pass, function *ssa.Function, effects map[ssa.Instruction][]concurrencyfacts.Operation) {
	reported := map[token.Pos]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			for _, closed := range effects[instruction] {
				if closed.Kind == concurrencyfacts.Close {
					reportFollowingSends(pass, function, instruction, closed, effects, reported)
				}
			}
		}
	}
}

func reportFollowingSends(
	pass *analysis.Pass, function *ssa.Function, instruction ssa.Instruction, closed concurrencyfacts.Operation,
	effects map[ssa.Instruction][]concurrencyfacts.Operation, reported map[token.Pos]bool,
) {
	// Crossing a backedge may compare two different runtime channel
	// values represented by the same loop-carried SSA value. Reporting
	// that as send-after-close is not sufficiently precise, so only
	// instructions reachable without a back edge are candidates.
	for _, candidate := range ssaflow.InstructionsReachableAfter(instruction) {
		for _, sent := range effects[candidate] {
			if sent.Kind != concurrencyfacts.Send || reported[candidate.Pos()] {
				continue
			}
			probe := analysisTrace.For(pass, "channelsafety", string(check.ChannelSendAfterClose), candidate.Pos())
			probe.Candidate(analysisTrace.Step{Reason: "send-reachable-after-close", Outcome: analysisTrace.OutcomeObserved})
			identity := heapmodel.NewStorage(nil).Same(sent.Resource.Value, closed.Resource.Value)
			emitChannelIdentityDecision(pass, function, probe, instruction, candidate, identity)
			if !identity.Proven() {
				continue
			}
			reported[candidate.Pos()] = true
			sendSource := syntax.SourceRange(pass, candidate.Pos())
			closeSource := syntax.SourceRange(pass, instruction.Pos())
			check.Report(pass, check.ChannelSendAfterClose, analysis.Diagnostic{
				Pos: sendSource.Pos(), End: sendSource.End(), Message: "send follows close of channel",
				Related: []analysis.RelatedInformation{{
					Pos: closeSource.Pos(), End: closeSource.End(), Message: "channel closed here",
				}},
			})
		}
	}
}

func emitChannelIdentityDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	probe analysisTrace.Probe,
	closeInstruction ssa.Instruction,
	send ssa.Instruction,
	identity ssaflow.IdentityProof,
) {
	if !probe.Enabled() {
		return
	}
	reason := "send-channel-identity-not-proven"
	outcome := analysisTrace.OutcomeUnknown
	if identity.Proven() {
		reason = "send-after-close-proven"
		outcome = analysisTrace.OutcomeRejected
	}
	probe.Decision(analysisTrace.Step{
		Reason: reason, Outcome: outcome, Pos: send.Pos(), Function: function.String(),
		Details: map[string]string{
			"close":           pass.Fset.Position(closeInstruction.Pos()).String(),
			"identity_reason": string(identity.Reason),
			"instruction":     send.String(),
		},
	})
}
