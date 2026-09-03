// Package processownership implements the processownership gohawk analyzer.
package processownership

import (
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
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
		Name:     "processownership",
		Doc:      "checks that started os/exec commands are waited on or transferred to a wait owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		evidence := lifecyclefacts.NewLifecycleEvidence(pass, "processownership", string(check.ProcessWait))
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, command, ok := startedCommand(instruction)
				if !ok {
					continue
				}
				evidence.ForCandidate(start.Pos())
				if commandOwnedElsewhere(evidence, function, start, command) {
					continue
				}
				reportStartedCommand(pass, evidence, function, start, command)
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
func commandOwnedElsewhere(evidence *lifecyclefacts.LifecycleEvidence, function *ssa.Function, start *ssa.Call, command ssa.Value) bool {
	owners := processOwnersRegisteredBefore(function, start, command)
	// A helper returning *exec.Cmd may already have registered cleanup
	// or wait ownership. Without interprocedural evidence either way,
	// reporting here would trade precision for recall. containerd wraps
	// command construction and returns the started command in binaryIO:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/cmd/containerd-shim-runc-v2/process/io.go#L288-L330
	if commandReturnedByHelper(command) {
		return true
	}
	// Caller retains a parameter command after this helper returns, so
	// helper-local Start does not transfer caller's Wait responsibility.
	if ssaflow.SameAsAny(command, parameterValues(function.Params)) || ssaflow.ExternallyOwnedValue(command) {
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
	if processOwnershipDominatesStart(evidence, function, start, command) ||
		processOwnerDominatesStart(evidence, function, start, owners) ||
		commandStoredExternallyBeforeStart(start, command) {
		return true
	}
	return successfulStartCannotReturn(start)
}

// reportStartedCommand asks the flow whether every successful return waits on
// or transfers the command, and reports the defect or the detached launch.
func reportStartedCommand(pass *analysis.Pass, evidence *lifecyclefacts.LifecycleEvidence, function *ssa.Function, start *ssa.Call, command ssa.Value) {
	leaks := ssaflow.UnownedReturnAfterCallSuccess(start, func(candidate ssa.Instruction) bool {
		return processOwnershipAction(evidence, candidate, command)
	}, func(returned *ssa.Return) bool {
		// Returning an aggregate that contains the command transfers Wait
		// responsibility just as directly as returning *exec.Cmd itself, and
		// so does returning the started os.Process, which the caller can
		// Wait on directly. Casbin's daemon launcher returns cmd.Process:
		// https://github.com/apache/casbin-gateway/blob/e3606894348d8cd52d85abc29cfb4d3ae99595cb/util/daemon.go#L121-L131
		return startFailureReturn(returned, start) || ssaflow.ReturnedValueOwnsValue(returned, command) ||
			returnsProcessHandle(returned, command)
	})
	emitProcessDecision(pass, function, start, command, leaks)
	if !leaks {
		return
	}
	// A launch whose handle is never touched again is a policy choice
	// the project made deliberately, such as opening a browser, and is
	// reported only by the opt-in detached audit. A handle that is
	// waited on or released on some paths but not all is a defect.
	if commandUnusedAfterStart(start, command) {
		check.Reportf(pass, check.ProcessDetached, start.Pos(), "started command is never waited on or released")
		return
	}
	check.Reportf(pass, check.ProcessWait, start.Pos(), "started command is not waited on every successful return path")
}

func emitProcessDecision(pass *analysis.Pass, function *ssa.Function, start *ssa.Call, command ssa.Value, leaks bool) {
	checkID := string(check.ProcessWait)
	if !analysisTrace.Enabled("processownership", checkID) {
		return
	}
	outcome, reason := analysisTrace.OutcomeAccepted, "wait-ownership-proven"
	if leaks {
		outcome, reason = analysisTrace.OutcomeRejected, "unowned-return"
	}
	details := map[string]string{}
	if command != nil && command.Type() != nil {
		details["command_type"] = command.Type().String()
	}
	analysisTrace.For(pass, "processownership", checkID, start.Pos()).Decision(analysisTrace.Step{
		Reason:   reason,
		Outcome:  outcome,
		Pos:      start.Pos(),
		Function: function.String(),
		Details:  details,
	})
}

// commandStoredExternallyBeforeStart reports whether the command was stored
// into caller-owned storage on every path to Start, typically a receiver
// field that a later method or goroutine waits through. The walk after Start
// cannot see that store, so it is asked here. Istio's Envoy driver keeps the
// command on the receiver and waits on e.cmd from a goroutine:
// https://github.com/istio/proxy/blob/1bdb025a454d26a55ffa11a50e5c0a70dff7d853/test/envoye2e/driver/envoy.go#L135-L154
func commandStoredExternallyBeforeStart(start *ssa.Call, command ssa.Value) bool {
	for _, block := range start.Parent().Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || !ssaflow.InstructionDominates(store, start) || !ssaflow.SameValue(store.Val, command) {
				continue
			}
			if storesProcessHandleInExternalField(store, command) || externallyOwnedAddress(store.Addr) {
				return true
			}
		}
	}
	return false
}

func externallyOwnedAddress(address ssa.Value) bool {
	field, ok := address.(*ssa.FieldAddr)
	return ok && ssaflow.ExternallyOwnedValue(field.X)
}

// commandUnusedAfterStart reports whether no instruction reachable after
// Start reads the command or anything derived from it, such as its Process.
func commandUnusedAfterStart(start *ssa.Call, command ssa.Value) bool {
	for _, block := range start.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if instruction == start || !ssaflow.InstructionMayFollow(start, instruction) {
				continue
			}
			if _, ok := instruction.(*ssa.DebugRef); ok {
				continue
			}
			// A literal that captures the handle may wait on it later.
			if closure, ok := instruction.(*ssa.MakeClosure); ok {
				for _, binding := range closure.Bindings {
					if ssaflow.CapturedBindingMatches(binding, command) {
						return false
					}
				}
			}
			// Only handing the handle on counts as a use: a call receiving it, a
			// store, a return, or a send. Reading a field such as the child's
			// PID for a log line touches nothing that could wait on or release
			// the process. agent-filesystem daemonizes itself this way:
			// https://github.com/redis/agent-filesystem/blob/62aebf8f4d4b3a3866f4034fc6501af7d5d4a133/mount/cmd/agent-filesystem-mount/main.go#L70-L90
			if !handsValueOn(instruction) {
				continue
			}
			for _, operand := range instruction.Operands(nil) {
				if operand == nil || *operand == nil || ssaflow.ValueDerivesFrom(*operand, start, map[ssa.Value]bool{}) {
					// Start's own error result is not a use of the handle.
					continue
				}
				if handleCarried(*operand, command) {
					return false
				}
			}
		}
	}
	return true
}

// handsValueOn reports whether an instruction can pass a value it consumes
// to code or storage that outlives the instruction.
func handsValueOn(instruction ssa.Instruction) bool {
	switch instruction.(type) {
	case ssa.CallInstruction, *ssa.Store, *ssa.Return, *ssa.Send, *ssa.MapUpdate, *ssa.Panic:
		return true
	}
	return false
}

// handleCarried reports whether value is the command handle or something
// bound to it: a projection such as cmd.Process, a pipe the command returned,
// or an aggregate holding either. Derivation stops at a scalar, because a PID
// or a name read from the handle is data the recipient cannot wait on or
// release.
func handleCarried(value, command ssa.Value) bool {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return ssaflow.NewReachingWalk(forms).Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if _, scalar := value.Type().Underlying().(*types.Basic); scalar {
			return false
		}
		if ssaflow.SameValue(value, command) {
			return true
		}
		if load, ok := value.(*ssa.UnOp); ok {
			for stored := range ssaflow.StoredInto(load.X) {
				if walk.Any(stored, func(walk ssaflow.ReachingWalk, stored ssa.Value) bool { return handleCarried(stored, command) }) {
					return true
				}
			}
		}
		instruction, ok := value.(ssa.Instruction)
		if !ok {
			return false
		}
		for _, operand := range instruction.Operands(nil) {
			if operand != nil && *operand != nil && handleCarried(*operand, command) {
				return true
			}
		}
		return false
	})
}
