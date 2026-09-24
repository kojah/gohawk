package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// FieldCall asks about the storage at one embedded field path, not every field
// of its owner. Calls through another field cannot overwrite this slot. Loading
// the slot reads it; effects on its pointee are deliberately a separate query.
// Paths are rebased through visible calls and share the ordinary work budget.
// As with Call, this accounts for uses through the supplied address, not hidden
// aliases. A storage-preservation consumer must separately exclude owner escape.
func (query *CallEffects) FieldCall(instruction ssa.Instruction, path EmbeddedFieldPath) CallEffectProof {
	if path.Depth < 0 || path.Depth > len(path.Fields) {
		return query.proof(effectUnknown)
	}
	if query.fields == nil {
		query.fields = NewCallGraphMemo[EmbeddedFieldPath, CallEffect]()
	}
	return query.proof(query.fieldCall(instruction, path))
}

// PreservesField proves that a complete field query found no write or address
// escape. A known asynchronous reader does not replace the slot. This is NOT
// a no-race, no-blocking, or no-effect guarantee about the loaded resource.
func (proof CallEffectProof) PreservesField() bool {
	return proof.Proven() && proof.Effects & ^(EffectRead|EffectAsync) == 0
}

func (query *CallEffects) fieldCall(instruction ssa.Instruction, path EmbeddedFieldPath) CallEffect {
	common := InstructionCall(instruction)
	if common == nil || path.Root == nil || !query.budget.Spend() {
		return effectUnknown
	}
	function, closure := DirectCallee(common)
	if function == nil {
		return effectUnknown
	}
	var effects CallEffect
	if _, launched := instruction.(*ssa.Go); launched {
		effects |= EffectAsync
	}
	found := false
	for _, binding := range CallBindings(common, function, closure) {
		if binding.Supplied == path.Root {
			found = true
			bound := path
			bound.Root = binding.Local
			effects |= query.fieldValue(bound)
		}
	}
	if !found {
		return effects | effectUnknown
	}
	return effects
}

func (query *CallEffects) fieldValue(path EmbeddedFieldPath) CallEffect {
	return query.fields.Summarize(path, path.Root.Parent(), query.budget, func() CallEffect {
		return query.fieldUses(path)
	}, func(_ SummaryUnavailable, partial CallEffect) CallEffect { return partial | effectUnknown })
}

func (query *CallEffects) fieldUses(path EmbeddedFieldPath) CallEffect {
	if path.Root.Referrers() == nil {
		return effectUnknown
	}
	var effects CallEffect
	for _, use := range *path.Root.Referrers() {
		if !query.budget.Spend() {
			return effects | effectUnknown
		}
		effects |= query.fieldUse(path, use)
	}
	return effects
}

// The projection walk narrows the path only at real embedded field addresses.
// It never follows a loaded pointer as if its pointee were the original slot,
// nor treats a whole-owner store or publication as unrelated to a child field.
func (query *CallEffects) fieldUse(path EmbeddedFieldPath, use ssa.Instruction) CallEffect {
	switch use := use.(type) {
	case *ssa.DebugRef:
		return 0
	case *ssa.FieldAddr:
		if path.Depth > 0 && use.Field != path.Fields[0] {
			return 0
		}
		child := EmbeddedFieldPath{Root: use}
		if path.Depth > 0 {
			child.Depth = path.Depth - 1
			copy(child.Fields[:], path.Fields[1:path.Depth])
		}
		return query.fieldUses(child)
	case *ssa.UnOp:
		if use.Op == token.MUL {
			return EffectRead
		}
	case *ssa.Store:
		if use.Addr == path.Root {
			return EffectMutate
		}
		return EffectRetain
	case *ssa.Call, *ssa.Go, *ssa.Defer:
		return query.fieldCall(use, path)
	}
	return effectUnknown
}
