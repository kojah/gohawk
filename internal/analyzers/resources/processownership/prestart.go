package processownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// All pre-Start policies consume one completed structural instruction census.
// A cutoff discards its prefix, so no owner registration or caller store can be
// inferred from shortened discovery. Argument/result visits share its allowance;
// heap, type and symbol-query internals retain their independent costs.
type processStartInstructions struct {
	proofs.Proof
	instructions []ssa.Instruction
	owners       []ssa.Value
}

func collectProcessStartInstructions(start *ssa.Call, command ssa.Value, budget *proofs.SearchBudget) processStartInstructions {
	if start == nil || start.Block() == nil || start.Parent() == nil {
		return processStartInstructions{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	var before []ssa.Instruction
	for instruction := range cfg.InstructionsStrictlyDominatingWithin(start, budget) {
		before = append(before, instruction)
	}
	owners := processOwnersRegisteredBefore(before, command, budget)
	if budget.Exhausted() {
		return processStartInstructions{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}}
	}
	return processStartInstructions{
		Proof:        proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceStructuralWalk},
		instructions: before, owners: owners,
	}
}

// Pre-start ownership is accepted only when cleanup registration dominates
// Start. Wrapper owners additionally need a later watcher that captures the
// same owner; a deferred method alone does not prove the process is observed.
func processOwnerDominatesStart(
	proof *commandProof,
	function *ssa.Function,
	start *ssa.Call,
	owners []ssa.Value, before []ssa.Instruction,
) bool {
	for _, instruction := range before {
		// Only deferred launches can register cleanup for this caller's
		// return. A helper's own defer completes before that helper returns;
		// a goroutine launch is not a deferred registration. Keep the exact
		// testing.Cleanup contract, which the completion engine also defers.
		_, deferred := instruction.(*ssa.Defer)
		call, called := instruction.(*ssa.Call)
		registered := called && ssacall.HasLibraryContract(call.Common(), ssacall.ContractTestingCleanup)
		if !deferred && !registered {
			continue
		}
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
			if result.Proven() && result.Reason == proofs.EvidenceDeferredCompletion {
				watcher := laterProcessOwnerWatcher(function, start, owners, proof.budget())
				return watcher.State != proofs.EvidenceDisproven
			}
		}
	}
	return false
}

func laterProcessOwnerWatcher(function *ssa.Function, start *ssa.Call, owners []ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		spawn, ok := instruction.(*ssa.Go)
		if !ok || spawn.Pos() <= start.Pos() {
			continue
		}
		closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
		if closure == nil {
			continue
		}
		for _, owner := range owners {
			result := lifecycle.ProveMayContainValueWithin(closure, owner, budget)
			if result.Proven() || result.Reason == proofs.EvidenceBudgetExhausted {
				return result
			}
		}
	}
	if budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}

// An interrupted reachability query leaves ownership unknown; it must not
// become a guarantee that the successful Start branch has no normal return.
func successfulStartCannotReturn(start *ssa.Call, budget *proofs.SearchBudget) proofs.Proof {
	block := start.Block()
	for _, successor := range block.Succs {
		if !budget.Spend() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
		success, known := ssapath.SuccessBranchWithin(block, successor, start, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
		if known && success {
			result := ssapath.ProveNormalReturnWithin(successor, nil, budget).Proof
			switch result.State {
			case proofs.EvidenceProven:
				result.State = proofs.EvidenceDisproven
			case proofs.EvidenceDisproven:
				result.State = proofs.EvidenceProven
			case proofs.EvidenceUnknown:
				return result
			}
			return result
		}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}

// Registering command cleanup before Start is sufficient only on a dominating
// path. A later registration cannot protect an early successful return, so it
// remains part of the ordinary post-Start flow proof instead.
func processOwnershipDominatesStart(
	proof *commandProof,
	before []ssa.Instruction,
	command ssa.Value,
) bool {
	for _, instruction := range before {
		if handoff := possiblePreStartResultlessHandoff(proof, instruction, command); handoff.State == proofs.EvidenceUnknown {
			return true
		}
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
			result.Proven() && (result.Reason == proofs.EvidenceDeferredCompletion || result.Reason == proofs.EvidenceCapturedByClosure) {
			return true
		}
	}
	return false
}

// A non-reference result cannot be a wrapper owner, but its helper can still
// retain or asynchronously expose the command. Such effects leave ownership
// unknown; ordinary reads and configuration writes do not transfer it. Calls
// with reference results retain the existing wrapper-owner proof above.
func possiblePreStartResultlessHandoff(proof *commandProof, instruction ssa.Instruction, command ssa.Value) proofs.Proof {
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	call, ok := instruction.(*ssa.Call)
	if !ok || heapmodel.CanHoldReference(call.Type()) {
		return missing
	}
	budget := proof.budget()
	for _, argument := range call.Common().Args {
		if !budget.Spend() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
		if !heapmodel.MayAlias(argument, command) {
			continue
		}
		effects := proof.evidence.CallEffectsWithin(call, argument, budget)
		if !effects.Proven() || effects.Effects&(ssacall.EffectRetain|ssacall.EffectAsync) != 0 {
			effects.State = proofs.EvidenceUnknown
			return effects.Proof
		}
	}
	return missing
}

func processOwnersRegisteredBefore(before []ssa.Instruction, command ssa.Value, budget *proofs.SearchBudget) []ssa.Value {
	var owners []ssa.Value
	for _, instruction := range before {
		if !budget.Spend() {
			return nil
		}
		call, ok := instruction.(*ssa.Call)
		if !ok || call.Common().StaticCallee() == nil {
			continue
		}
		// Only returned reference storage can be a wrapper owner. A void or
		// scalar result cannot retain the command, even when its call uses it.
		// Instruction-level cleanup and transfers are checked separately.
		// https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/mcp/stdio.go#L135
		if !heapmodel.CanHoldReference(call.Type()) {
			continue
		}
		for _, argument := range call.Common().Args {
			if !budget.Spend() {
				return nil
			}
			if heapmodel.MayAlias(argument, command) {
				owners = append(owners, call)
				owners = appendRegisteredOwnerResults(owners, call, budget)
				if budget.Exhausted() {
					return nil
				}
				break
			}
		}
	}
	return owners
}

// Result projections preserve the same owner candidate at each tuple position.
// Interrupted reference discovery discards the caller's accumulated owner set.
func appendRegisteredOwnerResults(owners []ssa.Value, call *ssa.Call, budget *proofs.SearchBudget) []ssa.Value {
	references := call.Referrers()
	if references == nil {
		return owners
	}
	for _, reference := range *references {
		if !budget.Spend() {
			return nil
		}
		if result, ok := reference.(*ssa.Extract); ok && heapmodel.CanHoldReference(result.Type()) {
			owners = append(owners, result)
		}
	}
	return owners
}

// proveProcessStart owns the ordered pre-Start suppression policy. Proven means
// a local wait obligation proceeds to flow analysis, not a proven violation.
// Possible registration or external ownership remains unknown; only a proven
// absence of successful normal returns yields an accepted final decision.
func proveProcessStart(proof *commandProof, function *ssa.Function, start *ssa.Call, command ssa.Value) processDecision {
	prefix := collectProcessStartInstructions(start, command, proof.budget())
	if !prefix.Proven() {
		reason := reasonPreStartEvidenceUnavailable
		if prefix.Reason == proofs.EvidenceBudgetExhausted {
			reason = reasonPreStartCutoff
		}
		return processDecision{proofs.EvidenceUnknown, reason}
	}
	owners := prefix.owners
	// A helper returning *exec.Cmd may already have registered cleanup
	// or wait ownership. Without interprocedural evidence either way,
	// reporting here would trade precision for recall. containerd wraps
	// command construction and returns the started command in binaryIO:
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/cmd/containerd-shim-runc-v2/process/io.go#L288-L330
	if commandReturnedByHelper(command) {
		return processDecision{proofs.EvidenceUnknown, reasonHelperOwnershipUnknown}
	}
	// Caller retains a parameter command after this helper returns, so
	// helper-local Start does not transfer caller's Wait responsibility.
	if heapmodel.MayAliasAny(command, parameterValues(function.Params)) || ssaflow.ExternallyOwnedValue(command) {
		return processDecision{proofs.EvidenceUnknown, reasonCallerCommandOwnershipUnknown}
	}
	// A command loaded from an element of an aggregate is shared with
	// every other reader of that aggregate, which may wait on it through
	// a different element load the flow cannot link back. cocoon starts
	// worker commands from one loop over a slice and waits in another:
	// https://github.com/cocoonstack/cocoon/blob/51ff88bcf8f175a2d82b162d9bf9f65604a607b5/cmd/storebench/main.go#L123-L138
	if ssaflow.ElementOfAggregate(command) {
		return processDecision{proofs.EvidenceUnknown, reasonAggregateCommandOwnershipUnknown}
	}
	// Cleanup may be registered before Start. This is common when a
	// constructor builds a teardown closure first, then starts the
	// process and returns that closure to its caller.
	if processOwnershipDominatesStart(proof, prefix.instructions, command) ||
		processOwnerDominatesStart(proof, function, start, owners, prefix.instructions) ||
		commandStoredExternallyBeforeStart(prefix.instructions, command) {
		return processDecision{proofs.EvidenceUnknown, reasonPreStartOwnershipUnknown}
	}
	returns := successfulStartCannotReturn(start, proof.budget())
	if returns.Reason == proofs.EvidenceBudgetExhausted {
		return processDecision{proofs.EvidenceUnknown, reasonPreStartCutoff}
	}
	if returns.State == proofs.EvidenceProven {
		return processDecision{proofs.EvidenceDisproven, reasonSuccessfulStartCannotReturn}
	}
	if returns.State == proofs.EvidenceUnknown {
		return processDecision{proofs.EvidenceUnknown, reasonPreStartEvidenceUnavailable}
	}
	return processDecision{proofs.EvidenceProven, reasonLocalWaitObligation}
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
