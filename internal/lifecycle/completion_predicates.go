package lifecycle

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// CompletionOutcome names the observed result of a synchronous helper call.
// Zero means unconditional; the other values require the named result to have
// the corresponding Boolean or error-interface value.
type CompletionOutcome uint8

const (
	CompletionAlways CompletionOutcome = iota
	CompletionWhenTrue
	CompletionWhenFalse
	CompletionWhenNil
	CompletionWhenNonNil
)

// CompletionPredicate is a serializable case of a function's behavior: a
// condition on one result, on Boolean arguments fixed to constants, or both.
// A zero Outcome with no Arguments is the unconditional case.
type CompletionPredicate struct {
	Result    int
	Outcome   CompletionOutcome
	Arguments ArgumentConstants
}

// ArgumentConstants names Boolean parameters by position, receiver first,
// and the constant each holds. In a summarized case it is what the case
// assumes; in a query it is what the call supplies. Positions past 63 are
// never bound.
type ArgumentConstants struct {
	Bound  uint64
	Values uint64
}

// Satisfies reports whether the supplied constants fix every argument the
// assumed constants name, to the same value.
func (supplied ArgumentConstants) Satisfies(assumed ArgumentConstants) bool {
	return assumed.Bound&^supplied.Bound == 0 && (assumed.Values^supplied.Values)&assumed.Bound == 0
}

// Matches reports whether a summarized case answers query: constants the
// query's call supplies, and the same result condition. A case with no result
// condition holds on every normal return, so it answers any result condition
// too.
func (summarized CompletionPredicate) Matches(query CompletionPredicate) bool {
	if !query.Arguments.Satisfies(summarized.Arguments) {
		return false
	}
	return summarized.Outcome == CompletionAlways || summarized.Result == query.Result && summarized.Outcome == query.Outcome
}

// argumentConstantsAt reports the Boolean constants a call supplies, including
// caller values the search's own bindings fix.
func argumentConstantsAt(instruction ssa.Instruction, known ssaflow.BooleanConstants) ArgumentConstants {
	bound, values := ssaflow.ConstantBooleanArgumentBits(ssaflow.InstructionCall(instruction), known)
	return ArgumentConstants{Bound: bound, Values: values}
}

// bindings maps the assumed constants onto function's Boolean parameters.
// It reports false when a bound position is not a Boolean parameter.
func (assumed ArgumentConstants) bindings(function *ssa.Function) (ssaflow.BooleanConstants, bool) {
	if assumed.Bound == 0 {
		return nil, true
	}
	constants := ssaflow.BooleanConstants{}
	for index, parameter := range function.Params {
		if index >= 64 || assumed.Bound&(1<<index) == 0 {
			continue
		}
		if basic, ok := parameter.Type().Underlying().(*types.Basic); !ok || basic.Kind() != types.Bool {
			return nil, false
		}
		constants[parameter] = assumed.Values&(1<<index) != 0
	}
	if len(constants) != bitCount(assumed.Bound) {
		return nil, false
	}
	return constants, true
}

func bitCount(bits uint64) int {
	count := 0
	for ; bits != 0; bits &= bits - 1 {
		count++
	}
	return count
}

// CompletionSummaryLookup supplies positive guarantees for unavailable callees.
// The implementation must bind target to an exact argument, distinguish the
// requested method from callback invocation, and match the complete predicate.
// False means no guarantee, never proof that the callee has no effect.
type CompletionSummaryLookup func(ssa.Instruction, ssa.Value, string, bool, CompletionPredicate) bool

// queryAt is the question a summary lookup answers for one call: this
// search's result condition, with the constants the call supplies.
func (search *completionSearch) queryAt(instruction ssa.Instruction) CompletionPredicate {
	return CompletionPredicate{
		Result: search.condition.result, Outcome: CompletionOutcome(search.condition.kind),
		Arguments: argumentConstantsAt(instruction, search.constants),
	}
}

// ProveCompletionForCase summarizes exact parameter cleanup on the normal
// returns of one case: the returns matching the predicate's result condition,
// on the paths feasible when its assumed arguments hold. It reuses the
// completion engine and its shared budget, callback bindings, and recursion
// guard; it does not invent a caller or SSA. Without ExactTarget, a method
// completion may settle a field or element beneath the parameter; the proof
// then names that path, and a caller must not credit a claim whose path is
// not known.
func ProveCompletionForCase(function *ssa.Function, predicate CompletionPredicate, request CompletionRequest) ssaflow.CompletionProof {
	unknown := ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	parameter, ok := request.Target.(*ssa.Parameter)
	if !ok || function == nil || parameter.Parent() != function || len(function.Blocks) == 0 ||
		!predicate.valid(function.Signature) || request.InvokeTarget && len(request.Methods) != 0 {
		return unknown
	}
	constants, ok := predicate.Arguments.bindings(function)
	if !ok || predicate.Outcome == CompletionAlways && len(constants) == 0 {
		return unknown
	}
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	locals := []mappedLocal{{local: parameter, supplied: parameter, kind: localExact}}
	for _, method := range methods {
		search := newCompletionSearch(method, CoverageEveryReturn, request.Budget)
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.exactInvocation, search.invokeTarget = request.InvokeTarget, request.InvokeTarget
		search.summarized = request.Summarized
		search.callContract = request.CallContract
		search.returnedSummaries = request.ReturnedSummaries
		search.constants = constants
		condition := completionCondition{result: predicate.Result, kind: completionConditionKind(predicate.Outcome)}
		proven := false
		paths := completionPaths{}
		search.paths = &paths
		search.memo.WithFunction(function, func() {
			if condition.kind == completionUnconditional {
				calls := func(candidate ssa.Instruction) bool { return search.instructionCompletes(candidate, locals, parameter) }
				proven = methodCallCoverageAssuming(function, calls, CoverageEveryReturn, ssaflow.EntryAssumptions{NonNil: parameter, Constants: constants})
				return
			}
			proven = search.conditionalCoverage(function, locals, parameter, condition)
		})
		if proven && !request.Budget.Exhausted() {
			return ssaflow.CompletionProof{
				Proof: ssaflow.Proof{
					State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCalledCompletion, Method: method, Provenance: ssaflow.EvidenceFromLocalSSA,
				},
				Path: paths.path, PathKnown: paths.known(),
			}
		}
	}
	return unknown
}

func (predicate CompletionPredicate) valid(signature *types.Signature) bool {
	if signature == nil || predicate.Result < 0 {
		return false
	}
	if predicate.Outcome == CompletionAlways {
		return predicate.Result == 0
	}
	if predicate.Result >= signature.Results().Len() {
		return false
	}
	result := signature.Results().At(predicate.Result).Type()
	switch predicate.Outcome {
	case CompletionWhenTrue, CompletionWhenFalse:
		basic, ok := result.Underlying().(*types.Basic)
		return ok && basic.Kind() == types.Bool
	case CompletionWhenNil, CompletionWhenNonNil:
		return syntax.IsErrorType(result)
	case CompletionAlways:
	}
	return false
}
