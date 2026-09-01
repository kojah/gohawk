// Package processownership implements the processownership gohawk analyzer.
package processownership

import (
	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
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
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, ok := instruction.(*ssa.Call)
				startCall := analysisutil.PackageMethod(analysisutil.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Start"})
				if !ok || !ssautil.CallMatchesSymbol(start.Common(), startCall) ||
					!execCommandValue(ssautil.CallReceiver(start.Common())) {
					continue
				}
				command := ssautil.CallReceiver(start.Common())
				owners := processOwnersRegisteredBefore(function, start, command)
				// A helper returning *exec.Cmd may already have registered cleanup
				// or wait ownership. Without interprocedural evidence either way,
				// reporting here would trade precision for recall. containerd wraps
				// command construction and returns the started command in binaryIO:
				// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/cmd/containerd-shim-runc-v2/process/io.go#L288-L330
				if commandReturnedByHelper(command) {
					continue
				}
				// Caller retains a parameter command after this helper returns, so
				// helper-local Start does not transfer caller's Wait responsibility.
				if ssautil.SameAsAny(command, parameterValues(function.Params)) || ssautil.ExternallyOwnedValue(command) {
					continue
				}
				// Cleanup may be registered before Start. This is common when a
				// constructor builds a teardown closure first, then starts the
				// process and returns that closure to its caller.
				if processOwnershipDominatesStart(function, start, command) || processOwnerDominatesStart(function, start, owners) {
					continue
				}
				if successfulStartCannotReturn(start) {
					continue
				}
				leaks := ssautil.UnownedReturnAfterCallSuccess(start, func(candidate ssa.Instruction) bool {
					return processOwnershipAction(pass, candidate, command)
				}, func(returned *ssa.Return) bool {
					// Returning an aggregate that contains the command transfers Wait
					// responsibility just as directly as returning *exec.Cmd itself.
					return startFailureReturn(returned, start) || ssautil.ReturnedValueOwnsValue(returned, command)
				})
				emitProcessDecision(pass, function, start, command, leaks)
				if leaks {
					check.Reportf(pass, check.ProcessWait, start.Pos(), "started command is not waited on every successful return path")
				}
			}
		}
	}
	return nil, nil
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
	analysisTrace.Emit(
		pass,
		analysisTrace.Event{
			Analyzer: "processownership",
			Check:    checkID,
			Phase:    "decision",
			Reason:   reason,
			Outcome:  outcome,
			Pos:      start.Pos(),
			Function: function.String(),
			Details:  details,
		},
	)
}
