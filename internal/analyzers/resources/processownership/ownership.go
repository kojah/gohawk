package processownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// processQueryBudget bounds one completion, transfer, or handoff question
// the proof asks; exhaustion is unknown evidence, never a wait or a leak.
const processQueryBudget = ssaflow.QueryBudget

// processPoolBudget bounds a whole started-command proof. A per-query bound
// does not bound the proof, which asks one question per instruction after
// Start and per registration before it; a candidate in a large function
// could otherwise cost without limit. A hundred full queries is far beyond
// an ordinary proof, so the pool decides only pathological candidates, and
// it decides them the way a single exhausted query does: as unknown.
const processPoolBudget = 100 * processQueryBudget

// commandProof is the evidence context for one started command: the
// lifecycle evidence its questions go through and the pool they draw on.
type commandProof struct {
	evidence *lifecyclefacts.LifecycleEvidence
	pool     *ssaflow.SearchBudget
}

// budget draws one query's allowance from the candidate's pool so the
// give-ups inside it reach the trace and the proof as a whole stays bounded.
func (proof *commandProof) budget() *ssaflow.SearchBudget {
	return proof.pool.Within(processQueryBudget)
}

// abandoned reports a question the search gave up on before deciding. What
// that permits is decided where the question was asked: before Start it
// leaves ownership possibly registered, after Start it leaves the action
// unknown, and neither is a reason to report.
func abandoned(result lifecyclefacts.Proof) bool {
	return result.Reason == ssaflow.EvidenceBudgetExhausted
}

// Pre-start ownership is accepted only when cleanup registration dominates
// Start. Wrapper owners additionally need a later watcher that captures the
// same owner; a deferred method alone does not prove the process is observed.
func processOwnerDominatesStart(
	proof *commandProof,
	function *ssa.Function,
	start *ssa.Call,
	owners []ssa.Value,
) bool {
	startIndex := ssaflow.InstructionIndex(start)
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
				completion := lifecycle.CompletionRequest{
					Instruction: instruction,
					Target:      owner,
					Methods:     []string{"close", "Close", "kill", "Kill", "Wait", "wait"},
					Budget:      proof.budget(),
				}
				result := proof.evidence.Prove(lifecyclefacts.EvidenceRequest{
					Instruction: instruction,
					Target:      owner,
					Completion:  &completion,
				})
				if abandoned(result) {
					return true
				}
				if result.Proven() && result.Reason == ssaflow.EvidenceDeferredCompletion {
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
				if lifecycle.MayContainValue(closure, owner) {
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
		if success, known := ssaflow.SuccessBranch(block, successor, start); known && success {
			return !ssaflow.NormalReturnReachableFrom(successor)
		}
	}
	return false
}

// Registering command cleanup before Start is sufficient only on a dominating
// path. A later registration cannot protect an early successful return, so it
// remains part of the ordinary post-Start flow proof instead.
func processOwnershipDominatesStart(
	proof *commandProof,
	function *ssa.Function,
	start *ssa.Call,
	command ssa.Value,
) bool {
	startIndex := ssaflow.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			completion := lifecycle.CompletionRequest{
				Instruction: instruction,
				Target:      command,
				Methods:     []string{"Wait"},
				Budget:      proof.budget(),
			}
			transfer := lifecycle.OwnershipTransferRequest{
				Instruction: instruction,
				Value:       command,
				Modes:       lifecycle.TransferCapturedByClosure,
			}
			result := proof.evidence.Prove(lifecyclefacts.EvidenceRequest{
				Instruction: instruction,
				Target:      command,
				Completion:  &completion,
				Transfer:    &transfer,
			})
			if abandoned(result) ||
				result.Proven() && (result.Reason == ssaflow.EvidenceDeferredCompletion || result.Reason == ssaflow.EvidenceCapturedByClosure) {
				return true
			}
		}
	}
	return false
}

func processOwnersRegisteredBefore(function *ssa.Function, start *ssa.Call, command ssa.Value) []ssa.Value {
	var owners []ssa.Value
	startIndex := ssaflow.InstructionIndex(start)
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
				if heapmodel.MayAlias(argument, command) {
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

func processOwnershipAction(proof *commandProof, instruction ssa.Instruction, command ssa.Value) ssaflow.EvidenceState {
	if possibleWaitHandoff(instruction, command, proof.budget()) {
		return ssaflow.EvidenceUnknown
	}
	if deferred := deferredClosureWaitsForCommand(instruction, command); deferred != ssaflow.EvidenceDisproven {
		return deferred
	}
	common := ssaflow.InstructionCall(instruction)
	completion := lifecycle.CompletionRequest{
		Instruction: instruction,
		Target:      command,
		Methods:     []string{"Wait"},
		Budget:      proof.budget(),
	}
	transfer := lifecycle.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       command,
		Modes: lifecycle.TransferStoredInField | lifecycle.TransferOwnerStoredInField |
			lifecycle.TransferCapturedByClosure | lifecycle.TransferCallResultStoredInField,
	}
	// A launched waiter owns reaping when every normal goroutine return waits,
	// including a nested defer. feint uses both direct and deferred background
	// waiters for deliberately longer-lived commands:
	// https://github.com/stephrobert/feint/blob/270aeb83c264ad109af885bb4e52f598265c5e1f/internal/cli/lifecycle.go#L183-L206
	// https://github.com/stephrobert/feint/blob/270aeb83c264ad109af885bb4e52f598265c5e1f/internal/core/machine/incus_watch.go#L51-L61
	// os.Process.Release explicitly relinquishes the parent's wait/reap
	// obligation for deliberately detached daemons:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/daemon/client/autostart_fork.go#L41-L45
	// The questions are asked in the order of the disjunction, so a cheap
	// syntactic witness is never charged for a search it did not need, and
	// the results of the searched ones are kept to tell an abandoned search
	// from a disproof.
	var ownership lifecyclefacts.Proof
	owns := func() bool {
		ownership = proof.evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      command,
			Completion:  &completion,
			Transfer:    &transfer,
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				return fact.ReturnedOwner | fact.Waited
			},
			ReceiverStore: true,
		})
		return ownership.Proven()
	}
	handle := ssaflow.EvidenceDisproven
	handles := func() bool {
		handle = processHandleOwnershipAction(proof, instruction, command)
		return handle == ssaflow.EvidenceProven
	}
	if waitsForCommand(instruction, command) ||
		ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os", Receiver: "Process", Name: "Release"})) &&
			heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), command, map[ssa.Value]bool{}) ||
		owns() ||
		storesProcessHandleInExternalField(instruction, command) ||
		handles() ||
		ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", "Exit")) {
		return ssaflow.EvidenceProven
	}
	if abandoned(ownership) || handle == ssaflow.EvidenceUnknown {
		return ssaflow.EvidenceUnknown
	}
	return ssaflow.EvidenceDisproven
}

// A callback supplied to an opaque runner may own the wait. This is a reason
// to decline loss, not evidence that the runner invokes or joins the callback.
// A started worker with no normal return also remains opaque when it contains
// a positive Wait witness: every-return completion deliberately excludes it.
// https://github.com/la5nta/pat/blob/2e6a8d14baf0268f4e2aa4d01784a54ca935cf52/internal/prehook/prehook.go#L109-L114
func possibleWaitHandoff(instruction ssa.Instruction, command ssa.Value, budget *ssaflow.SearchBudget) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	// A merged receiver may select the successfully started command. The
	// flow does not retain acquisition-error/receiver correlation, so possible
	// identity makes this action unknown, never a guaranteed Wait. An earlier
	// return that bypasses the action is still checked by the ordinary flow.
	// https://github.com/raskrebs/sonar/blob/9c963b8447d6ca08dd4a3c0bc6c0bf27527cd793/internal/runs/runs_test.go#L117-L130
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"})) {
		receiver := ssaflow.CallReceiver(common)
		_, merged := receiver.(*ssa.Phi)
		return merged && heapmodel.MayAlias(receiver, command) &&
			!heapmodel.NewStorage(nil).Same(receiver, command).Proven()
	}
	if _, spawned := instruction.(*ssa.Go); spawned {
		callee, _ := ssaflow.DirectCallee(common)
		if callee == nil || len(callee.Blocks) == 0 || ssaflow.NormalReturnReachableFrom(callee.Blocks[0]) {
			return false
		}
		return lifecycle.ProveCompletion(lifecycle.CompletionRequest{
			Instruction: instruction, Target: command, Methods: []string{"Wait"},
			Coverage: lifecycle.CoverageAnywhere, Budget: budget,
		}).Proven()
	}
	callee, _ := ssaflow.DirectCallee(common)
	if callee != nil && len(callee.Blocks) != 0 {
		return false
	}
	for _, argument := range common.Args {
		if _, callback := argument.(*ssa.MakeClosure); callback && lifecycle.MayContainValue(argument, command) {
			return true
		}
	}
	return false
}

func deferredClosureWaitsForCommand(instruction ssa.Instruction, command ssa.Value) ssaflow.EvidenceState {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return ssaflow.EvidenceDisproven
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return ssaflow.EvidenceDisproven
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	if closure == nil {
		return ssaflow.EvidenceDisproven
	}
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return ssaflow.EvidenceDisproven
	}
	waitsOnEveryReturn := func(local ssa.Value) ssaflow.EvidenceState {
		// A successful Cmd.Start guarantees Cmd.Process is non-nil. Use the
		// concrete Wait receiver as the closure's non-nil assumption so a
		// defensive `if cmd.Process != nil` guard does not create a spurious
		// path that skips Wait.
		var receivers []ssa.Value
		for _, block := range function.Blocks {
			for _, candidate := range block.Instrs {
				if waitsForCommand(candidate, local) {
					receivers = append(receivers, ssaflow.CallReceiver(ssaflow.InstructionCall(candidate)))
				}
			}
		}
		for _, receiver := range receivers {
			if lifecycle.MethodCallCoverage(function, func(candidate ssa.Instruction) bool {
				return waitsForCommand(candidate, local)
			}, lifecycle.CoverageEveryReturn, receiver) {
				return ssaflow.EvidenceProven
			}
		}
		return guardedDeferredWait(function, local)
	}
	for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
		if heapmodel.CapturedBindingMatches(captured.Binding, command) {
			if proof := waitsOnEveryReturn(captured.Free); proof != ssaflow.EvidenceDisproven {
				return proof
			}
		}
	}
	for index, parameter := range function.Params {
		if index < len(common.Args) && heapmodel.MayAlias(common.Args[index], command) {
			if proof := waitsOnEveryReturn(parameter); proof != ssaflow.EvidenceDisproven {
				return proof
			}
		}
	}
	return ssaflow.EvidenceDisproven
}

// A deferred waiter guarded only by its captured Cmd.Process field may own
// reaping, but distinct loads do not establish stable identity. Preserve that
// uncertainty without teaching shared non-nil flow that possible aliases are
// equal. A Boolean condition still leaves an unowned path, and a visible field
// replacement defeats even this possible successful-Start contract.
func guardedDeferredWait(function *ssa.Function, command ssa.Value) ssaflow.EvidenceState {
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](function) {
		if heapmodel.ValueDerivesFrom(store.Addr, command, map[ssa.Value]bool{}) {
			return ssaflow.EvidenceDisproven
		}
	}
	for _, load := range ssaflow.InstructionsOf[*ssa.UnOp](function) {
		if !osProcessDerivedFromCommand(load, command) {
			continue
		}
		if lifecycle.MethodCallCoverage(function, func(candidate ssa.Instruction) bool {
			return waitsForCommand(candidate, command)
		}, lifecycle.CoverageEveryReturn, load) {
			return ssaflow.EvidenceUnknown
		}
	}
	return ssaflow.EvidenceDisproven
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
	return ok && ssaflow.ExternallyOwnedValue(field.X)
}

func processHandleOwnershipAction(proof *commandProof, instruction ssa.Instruction, command ssa.Value) ssaflow.EvidenceState {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return ssaflow.EvidenceDisproven
	}
	state := ssaflow.EvidenceDisproven
	for _, argument := range common.Args {
		if !osProcessDerivedFromCommand(argument, command) {
			continue
		}
		completion := lifecycle.CompletionRequest{
			Instruction: instruction,
			Target:      argument,
			Methods:     []string{"Wait"},
			Budget:      proof.budget(),
		}
		result := proof.evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      argument,
			Completion:  &completion,
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				return fact.ReturnedOwner | fact.Waited
			},
		})
		if result.Proven() {
			return ssaflow.EvidenceProven
		}
		if abandoned(result) {
			state = ssaflow.EvidenceUnknown
		}
	}
	return state
}
