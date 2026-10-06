package lifecyclefacts

import (
	"go/token"
	"strconv"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// CallEffectsWithin exposes local call-effect evidence beside lifecycle evidence.
// Effects are possible uses, never proof of cleanup or ownership transfer;
// an absent Retained bit is not a read-only contract. Imported bodies stay unknown.
// Existing visits share budget while retaining the local QueryBudget cap. Cutoff
// is unknown even if the caller has allowance left. Graph/alias/type and binding
// construction costs remain separate. Nil selects an independent default allowance
// through the same authoritative trace path.
func (evidence *LifecycleEvidence) CallEffectsWithin(
	instruction ssa.Instruction, target ssa.Value, budget *proofs.SearchBudget,
) ssaflow.CallEffectProof {
	queryBudget := budget.Within(proofs.QueryBudget).Observed(evidence.probe.Observer())
	proof := ssaflow.NewCallEffects(queryBudget).Call(instruction, target)
	if !evidence.probe.Enabled() {
		return proof
	}
	details := evidenceDetails(instruction, target)
	for name, effect := range map[string]ssaflow.CallEffect{
		"read": ssaflow.EffectRead, "mutate": ssaflow.EffectMutate,
		"retain": ssaflow.EffectRetain, "async": ssaflow.EffectAsync, "invoke": ssaflow.EffectInvoke,
	} {
		details[name] = strconv.FormatBool(proof.Effects&effect != 0)
	}
	outcome := analysisTrace.OutcomeUnknown
	if proof.Proven() {
		outcome = analysisTrace.OutcomeAccepted
	}
	position := token.NoPos
	if instruction != nil {
		position = instruction.Pos()
	}
	evidence.probe.Evidence(analysisTrace.Step{
		Reason: proof.Reason.String(), Outcome: outcome, Pos: position,
		Function: instructionFunction(instruction), Details: details,
	})
	return proof
}
