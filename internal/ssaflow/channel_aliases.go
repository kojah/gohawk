package ssaflow

import "golang.org/x/tools/go/ssa"

// Channel alias discovery follows direction conversions, initialized cell
// reads, lexical captures and static call arguments. Uses outside those forms
// remain visible to the consumer; uncertain initialization and interrupted
// traversal cannot publish an absence-of-use claim.

// ChannelUse is an instruction consuming a channel value rather than moving
// it through one of the modeled aliases.
type ChannelUse struct {
	Value       ssa.Value
	Instruction ssa.Instruction
}

// ChannelValuesProof publishes a complete census of modeled aliases and uses.
// Callee parameters may also receive other values at other call sites; this
// census does not establish an exclusive channel identity or execution path.
type ChannelValuesProof struct {
	Proof
	Values []ssa.Value
	Uses   []ChannelUse
}

// ProveChannelValuesWithin follows the locally made channel through static
// callees and read-only captures under budget. Unsupported moves remain uses.
// Reads preceding the unique store are excluded; uncertain read/capture order
// or budget cutoff publishes neither aliases nor uses. Nil budget is unbounded.
func ProveChannelValuesWithin(made *ssa.MakeChan, budget *SearchBudget) ChannelValuesProof {
	unknown := ChannelValuesProof{Proof: Proof{Reason: EvidenceUnavailable}}
	if made == nil {
		return unknown
	}
	values := []ssa.Value{made}
	member := map[ssa.Value]bool{made: true}
	pending := []ssa.Value{made}
	var uses []ChannelUse
	for len(pending) > 0 {
		if !budget.Spend() {
			return ChannelValuesProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
		}
		value := pending[0]
		pending = pending[1:]
		for user := range ReferrersWithin(value, budget) {
			moved, available := channelMoveWithin(value, user, budget)
			if budget.Exhausted() || budget.PoolExhausted() {
				return ChannelValuesProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
			}
			if !available {
				return unknown
			}
			if moved == nil {
				uses = append(uses, ChannelUse{Value: value, Instruction: user})
				continue
			}
			for _, target := range moved {
				if !budget.Spend() {
					return ChannelValuesProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
				}
				if target != nil && !member[target] {
					member[target] = true
					values = append(values, target)
					pending = append(pending, target)
				}
			}
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return ChannelValuesProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
	}
	return ChannelValuesProof{Proof: Proof{State: EvidenceProven, Reason: EvidenceStructuralWalk, Provenance: EvidenceFromLocalSSA}, Values: values, Uses: uses}
}

// A nil move is an opaque use. False availability means the census cannot
// even establish which reads may carry the channel.
func channelMoveWithin(value ssa.Value, user ssa.Instruction, budget *SearchBudget) ([]ssa.Value, bool) {
	switch typed := user.(type) {
	case *ssa.ChangeType:
		return []ssa.Value{typed}, true
	case *ssa.Store:
		cell, ok := typed.Addr.(*ssa.Alloc)
		if !ok || typed.Val != value {
			return nil, true
		}
		store, once := writtenOnceStoreWithin(cell, budget)
		if !once || store.Val != value {
			return nil, true
		}
		return cellCopiesWithin(cell, store, budget)
	case *ssa.Call, *ssa.Go, *ssa.Defer:
		return argumentParametersWithin(value, InstructionCall(user), budget), true
	}
	return nil, true
}

// Acyclic reads before the store are snapshots of the old cell. A cyclic
// read can see an earlier iteration's store, so it remains unavailable.
// Captures require initialization before creation; execution after a later
// store is not inferred from mere registration. Unsupported nested uses keep
// the store opaque, preserving the bounded lexical traversal policy.
func cellCopiesWithin(cell *ssa.Alloc, store *ssa.Store, budget *SearchBudget) ([]ssa.Value, bool) {
	var copies []ssa.Value
	for user := range ReferrersWithin(cell, budget) {
		switch typed := user.(type) {
		case *ssa.Store:
		case *ssa.UnOp:
			if InstructionDominatesWithin(store, typed, budget) {
				copies = append(copies, typed)
			} else if !InstructionDominatesWithin(typed, store, budget) || BlockInCycleWithin(typed.Block(), budget) {
				return nil, false
			}
		case *ssa.MakeClosure:
			if !InstructionDominatesWithin(store, typed, budget) {
				return nil, false
			}
			function, ok := typed.Fn.(*ssa.Function)
			if !ok {
				return nil, true
			}
			for pair := range ClosureBindingPairsWithin(function, typed, budget) {
				if pair.Binding != cell {
					continue
				}
				for load := range ReferrersWithin(pair.Free, budget) {
					if unop, ok := load.(*ssa.UnOp); ok {
						copies = append(copies, unop)
					} else {
						return nil, true
					}
				}
			}
		default:
			return nil, true
		}
	}
	return copies, true
}

// Only static callees with bodies expose parameter uses. Other calls remain
// opaque uses; metadata alone does not supply a callee-use census.
func argumentParametersWithin(value ssa.Value, common *ssa.CallCommon, budget *SearchBudget) []ssa.Value {
	if common == nil || common.IsInvoke() {
		return nil
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 || len(callee.Params) != len(common.Args) {
		return nil
	}
	var parameters []ssa.Value
	for index, argument := range common.Args {
		if !budget.Spend() {
			return nil
		}
		if argument == value {
			parameters = append(parameters, callee.Params[index])
		}
	}
	return parameters
}
