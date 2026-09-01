package processownership

import (
	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func processOwnerDominatesStart(function *ssa.Function, start *ssa.Call, owners []ssa.Value) bool {
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			for _, owner := range owners {
				if ssautil.ProveCompletion(ssautil.CompletionRequest{
					Instruction: instruction,
					Target:      owner,
					Methods:     []string{"close", "Close", "kill", "Kill", "Wait", "wait"},
					Modes:       ssautil.CompletionDeferred,
				}).Proven() {
					return laterProcessOwnerWatcher(function, start, owners)
				}
			}
		}
	}
	return false
}

func laterProcessOwnerWatcher(function *ssa.Function, start *ssa.Call, owners []ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			spawn, ok := instruction.(*ssa.Go)
			if !ok || spawn.Pos() <= start.Pos() {
				continue
			}
			closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
			if closure == nil {
				continue
			}
			for _, owner := range owners {
				if ssautil.ValueContainsValue(closure, owner) {
					return true
				}
			}
		}
	}
	return false
}

func successfulStartCannotReturn(start *ssa.Call) bool {
	block := start.Block()
	for _, successor := range block.Succs {
		if success, known := ssautil.SuccessBranch(block, successor, start); known && success {
			return !ssautil.NormalReturnReachableFrom(successor)
		}
	}
	return false
}

func processOwnershipDominatesStart(function *ssa.Function, start *ssa.Call, command ssa.Value) bool {
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			if ssautil.ProveCompletion(ssautil.CompletionRequest{
				Instruction: instruction,
				Target:      command,
				Methods:     []string{"Wait"},
				Modes:       ssautil.CompletionDeferred,
			}).Proven() || ssautil.ProveOwnershipTransfer(ssautil.OwnershipTransferRequest{
				Instruction: instruction,
				Value:       command,
				Modes:       ssautil.TransferCapturedByClosure,
			}).Proven() {
				return true
			}
		}
	}
	return false
}

func processOwnersRegisteredBefore(function *ssa.Function, start *ssa.Call, command ssa.Value) []ssa.Value {
	var owners []ssa.Value
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			call, ok := instruction.(*ssa.Call)
			if !ok || call.Common().StaticCallee() == nil {
				continue
			}
			for _, argument := range call.Common().Args {
				if ssautil.SameValue(argument, command) {
					owners = append(owners, call)
					if call.Referrers() != nil {
						for _, reference := range *call.Referrers() {
							if result, ok := reference.(*ssa.Extract); ok {
								owners = append(owners, result)
							}
						}
					}
					break
				}
			}
		}
	}
	return owners
}

func processOwnershipAction(pass *analysis.Pass, instruction ssa.Instruction, command ssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	// os.Process.Release explicitly relinquishes the parent's wait/reap
	// obligation for deliberately detached daemons:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/daemon/client/autostart_fork.go#L41-L45
	return waitsForCommand(instruction, command) ||
		ssautil.CallMatchesSymbol(common, analysisutil.PackageMethod(analysisutil.MethodSymbol{PackagePath: "os", Receiver: "Process", Name: "Release"})) &&
			ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), command, map[ssa.Value]bool{}) ||
		ssautil.ProveCompletion(ssautil.CompletionRequest{
			Instruction: instruction,
			Target:      command,
			Methods:     []string{"Wait"},
			Modes: ssautil.CompletionDeferred | ssautil.CompletionInClosure |
				ssautil.CompletionByHelper,
		}).Proven() ||
		ssautil.ProveOwnershipTransfer(ssautil.OwnershipTransferRequest{
			Instruction: instruction,
			Value:       command,
			Modes: ssautil.TransferStoredInField | ssautil.TransferOwnerStoredInField |
				ssautil.TransferCapturedByClosure | ssautil.TransferCallResultStoredInField,
		}).Proven() ||
		storesProcessHandleInExternalField(instruction, command) ||
		lifecyclefacts.OwnsArgument(lifecyclefacts.ArgumentEvidence{
			Pass: pass, Analyzer: "processownership", Check: string(check.ProcessWait), Instruction: instruction, Target: command,
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.ReturnedOwner | fact.Waited },
		}) ||
		lifecyclefacts.StoresInEscapingReceiver(lifecyclefacts.ArgumentEvidence{
			Pass: pass, Analyzer: "processownership", Check: string(check.ProcessWait), Instruction: instruction, Target: command,
		}) ||
		ssautil.CallStartsClosureCallingMethodOnArgument(instruction, "Wait", command) ||
		startedWrapperWaits(pass, instruction, command) ||
		processHandleOwnershipAction(pass, instruction, command) ||
		ssautil.CallMatchesSymbol(common, analysisutil.PackageFunction("os", "Exit"))
}

func storesProcessHandleInExternalField(instruction ssa.Instruction, command ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !osProcessDerivedFromCommand(store.Val, command) {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	// Persisting the process handle on a caller-owned receiver transfers the
	// reaping obligation without exposing *exec.Cmd itself. GitHub CLI starts a
	// pager this way and waits from StopPager:
	// https://github.com/cli/cli/blob/d528f20f2ee02f6703773e9f56c90e3c3f5d46b0/pkg/iostreams/iostreams.go#L256-L274
	return ok && ssautil.ExternallyOwnedValue(field.X)
}

func startedWrapperWaits(pass *analysis.Pass, instruction ssa.Instruction, command ssa.Value) bool {
	proof := ssautil.ProveCompletion(ssautil.CompletionRequest{
		Instruction: instruction,
		Target:      command,
		Methods:     []string{"Wait"},
		Modes:       ssautil.CompletionByStartedHelper,
	})
	if !proof.Proven() {
		return false
	}
	if analysisTrace.Enabled("processownership", string(check.ProcessWait)) {
		analysisTrace.Emit(
			pass,
			analysisTrace.Event{
				Analyzer: "processownership",
				Check:    string(check.ProcessWait),
				Phase:    "evidence",
				Reason:   "started-wrapper-waiter",
				Outcome:  analysisTrace.OutcomeAccepted,
				Pos:      instruction.Pos(),
				Function: instruction.Parent().String(),
			},
		)
	}
	return true
}

func processHandleOwnershipAction(pass *analysis.Pass, instruction ssa.Instruction, command ssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, argument := range common.Args {
		if !osProcessDerivedFromCommand(argument, command) {
			continue
		}
		if lifecyclefacts.OwnsArgument(lifecyclefacts.ArgumentEvidence{
			Pass: pass, Analyzer: "processownership", Check: string(check.ProcessWait), Instruction: instruction, Target: argument,
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.ReturnedOwner | fact.Waited },
		}) || ssautil.ProveCompletion(ssautil.CompletionRequest{
			Instruction: instruction,
			Target:      argument,
			Methods:     []string{"Wait"},
			Modes:       ssautil.CompletionByHelper,
		}).Proven() ||
			ssautil.CallStartsClosureCallingMethodOnArgument(instruction, "Wait", argument) {
			if analysisTrace.Enabled("processownership", string(check.ProcessWait)) {
				analysisTrace.Emit(
					pass,
					analysisTrace.Event{
						Analyzer: "processownership",
						Check:    string(check.ProcessWait),
						Phase:    "evidence",
						Reason:   "process-handle-owner",
						Outcome:  analysisTrace.OutcomeAccepted,
						Pos:      instruction.Pos(),
						Function: instruction.Parent().String(),
					},
				)
			}
			return true
		}
	}
	return false
}
