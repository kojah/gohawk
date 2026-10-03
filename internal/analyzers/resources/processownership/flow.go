package processownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Post-Start flow owns acquisition-time command resolution, merged-command
// alternatives, return ownership and immediate success facts. Its one final
// decision carries the resolved command and uncovered return to presentation.
type processReturnProof struct {
	decision processDecision
	command  ssa.Value
	witness  *ssa.Return
}

func proveProcessReturns(pass *analysis.Pass, proof *commandProof, function *ssa.Function, start *ssa.Call, command ssa.Value) processReturnProof {
	// The receiver may be a load from a returned value-owner's field. Resolve
	// that acquisition-time load before comparing it with the owner's contents.
	// https://github.com/minio/selfupdate/blob/5b54254443f7ab80e750e1761590c1f029ecc42f/internal/binarydist/bzip2.go#L26-L40
	if resolved := heapmodel.NewStorage(nil).Resolve(command); resolved.Proven() {
		command = resolved.Value
	}
	merged := successfulCommandMerge(start, command)
	unknown := false
	owns := func(candidate ssa.Instruction) bool {
		action := proof.action(candidate, command)
		if action != ssaflow.EvidenceProven && merged != nil {
			if mergedAction := proof.action(candidate, merged); mergedAction != ssaflow.EvidenceDisproven {
				action = mergedAction
			}
		}
		unknown = unknown || action == ssaflow.EvidenceUnknown
		return action != ssaflow.EvidenceDisproven
	}
	probe := analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos())
	allowReturn := func(returned *ssa.Return) bool {
		// Returning an aggregate that contains the command transfers Wait
		// responsibility just as directly as returning *exec.Cmd itself, and
		// so does returning the started os.Process, which the caller can
		// Wait on directly. Casbin's daemon launcher returns cmd.Process:
		// https://github.com/apache/casbin-gateway/blob/e3606894348d8cd52d85abc29cfb4d3ae99595cb/util/daemon.go#L121-L131
		// Each rule that excuses a return is traced by name, so a return the
		// flow accepted can be attributed to the rule that accepted it.
		for _, rule := range []struct {
			reason processReason
			holds  func() bool
		}{
			{reasonStartFailureReturn, func() bool { return startFailureReturn(returned, start) }},
			{reasonReturnedOwner, func() bool { return lifecycle.ReturnedValueOwnsValue(returned, command) }},
			{reasonReturnedHandle, func() bool { return returnsProcessHandle(returned, command) }},
			{reasonReturnedMergedOwner, func() bool {
				return merged != nil && (lifecycle.ReturnedValueOwnsValue(returned, merged) || returnsProcessHandle(returned, merged))
			}},
		} {
			if rule.holds() {
				probe.Evidence(analysisTrace.Step{Reason: rule.reason.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: returned.Pos()})
				return true
			}
		}
		return false
	}
	var witness *ssa.Return
	// Fix only the immediate success-edge load. A later Process load may have
	// changed, so this branch fact must not imply stable handle identity or
	// excuse an additional Boolean condition around the wait or release.
	assumptions := ssaflow.EntryAssumptions{}
	if guard := proveImmediateProcessGuard(start, command); guard.State == ssaflow.EvidenceProven {
		assumptions.Constants = ssaflow.FixedValues{guard.NonNil: ssaflow.OutcomeNonNil}
		probe.Evidence(analysisTrace.Step{
			Reason: guard.Reason.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: guard.NonNil.Pos(), Function: function.String(),
		})
	}
	// Result summaries exclude only branches the shared proof rules out.
	// They never supply wait ownership; opaque results retain both edges.
	successors := summaryKnowledge.Provider(pass).Successors()
	if merged != nil {
		assumptions.NonNil = merged
		witness = ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{
			After:       merged,
			Owns:        owns,
			AllowReturn: allowReturn,
			Assume:      assumptions,
			Successors:  successors,
		})
	} else {
		witness = ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{
			AfterCallSuccess: start, Owns: owns, AllowReturn: allowReturn, Assume: assumptions, Successors: successors,
		})
	}
	return processReturnProof{
		decision: decideProcessReturn(start, command, witness, unknown, proof.budget()),
		command:  command, witness: witness,
	}
}
