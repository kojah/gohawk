// Package processownership implements the processownership gohawk analyzer.
package processownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "processownership",
		Doc:      "checks that started os/exec commands are waited on or transferred to a wait owner",
		Requires: summaryKnowledge.Requires(),
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("processownership", string(check.ProcessWait))
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, command, ok := startedCommand(instruction)
				if !ok {
					continue
				}
				evidence.ForCandidate(start.Pos())
				probe := analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos())
				proof := &commandProof{evidence: evidence, pool: ssaflow.NewSearchBudget(processPoolBudget).Observed(probe.Observer())}
				if commandOwnedElsewhere(pass, proof, function, start, command) {
					continue
				}
				reportStartedCommand(pass, proof, function, start, command)
			}
		}
	}
	return nil, nil
}

// startedCommand returns the Start call and the *exec.Cmd it starts.
func startedCommand(instruction ssa.Instruction) (*ssa.Call, ssa.Value, bool) { //nolint:ireturn // Commands retain their concrete SSA forms.
	start, ok := instruction.(*ssa.Call)
	startCall := syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Start"})
	if !ok || !ssaflow.CallMatchesSymbol(start.Common(), startCall) || !execCommandValue(ssaflow.CallReceiver(start.Common())) {
		return nil, nil, false
	}
	return start, ssaflow.CallReceiver(start.Common()), true
}

// commandOwnedElsewhere reports whether the started command's Wait
// responsibility provably or possibly lies outside this function, so the
// flow after Start is not asked about it.
func commandOwnedElsewhere(
	pass *analysis.Pass, proof *commandProof, function *ssa.Function, start *ssa.Call, command ssa.Value,
) bool {
	prefix := collectProcessStartInstructions(start, command, proof.budget())
	if !prefix.Proven() {
		analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos()).Decision(analysisTrace.Step{
			Reason: prefix.Reason.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: start.Pos(),
		})
		return true
	}
	owners := prefix.owners
	// A helper returning *exec.Cmd may already have registered cleanup
	// or wait ownership. Without interprocedural evidence either way,
	// reporting here would trade precision for recall. containerd wraps
	// command construction and returns the started command in binaryIO:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/cmd/containerd-shim-runc-v2/process/io.go#L288-L330
	if commandReturnedByHelper(command) {
		analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos()).Decision(analysisTrace.Step{
			Reason: reasonHelperOwnershipUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: start.Pos(),
		})
		return true
	}
	// Caller retains a parameter command after this helper returns, so
	// helper-local Start does not transfer caller's Wait responsibility.
	if heapmodel.MayAliasAny(command, parameterValues(function.Params)) || ssaflow.ExternallyOwnedValue(command) {
		return true
	}
	// A command loaded from an element of an aggregate is shared with
	// every other reader of that aggregate, which may wait on it through
	// a different element load the flow cannot link back. cocoon starts
	// worker commands from one loop over a slice and waits in another:
	// https://github.com/cocoonstack/cocoon/blob/51ff88bcf8f175a2d82b162d9bf9f65604a607b5/cmd/storebench/main.go#L123-L138
	if ssaflow.ElementOfAggregate(command) {
		return true
	}
	// Cleanup may be registered before Start. This is common when a
	// constructor builds a teardown closure first, then starts the
	// process and returns that closure to its caller.
	if processOwnershipDominatesStart(proof, prefix.instructions, command) ||
		processOwnerDominatesStart(proof, function, start, owners, prefix.instructions) ||
		commandStoredExternallyBeforeStart(prefix.instructions, command) {
		return true
	}
	returns := successfulStartCannotReturn(start, proof.budget())
	if returns.Reason == ssaflow.EvidenceBudgetExhausted {
		analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos()).Decision(analysisTrace.Step{
			Reason: returns.Reason.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: start.Pos(),
		})
	}
	return returns.State != ssaflow.EvidenceDisproven
}

// reportStartedCommand asks the flow whether every successful return waits on
// or transfers the command, and reports partial wait ownership.
func reportStartedCommand(pass *analysis.Pass, proof *commandProof, function *ssa.Function, start *ssa.Call, command ssa.Value) {
	// The receiver may be a load from a returned value-owner's field. Resolve
	// that acquisition-time load before comparing it with the owner's contents.
	// https://github.com/minio/selfupdate/blob/5b54254443f7ab80e750e1761590c1f029ecc42f/internal/binarydist/bzip2.go#L26-L40
	if resolved := heapmodel.NewStorage(nil).Resolve(command); resolved.Proven() {
		command = resolved.Value
	}
	merged := successfulCommandMerge(start, command)
	unknown := false
	owns := func(candidate ssa.Instruction) bool {
		action := processOwnershipAction(proof, candidate, command)
		if action != ssaflow.EvidenceProven && merged != nil {
			if mergedAction := processOwnershipAction(proof, candidate, merged); mergedAction != ssaflow.EvidenceDisproven {
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
	decision := decideProcessReturn(start, command, witness, unknown, proof.budget())
	emitProcessDecision(pass, function, start, command, decision)
	if decision.state != ssaflow.EvidenceProven {
		return
	}
	subject := "the command"
	if name := commandName(pass, command); name != "" {
		subject = "`" + name + "`"
	}
	source := syntax.SourceRange(pass, start.Pos())
	check.Report(pass, check.ProcessWait, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: "started command is not waited on every successful return path",
		Related: check.ReturnEvidence(pass, start, witness, "waiting for "+subject),
	})
}

// commandStoredExternallyBeforeStart reports whether the command was stored
// into caller-owned storage on every path to Start, typically a receiver
// field that a later method or goroutine waits through. The walk after Start
// cannot see that store, so it is asked here. Istio's Envoy driver keeps the
// command on the receiver and waits on e.cmd from a goroutine:
// https://github.com/istio/proxy/blob/1bdb025a454d26a55ffa11a50e5c0a70dff7d853/test/envoye2e/driver/envoy.go#L135-L154
func commandStoredExternallyBeforeStart(before []ssa.Instruction, command ssa.Value) bool {
	for _, instruction := range before {
		store, ok := instruction.(*ssa.Store)
		if !ok || !heapmodel.MayAlias(store.Val, command) {
			continue
		}
		if storesProcessHandleInExternalField(store, command) || externallyOwnedAddress(store.Addr) {
			return true
		}
	}
	return false
}

func externallyOwnedAddress(address ssa.Value) bool {
	field, ok := address.(*ssa.FieldAddr)
	return ok && ssaflow.ExternallyOwnedValue(field.X)
}

// commandName names the command by its variable: the one exec.Command's
// result was assigned to, or the local a load reads it from.
func commandName(pass *analysis.Pass, command ssa.Value) string {
	switch value := command.(type) {
	case *ssa.Call:
		return syntax.AssignedName(pass, value.Pos(), 0)
	case *ssa.UnOp:
		if cell, ok := value.X.(*ssa.Alloc); ok && value.Op == token.MUL {
			return cell.Comment
		}
	case *ssa.Alloc:
		return value.Comment
	}
	return ""
}
