package concurrencyfacts

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

const maxSelectArms = 8

// A select executes exactly one communication, or its default arm. We keep
// every arm explicit and incomplete for linear consumers; flattening them
// would invent an unavoidable wait.
func (engine *Engine) appendSelect(result *Summary, selection *ssa.Select) string {
	if len(selection.States) == 0 || len(selection.States) > maxSelectArms {
		return "protocol-select-alternatives-unknown"
	}
	choice := SelectChoice{Prefix: len(result.Operations), Site: selection.Pos()}
	for _, state := range selection.States {
		kind := Receive
		if state.Dir == types.SendOnly {
			kind = Send
			if state.Send == nil || !scalarType(state.Send.Type()) {
				return "protocol-payload-unknown"
			}
		} else if state.Dir != types.RecvOnly {
			return "protocol-select-alternatives-unknown"
		}
		resource, ok := engine.reference(state.Chan)
		if !ok {
			return "protocol-channel-identity-unknown"
		}
		choice.Arms = append(choice.Arms, SelectArm{Operation: Operation{
			Kind: kind, Resource: resource, Source: state.Pos, Site: selection.Pos(),
		}})
	}
	if !selection.Blocking {
		choice.Arms = append(choice.Arms, SelectArm{Default: true})
	}
	result.Choices = append(result.Choices, choice)
	return "protocol-select-alternatives"
}
