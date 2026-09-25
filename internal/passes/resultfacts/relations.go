package resultfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Relations tie one result to a parameter or to another result. They carry
// the two implications lifecycle proofs keep needing from a helper: a
// Boolean predicate that reports an error's nilness, and a value that is
// present exactly when its paired error is absent. Each relation is proven
// over every normal return under its assumption and is exported beside the
// unconditional guarantees. A missing relation says nothing; it never
// establishes that the opposite implication holds.

// RelationKind names one implication. Parameter kinds constrain a Boolean
// result by a nilable parameter; result kinds constrain a nilable result by
// an error result of the same call.
type RelationKind uint8

const (
	// FalseWhenParameterNil: the Boolean result is false on every return
	// reachable when the parameter is nil. A predicate such as failed(err)
	// then cannot take its true branch for a successful acquisition.
	FalseWhenParameterNil RelationKind = iota + 1
	// TrueWhenParameterNonNil: the Boolean result is true on every return
	// reachable when the parameter is non-nil, so the false branch of the
	// predicate is the branch where the parameter was nil.
	TrueWhenParameterNonNil
	// NonNilWhenResultNil: the result is non-nil on every return where the
	// operand error result is nil.
	NonNilWhenResultNil
	// NilWhenResultNonNil: the result is nil on every return where the
	// operand error result is non-nil.
	NilWhenResultNonNil
	// ReturnsParameter: the result is the operand parameter itself, under the
	// same static type, on every normal return. A builder returning its
	// receiver and a pass-through wrapper have this shape; a caller may then
	// treat the result as the argument it passed.
	ReturnsParameter
)

// Relation is one proven implication about Result. Operand is a parameter
// index for the parameter kinds and a result index for the result kinds.
type Relation struct {
	Result  int
	Kind    RelationKind
	Operand int
}

// Holds reports whether the summary proved the relation.
func (summary Summary) Holds(kind RelationKind, result, operand int) bool {
	for _, relation := range summary.relations {
		if relation.Kind == kind && relation.Result == result && relation.Operand == operand {
			return true
		}
	}
	return false
}

// Relations returns every proven relation.
func (summary Summary) Relations() []Relation {
	return summary.relations
}

func (engine *Engine) relations(function *ssa.Function, budget *ssaflow.SearchBudget) []Relation {
	var relations []Relation
	results := function.Signature.Results()
	for result := range results.Len() {
		resultType := results.At(result).Type()
		for index, parameter := range function.Params {
			if budget.Spend() && types.Identical(parameter.Type(), resultType) && lifecycle.ReturnsParameterUnchanged(function, parameter, result) {
				relations = append(relations, Relation{Result: result, Kind: ReturnsParameter, Operand: index})
			}
		}
		if isBoolean(resultType) {
			for index, parameter := range function.Params {
				if !nilable(parameter.Type()) {
					continue
				}
				if engine.parameterRelation(function, result, parameter, true, AlwaysFalse, budget) {
					relations = append(relations, Relation{Result: result, Kind: FalseWhenParameterNil, Operand: index})
				}
				if engine.parameterRelation(function, result, parameter, false, AlwaysTrue, budget) {
					relations = append(relations, Relation{Result: result, Kind: TrueWhenParameterNonNil, Operand: index})
				}
			}
		}
		if !nilable(resultType) {
			continue
		}
		for operand := range results.Len() {
			if operand == result || !types.Identical(results.At(operand).Type(), types.Universe.Lookup("error").Type()) {
				continue
			}
			if engine.resultRelation(function, result, operand, NonNilWhenResultNil, budget) {
				relations = append(relations, Relation{Result: result, Kind: NonNilWhenResultNil, Operand: operand})
			}
			if engine.resultRelation(function, result, operand, NilWhenResultNonNil, budget) {
				relations = append(relations, Relation{Result: result, Kind: NilWhenResultNonNil, Operand: operand})
			}
		}
	}
	return relations
}

// parameterRelation walks the body under the assumption that parameter is
// nil (or non-nil) and requires the result to be the expected literal on
// every reachable normal return. Only a comparison of the exact formal prunes
// a branch: a wrapped error may be non-nil when its source is nil, and a
// spilled cell may have been reassigned.
// https://github.com/norwoodj/helm-docs/blob/a5573af096a4b526dcbc3c896c220b1714a0765b/pkg/helm/chart_info.go#L94-L106
func (engine *Engine) parameterRelation(
	function *ssa.Function, result int, parameter *ssa.Parameter, assumeNil bool, expected Guarantee, budget *ssaflow.SearchBudget,
) bool {
	valid, witness := true, false
	ssaflow.WalkStates([]*ssa.BasicBlock{function.Blocks[0]}, func(block *ssa.BasicBlock) *ssa.BasicBlock { return block },
		func(block *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					valid = false
					return nil, false
				}
				returned, ok := instruction.(*ssa.Return)
				if !ok {
					continue
				}
				witness = true
				if result >= len(returned.Results) || engine.assumedValue(returned.Results[result], parameter, assumeNil, budget) != expected {
					valid = false
					return nil, false
				}
			}
			return assumedNilnessSuccessors(block, parameter, assumeNil), true
		})
	return valid && witness && !budget.Exhausted()
}

// assumedValue resolves a Boolean result under the assumed nilness of the
// exact parameter: a comparison of that parameter with nil is decided by the
// assumption, and anything else falls back to the unconditional guarantee.
func (engine *Engine) assumedValue(value ssa.Value, parameter *ssa.Parameter, assumeNil bool, budget *ssaflow.SearchBudget) Guarantee {
	result, ok := ssaflow.ResolveReachingValue(
		ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		func(_ ssaflow.ReachingWalk, leaf ssa.Value) (Guarantee, bool) {
			if guarantee, decided := comparisonUnderAssumption(leaf, parameter, assumeNil); decided {
				return guarantee, true
			}
			guarantee := engine.leaf(leaf, budget)
			return guarantee, guarantee != Unknown
		},
		func(guarantee Guarantee) Guarantee { return guarantee },
	)
	if !ok || budget.Exhausted() {
		return Unknown
	}
	return result
}

func comparisonUnderAssumption(value ssa.Value, parameter *ssa.Parameter, assumeNil bool) (Guarantee, bool) {
	comparison, ok := value.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return Unknown, false
	}
	comparesNil := comparison.X == parameter && ssaflow.DefinitelyNil(comparison.Y) ||
		comparison.Y == parameter && ssaflow.DefinitelyNil(comparison.X)
	if !comparesNil {
		return Unknown, false
	}
	equal := assumeNil
	if comparison.Op == token.NEQ {
		equal = !equal
	}
	if equal {
		return AlwaysTrue, true
	}
	return AlwaysFalse, true
}

// assumedNilnessSuccessors keeps only the successor consistent with the
// assumed nilness of the exact parameter, when the block branches on it.
func assumedNilnessSuccessors(block *ssa.BasicBlock, parameter *ssa.Parameter, assumeNil bool) []*ssa.BasicBlock {
	if len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return block.Succs
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return block.Succs
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.X != parameter && comparison.Y != parameter {
		return block.Succs
	}
	for _, successor := range block.Succs {
		if isNil, known := ssaflow.SuccessBranch(block, successor, parameter); known && isNil == assumeNil {
			return []*ssa.BasicBlock{successor}
		}
	}
	return block.Succs
}

// resultRelation checks every normal return: where the error operand is
// known nil the result must be known non-nil, and where it is known non-nil
// the result must be known nil, depending on kind. A return that forwards
// both positions from one call inherits that callee's relation. A return
// whose error nilness is unknown proves nothing, and at least one return
// must witness the assumed side.
func (engine *Engine) resultRelation(function *ssa.Function, result, operand int, kind RelationKind, budget *ssaflow.SearchBudget) bool {
	witness := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok || result >= len(returned.Results) || operand >= len(returned.Results) {
				continue
			}
			holds, witnessed := engine.returnHolds(returned.Results[result], returned.Results[operand], kind, budget)
			if !holds {
				return false
			}
			witness = witness || witnessed
		}
	}
	return witness && !budget.Exhausted()
}

// returnHolds judges one return: whether it keeps the relation, and whether it
// witnesses it rather than holding only vacuously, as a return whose error is
// nil does for a claim about non-nil errors.
func (engine *Engine) returnHolds(resultValue, errorValue ssa.Value, kind RelationKind, budget *ssaflow.SearchBudget) (bool, bool) {
	switch engine.value(errorValue, budget) {
	case AlwaysNil:
		if kind == NonNilWhenResultNil {
			return engine.value(resultValue, budget) == AlwaysNonNil, true
		}
		return true, false
	case AlwaysNonNil:
		if kind == NilWhenResultNonNil {
			return engine.value(resultValue, budget) == AlwaysNil, true
		}
		return true, false
	case Unknown, AlwaysTrue, AlwaysFalse:
		// An error of unknown nilness is decided below by the result alone
		// or by a forwarded pair.
	}
	// A return whose result already has the implied nilness holds the
	// implication whatever its error: return nil, err after a failed call is
	// nil where the error is non-nil.
	if resultSatisfies(engine.value(resultValue, budget), kind) {
		return true, true
	}
	callee, resultIndex, operandIndex, forwarded := forwardedPair(resultValue, errorValue)
	if !forwarded || !engine.function(callee, budget).Holds(kind, resultIndex, operandIndex) {
		return false, false
	}
	return true, true
}

// resultSatisfies reports whether a result of this guarantee meets the
// relation's consequent on its own.
func resultSatisfies(guarantee Guarantee, kind RelationKind) bool {
	return kind == NilWhenResultNonNil && guarantee == AlwaysNil || kind == NonNilWhenResultNil && guarantee == AlwaysNonNil
}

// forwardedPair resolves a return that passes two results of one call
// through unchanged, so the callee's relation between them carries over.
func forwardedPair(resultValue, errorValue ssa.Value) (*ssa.Function, int, int, bool) {
	resultCall, resultIndex, ok := ssaflow.CallResultSource(resultValue)
	if !ok {
		return nil, 0, 0, false
	}
	errorCall, errorIndex, ok := ssaflow.CallResultSource(errorValue)
	if !ok || errorCall != resultCall {
		return nil, 0, 0, false
	}
	callee := ssaflow.ResolvedCallee(resultCall.Common())
	return callee, resultIndex, errorIndex, callee != nil
}

func isBoolean(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func nilable(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan, *types.Signature:
		return true
	}
	return false
}
