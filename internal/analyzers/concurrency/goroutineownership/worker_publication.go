package goroutineownership

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Worker publication guards prevent an I/O or context shutdown witness from
// hiding later blocking output. Within variants charge the caller's census;
// their false result is usable only when that allowance remains available.

// An opaque helper receiving a send-capable channel can publish after its
// context is canceled. Capacity does not bound its send count. Badwolf's storage
// implementation ignores cancellation while streaming results, so the caller's
// canceled-context arm must not hide the abandoned producer behind the helper.
// https://github.com/google/badwolf/blob/6cde56dbc7db828597ea856db6c3e1f331e70916/storage/memoization/memoization.go#L194-L222
func workerHandsOffOutputChannelWithin(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			continue
		}
		common := call.Common()
		if _, builtin := common.Value.(*ssa.Builtin); builtin {
			continue
		}
		if callee := common.StaticCallee(); callee != nil && len(callee.Blocks) > 0 {
			continue
		}
		for _, argument := range common.Args {
			if !budget.Spend() {
				return false
			}
			channel, ok := argument.Type().Underlying().(*types.Chan)
			if !ok || channel.Dir() == types.RecvOnly {
				continue
			}
			disabled := ssaflow.DefinitelyNilWithin(argument, budget)
			if budget.Exhausted() {
				return false
			}
			if !disabled {
				return true
			}
		}
	}
	return false
}

// Releasing an I/O operation cannot settle a subsequent blocking publication.
// Keep the existing completion proof authoritative for workers with sends.
// https://github.com/pterodactyl/wings/blob/d6116827313dae176ddf4741e233554392993398/server/transfer/source.go#L87-L96
func workerHasSendWithin(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		if _, send := instruction.(*ssa.Send); send {
			return true
		}
		if choice, ok := instruction.(*ssa.Select); ok && slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
			return budget.Spend() && state.Dir == types.SendOnly
		}) {
			return true
		}
	}
	return false
}
