package lifecyclefacts

import (
	"go/token"
	"strconv"

	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

// CallEffects exposes local call-effect evidence beside lifecycle evidence,
// without treating an absent Retained bit as a read-only contract. Effects are
// possible uses, never proof of cleanup or ownership transfer. Imported bodies
// remain unknown until a dedicated effect summary can establish their safety.
func (evidence *LifecycleEvidence) CallEffects(instruction ssa.Instruction, target ssa.Value) ssaflow.CallEffectProof {
	proof := ssaflow.NewCallEffects(ssaflow.NewSearchBudget(1000)).Call(instruction, target)
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
		Reason: string(proof.Reason), Outcome: outcome, Pos: position,
		Function: instructionFunction(instruction), Details: details,
	})
	return proof
}
