// Package channelsafety implements the channelsafety gohawk analyzer.
package channelsafety

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "channelsafety", Doc: "checks channel operations for reachable use after close",
		Requires: []*analysis.Analyzer{buildssa.Analyzer}, Run: runChannelSafety,
	}
}

func runChannelSafety(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportSendsAfterClose(pass, function)
	}
	return nil, nil
}

func reportSendsAfterClose(pass *analysis.Pass, function *ssa.Function) {
	reported := map[token.Pos]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) || len(common.Args) != 1 {
				continue
			}
			if _, deferred := instruction.(*ssa.Defer); deferred {
				continue
			}
			// Crossing a backedge may compare two different runtime channel
			// values represented by the same loop-carried SSA value. Reporting
			// that as send-after-close is not sufficiently precise, so only
			// instructions reachable without a back edge are candidates.
			for _, candidate := range ssaflow.InstructionsReachableAfter(instruction) {
				send, ok := candidate.(*ssa.Send)
				if !ok || reported[send.Pos()] {
					continue
				}
				probe := analysisTrace.For(pass, "channelsafety", string(check.ChannelSendAfterClose), send.Pos())
				probe.Candidate(analysisTrace.Step{Reason: "send-reachable-after-close", Outcome: analysisTrace.OutcomeObserved})
				identity := ssaflow.NewStorage(ssaflow.NewSearchBudget(1000)).Same(send.Chan, common.Args[0])
				if !identity.Proven() {
					emitChannelIdentityDecision(pass, function, probe, instruction, send, identity)
					continue
				}
				emitChannelIdentityDecision(pass, function, probe, instruction, send, identity)
				reported[send.Pos()] = true
				sendSource := syntax.SourceRange(pass, send.Pos())
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
}

func emitChannelIdentityDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	probe analysisTrace.Probe,
	closeInstruction ssa.Instruction,
	send *ssa.Send,
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
