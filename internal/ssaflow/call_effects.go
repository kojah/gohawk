package ssaflow

import (
	"slices"

	"golang.org/x/tools/go/ssa"
)

// CallEffect describes a possible use, not an action guaranteed on every path.
// Effects concern the supplied value and storage selected directly beneath it;
// loading a pointer reads its slot, it does not alias that slot with its pointee.
type CallEffect uint8

const (
	EffectRead CallEffect = 1 << iota
	EffectMutate
	EffectRetain
	EffectAsync
	EffectInvoke
	effectUnknown
)

// CallEffectProof separates discovered effects from completeness. An incomplete
// proof never establishes the absence of mutation or escape. Retention includes
// returns and conservative local spills; it never proves ownership transfer.
type CallEffectProof struct {
	Proof
	Effects CallEffect
}

// PreservesStorage proves no write, retention, asynchronous exposure, or callback
// invocation through this address. Reading alone is safe; a read followed by
// retaining the pointer is not. The zero value and incomplete proofs are unsafe.
func (proof CallEffectProof) PreservesStorage() bool {
	return proof.Proven() && proof.Effects & ^EffectRead == 0
}

// CallEffects owns bounded, memoized evidence for a single SSA query context.
// Bodies must be available. Opaque/dynamic calls, recursion and budget exhaustion
// are unknown, never inferred pure from missing lifecycle-summary bits.
type CallEffects struct {
	budget *SearchBudget
	memo   *CallGraphMemo[ssa.Value, CallEffect]
}

// NewCallEffects constructs a query with a shared instruction budget. A nil
// budget receives the same bounded 1000-step default as local storage queries.
func NewCallEffects(budget *SearchBudget) *CallEffects {
	if budget == nil {
		budget = NewSearchBudget(QueryBudget)
	}
	return &CallEffects{budget: budget, memo: NewCallGraphMemo[ssa.Value, CallEffect]()}
}

// Value summarizes all visible uses of a parameter or captured address. Direct
// projections and wrappers are followed; loaded values are snapshots, not aliases
// of their slots. This is not a transitive heap or lifecycle-effect summary.
func (query *CallEffects) Value(value ssa.Value) CallEffectProof {
	return query.proof(query.value(value))
}

// Call summarizes a call's uses of exactly value, through every matching argument
// and capture. Callers inspecting aliases must query those addresses too. A value
// not directly supplied is unknown rather than assumed untouched.
func (query *CallEffects) Call(instruction ssa.Instruction, value ssa.Value) CallEffectProof {
	return query.proof(query.call(instruction, value))
}

func (query *CallEffects) proof(effects CallEffect) CallEffectProof {
	proof := Proof{State: EvidenceProven, Reason: "call-effects-known", Provenance: EvidenceFromLocalSSA}
	if effects&effectUnknown != 0 {
		proof.State, proof.Reason = EvidenceUnknown, EvidenceUnavailable
	}
	if query.budget == nil || query.budget.Exhausted() {
		proof.State, proof.Reason = EvidenceUnknown, EvidenceBudgetExhausted
	}
	return CallEffectProof{Proof: proof, Effects: effects & ^effectUnknown}
}

func (query *CallEffects) value(value ssa.Value) CallEffect {
	if value == nil || query.budget == nil || !query.budget.Spend() {
		query.memo.Incomplete()
		return effectUnknown
	}
	return query.memo.Summarize(value, value.Parent(), query.budget, func() CallEffect {
		return query.uses(value, make(map[ssa.Value]bool))
	}, func(_ SummaryUnavailable, partial CallEffect) CallEffect {
		// Discovered uses remain useful evidence, but never establish purity
		// when the rest of the summary could not be computed.
		return partial | effectUnknown
	})
}

func (query *CallEffects) call(instruction ssa.Instruction, value ssa.Value) CallEffect {
	common := InstructionCall(instruction)
	if common == nil || value == nil || query.budget == nil || !query.budget.Spend() {
		query.memo.Incomplete()
		return effectUnknown
	}
	var effects CallEffect
	if _, launched := instruction.(*ssa.Go); launched {
		effects |= EffectAsync
	}
	if common.Value == value {
		return effects | EffectInvoke | effectUnknown
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		if !slices.Contains(common.Args, value) {
			return effects | effectUnknown
		}
		return effects | builtinEffects(builtin.Name())
	}
	function, closure := DirectCallee(common)
	if function == nil || len(function.Blocks) == 0 {
		return effects | effectUnknown
	}
	found := false
	for _, binding := range CallBindings(common, function, closure) {
		if binding.Supplied == value {
			found = true
			effects |= query.value(binding.Local)
		}
	}
	if !found {
		effects |= effectUnknown
	}
	return effects
}

func builtinEffects(name string) CallEffect {
	switch name {
	case "len", "cap":
		return EffectRead
	case "copy", "clear", "delete", "close":
		return EffectMutate
	case "append":
		return EffectMutate | EffectRetain
	default:
		return effectUnknown
	}
}
