package processownership

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"golang.org/x/tools/go/ssa"
)

// Candidate-local actions do not depend on the flow's path guards. Repeated
// visits reuse the same instruction/command evidence, including conservative
// unknown cutoffs; a different command or fresh candidate has its own query.
type commandActionKey struct {
	instruction ssa.Instruction
	command     ssa.Value
}

func (proof *commandProof) action(instruction ssa.Instruction, command ssa.Value) proofs.EvidenceState {
	key := commandActionKey{instruction: instruction, command: command}
	if action, ok := proof.actions[key]; ok {
		return action
	}
	action := processOwnershipAction(proof, instruction, command)
	if proof.actions == nil {
		proof.actions = make(map[commandActionKey]proofs.EvidenceState)
	}
	proof.actions[key] = action
	return action
}
