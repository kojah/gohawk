package processownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// All pre-Start policies consume one completed structural instruction census.
// A cutoff discards its prefix, so no owner registration or caller store can be
// inferred from shortened discovery. Argument/result visits share its allowance;
// heap, type and symbol-query internals retain their independent costs.
type processStartInstructions struct {
	ssaflow.Proof
	instructions []ssa.Instruction
	owners       []ssa.Value
}

func collectProcessStartInstructions(start *ssa.Call, command ssa.Value, budget *ssaflow.SearchBudget) processStartInstructions {
	if start == nil || start.Block() == nil || start.Parent() == nil {
		return processStartInstructions{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	var before []ssa.Instruction
	for instruction := range ssaflow.InstructionsStrictlyDominatingWithin(start, budget) {
		before = append(before, instruction)
	}
	owners := processOwnersRegisteredBefore(before, command, budget)
	if budget.Exhausted() {
		return processStartInstructions{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	return processStartInstructions{
		Proof:        ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk},
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
				watcher := laterProcessOwnerWatcher(function, start, owners, proof.budget())
				return watcher.State != ssaflow.EvidenceDisproven
			}
		}
	}
	return false
}

func laterProcessOwnerWatcher(function *ssa.Function, start *ssa.Call, owners []ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
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
			if result.Proven() || result.Reason == ssaflow.EvidenceBudgetExhausted {
				return result
			}
		}
	}
	if budget.Exhausted() {
		return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
}

// An interrupted reachability query leaves ownership unknown; it must not
// become a guarantee that the successful Start branch has no normal return.
func successfulStartCannotReturn(start *ssa.Call, budget *ssaflow.SearchBudget) ssaflow.Proof {
	block := start.Block()
	for _, successor := range block.Succs {
		if !budget.Spend() {
			return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if success, known := ssaflow.SuccessBranch(block, successor, start); known && success {
			result := ssaflow.ProveNormalReturnWithin(successor, nil, budget).Proof
			switch result.State {
			case ssaflow.EvidenceProven:
				result.State = ssaflow.EvidenceDisproven
			case ssaflow.EvidenceDisproven:
				result.State = ssaflow.EvidenceProven
			case ssaflow.EvidenceUnknown:
				return result
			}
			return result
		}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
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
		if handoff := possiblePreStartResultlessHandoff(proof, instruction, command); handoff.State == ssaflow.EvidenceUnknown {
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
			result.Proven() && (result.Reason == ssaflow.EvidenceDeferredCompletion || result.Reason == ssaflow.EvidenceCapturedByClosure) {
			return true
		}
	}
	return false
}

// A non-reference result cannot be a wrapper owner, but its helper can still
// retain or asynchronously expose the command. Such effects leave ownership
// unknown; ordinary reads and configuration writes do not transfer it. Calls
// with reference results retain the existing wrapper-owner proof above.
func possiblePreStartResultlessHandoff(proof *commandProof, instruction ssa.Instruction, command ssa.Value) ssaflow.Proof {
	missing := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
	call, ok := instruction.(*ssa.Call)
	if !ok || heapmodel.CanHoldReference(call.Type()) {
		return missing
	}
	budget := proof.budget()
	for _, argument := range call.Common().Args {
		if !budget.Spend() {
			return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
		}
		if !heapmodel.MayAlias(argument, command) {
			continue
		}
		effects := proof.evidence.CallEffectsWithin(call, argument, budget)
		if !effects.Proven() || effects.Effects&(ssaflow.EffectRetain|ssaflow.EffectAsync) != 0 {
			effects.State = ssaflow.EvidenceUnknown
			return effects.Proof
		}
	}
	return missing
}

func processOwnersRegisteredBefore(before []ssa.Instruction, command ssa.Value, budget *ssaflow.SearchBudget) []ssa.Value {
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
func appendRegisteredOwnerResults(owners []ssa.Value, call *ssa.Call, budget *ssaflow.SearchBudget) []ssa.Value {
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
