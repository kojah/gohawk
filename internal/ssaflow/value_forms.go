package ssaflow

import (
	"go/token"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// Transparent value forms are the SSA wrappers a proof may choose to see
// through. Each caller names the forms that are sound for its question; an
// unfamiliar or ambiguous transformation ends the proof instead of guessing
// equivalence. Derivation, access paths, and call results that build on these
// forms live in their own files.

// TransparentValueForm identifies an SSA wrapper that a caller has chosen to
// treat as preserving the evidence it is following. Convert is deliberately
// opt-in because it may change a value's representation or meaning.
type TransparentValueForm uint8

// TransparentNone is the empty set of forms: it follows no wrapper. A proof
// that must see a value exactly as it was written names it rather than
// passing a bare zero.
const TransparentNone TransparentValueForm = 0

const (
	TransparentChangeInterface TransparentValueForm = 1 << iota
	TransparentChangeType
	TransparentConvert
	TransparentMakeInterface
	// TransparentTypeAssert unwraps a type assertion. The asserted value is
	// the same object seen through a different static type, so a proof about
	// identity or ownership may follow it. A proof about what a value can do,
	// such as whether a method is reachable, must not: the assertion is the
	// point where that changes.
	TransparentTypeAssert
)

// UnwrapTransparentValue returns the operand of value only when its concrete
// SSA form is among forms. There is intentionally no catch-all form: each
// analysis must select the transformations that preserve its own evidence.
func UnwrapTransparentValue(value ssa.Value, forms TransparentValueForm) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return transparentOperand(typed.X, forms, TransparentChangeInterface)
	case *ssa.ChangeType:
		return transparentOperand(typed.X, forms, TransparentChangeType)
	case *ssa.Convert:
		return transparentOperand(typed.X, forms, TransparentConvert)
	case *ssa.MakeInterface:
		return transparentOperand(typed.X, forms, TransparentMakeInterface)
	case *ssa.TypeAssert:
		if typed.CommaOk {
			// The comma-ok form yields a tuple; the caller reaches the asserted
			// value through the Extract that selects element zero.
			return nil, false
		}
		return transparentOperand(typed.X, forms, TransparentTypeAssert)
	case *ssa.Extract:
		assertion, ok := typed.Tuple.(*ssa.TypeAssert)
		if !ok || typed.Index != 0 {
			return nil, false
		}
		return transparentOperand(assertion.X, forms, TransparentTypeAssert)
	default:
		return nil, false
	}
}

// ForwardedValue reports the value a reference produces when it only carries
// its operand onward: a wrapper conversion, a tuple extraction, or a phi that
// merges it. It is the forward companion of UnwrapTransparentValue, which
// peels a value back to the operand it came from. A walk over referrers needs
// the step in this direction, and writing it out at each such walk is how two
// of them came to spell the same set differently.
//
// The set is fixed rather than selected, because a forward walk asks where a
// value ends up rather than what evidence survives a wrapper: every form here
// carries the same value onward, and a caller that must stop at one of them is
// asking a different question.
func ForwardedValue(reference ssa.Instruction) (ssa.Value, bool) {
	switch reference.(type) {
	case *ssa.ChangeInterface, *ssa.ChangeType, *ssa.Convert, *ssa.Extract, *ssa.MakeInterface, *ssa.Phi:
		value, ok := reference.(ssa.Value)
		return value, ok
	}
	return nil, false
}

func transparentOperand(operand ssa.Value, forms, form TransparentValueForm) (ssa.Value, bool) {
	if forms&form == 0 {
		return nil, false
	}
	return operand, true
}

// BooleanNegationSource returns the operand behind a chain of SSA Boolean NOT
// instructions and whether an odd number of negations reverses its truth. It
// stops at every other form, including loads, conversions, comparisons and phi
// merges; it neither evaluates the operand nor establishes its stability.
func BooleanNegationSource(value ssa.Value) (ssa.Value, bool) {
	return BooleanNegationSourceWithin(value, nil)
}

// BooleanNegationSourceWithin charges each NOT step to budget. Cutoff returns
// no operand evidence; callers retain the exhaustion state.
func BooleanNegationSourceWithin(value ssa.Value, budget *proofs.SearchBudget) (ssa.Value, bool) {
	negated := false
	for {
		if !budget.Spend() {
			return nil, false
		}
		not, ok := value.(*ssa.UnOp)
		if !ok || not.Op != token.NOT {
			return value, negated
		}
		value, negated = not.X, !negated
	}
}
