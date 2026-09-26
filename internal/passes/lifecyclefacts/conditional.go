package lifecyclefacts

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/resourcemodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Conditional facts are the cases of a lifecycle summary: positive cleanup
// guarantees that hold on the normal returns matching a result condition, on
// the paths feasible when Boolean arguments are fixed to constants, or both.
// They deliberately do not encode a complete effect set or negative claims,
// so a missing case is unknown, never proof that the callee leaves the target
// open. The shared completion engine proves each case; this package owns the
// bounded export, serialization, and matching a case to an importing call.
//
// Argument cases are bounded structurally: only Boolean parameters that can
// decide a branch, directly or through a closure or nested call, and at most
// maxCaseParameters of them, so a function has at most eight assignments. A
// function outside that bound keeps only its unconditional and result cases,
// which is exactly the summary it had before argument cases existed.
const (
	conditionalExportBudget = 2000
	maxCaseParameters       = 2
)

// summarizeConditional proves the function's cases as discharges with a
// condition: a cleanup method, or SynchronousInvokeMethod for a callback
// parameter, on every normal return of the case, at the path it settles.
func summarizeConditional(pass *analysis.Pass, function *ssa.Function) []Discharge {
	conditions := caseConditions(function)
	if len(conditions) == 0 {
		return nil
	}
	budget := ssaflow.NewSearchBudget(conditionalExportBudget)
	lookup := conditionalLookup(func(instruction ssa.Instruction) (Fact, bool) { return importFact(pass, instruction) }, budget, nil)
	var summary []Discharge
	for index, parameter := range function.Params {
		for _, method := range conditionalMethods(parameter.Type()) {
			var proven []ssaflow.CallCondition
			for _, condition := range conditions {
				if !budget.Spend() {
					return summary
				}
				if impliedByProven(condition, proven) {
					continue
				}
				request := lifecycle.CompletionRequest{
					Target: parameter, Budget: budget, Summarized: lookup, CallContract: resourcemodel.ConditionalReleases(budget),
					ExactTarget: true,
				}
				request.InvokeTarget = method == ""
				if !request.InvokeTarget {
					request.Methods = []string{method}
				}
				path, ok := provenCasePath(function, condition, request)
				if ok {
					proven = append(proven, condition)
					discharged := method
					if request.InvokeTarget {
						discharged = SynchronousInvokeMethod
					}
					summary = append(summary, Discharge{Condition: condition, Parameter: index, Method: discharged, Path: path})
				}
			}
		}
	}
	return summary
}

// provenCasePath proves one case for the parameter itself and, failing that
// for a method, for a field or element beneath it. A path claim needs the
// proof to name one non-empty path; a completion settled through another
// summary, whose path is unknown, is claimed only on the parameter itself.
func provenCasePath(function *ssa.Function, condition ssaflow.CallCondition, request lifecycle.CompletionRequest) (string, bool) {
	if lifecycle.ProveCompletionForCase(function, condition, request).Proven() {
		return "", true
	}
	if request.InvokeTarget {
		return "", false
	}
	request.ExactTarget = false
	proof := lifecycle.ProveCompletionForCase(function, condition, request)
	if proof.Proven() && proof.PathKnown && proof.Path != "" {
		return proof.Path, true
	}
	return "", false
}

// withoutUnconditional drops the cases an unconditional discharge already
// states: a case whose claim holds on every return anyway adds nothing a
// caller could select.
func withoutUnconditional(cases, unconditional []Discharge) []Discharge {
	var kept []Discharge
	for _, candidate := range cases {
		stated := slices.ContainsFunc(unconditional, func(discharge Discharge) bool {
			return discharge.Condition.Unconditional() && discharge.Parameter == candidate.Parameter &&
				discharge.Method == candidate.Method && discharge.Path == candidate.Path
		})
		if !stated {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// impliedByProven reports whether a proven case with the same result
// condition and fewer assumed arguments already covers condition, so the
// narrower case would add nothing a caller could use.
func impliedByProven(condition ssaflow.CallCondition, proven []ssaflow.CallCondition) bool {
	for _, earlier := range proven {
		if earlier.Matches(condition) {
			return true
		}
	}
	return false
}

// caseConditions lists the cases worth proving, most general first: each
// result condition alone, then each assignment of one guarding parameter,
// then of both, with and without each result condition. The unconditional
// case with no arguments is the fact's Must claims and is not repeated.
func caseConditions(function *ssa.Function) []ssaflow.CallCondition {
	results := resultConditions(function.Signature)
	conditions := slices.Clone(results)
	for _, assignment := range argumentAssignments(guardingParameters(function)) {
		conditions = append(conditions, assignment)
		for _, result := range results {
			result.Arguments, result.Nilness = assignment.Arguments, assignment.Nilness
			conditions = append(conditions, result)
		}
	}
	return conditions
}

// guard is a parameter that can decide a branch: a Boolean, or a nilable
// value compared with nil.
type guard struct {
	index   int
	nilable bool
}

// guardingParameters returns up to maxCaseParameters parameters that can
// decide a branch: a Boolean that is a branch condition, possibly negated,
// or that flows into a closure or a call whose body may test it; or a
// nilable value the body compares with nil, directly or in a closure that
// captures it. A nilable value merely passed on is not a guard: most pointer
// parameters are, and counting them would crowd out the flags that decide.
func guardingParameters(function *ssa.Function) []guard {
	var guarding []guard
	for index, parameter := range function.Params {
		if index >= 64 || len(guarding) == maxCaseParameters {
			break
		}
		if basic, ok := parameter.Type().Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
			if slices.ContainsFunc(*parameter.Referrers(), booleanDecides) {
				guarding = append(guarding, guard{index: index})
			}
			continue
		}
		if ssaflow.Nilable(parameter.Type()) && slices.ContainsFunc(*parameter.Referrers(), ssaflow.ComparesWithNil) {
			guarding = append(guarding, guard{index: index, nilable: true})
		}
	}
	return guarding
}

// booleanDecides reports whether a use of a Boolean parameter can reach a
// branch: a branch or negation, an argument to a call, or a store into the
// cell a closure captures it by.
func booleanDecides(user ssa.Instruction) bool {
	switch typed := user.(type) {
	case *ssa.If, *ssa.MakeClosure, *ssa.Call, *ssa.Defer, *ssa.Go:
		return true
	case *ssa.UnOp:
		return typed.Op == token.NOT
	case *ssa.Store:
		_, cell := typed.Addr.(*ssa.Alloc)
		return cell
	}
	return false
}

// argumentAssignments lists every assignment of the two outcomes to one of
// the guards, then to both, most general first: true and false for a
// Boolean, nil and non-nil for a nilable value.
func argumentAssignments(guards []guard) []ssaflow.CallCondition {
	var assignments []ssaflow.CallCondition
	for _, one := range guards {
		assignments = append(assignments, one.assign(true), one.assign(false))
	}
	if len(guards) == 2 {
		for _, first := range []bool{true, false} {
			for _, second := range []bool{true, false} {
				assignment := guards[0].assign(first)
				other := guards[1].assign(second)
				assignment.Arguments.Bound |= other.Arguments.Bound
				assignment.Arguments.Values |= other.Arguments.Values
				assignment.Nilness.Bound |= other.Nilness.Bound
				assignment.Nilness.Values |= other.Nilness.Values
				assignments = append(assignments, assignment)
			}
		}
	}
	return assignments
}

// assign is the condition fixing the guard to one of its two outcomes: true,
// or nil for a nilable guard, when set.
func (one guard) assign(set bool) ssaflow.CallCondition {
	bit := uint64(1) << one.index
	values := uint64(0)
	if set {
		values = bit
	}
	if one.nilable {
		return ssaflow.CallCondition{Nilness: ssaflow.ArgumentConstants{Bound: bit, Values: values}}
	}
	return ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: bit, Values: values}}
}

func resultConditions(signature *types.Signature) []ssaflow.CallCondition {
	var conditions []ssaflow.CallCondition
	// This exported model covers at most four result slots. Other results are
	// opaque rather than adding unbounded combinations to dependency analysis.
	for index := range min(signature.Results().Len(), 4) {
		result := signature.Results().At(index).Type()
		var outcomes []ssaflow.Outcome
		if basic, ok := result.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
			outcomes = []ssaflow.Outcome{ssaflow.OutcomeTrue, ssaflow.OutcomeFalse}
		} else if syntax.IsErrorType(result) {
			outcomes = []ssaflow.Outcome{ssaflow.OutcomeNil, ssaflow.OutcomeNonNil}
		}
		for _, outcome := range outcomes {
			conditions = append(conditions, ssaflow.CallCondition{Result: index, Outcome: outcome})
		}
	}
	return conditions
}

func conditionalMethods(value types.Type) []string {
	if _, callback := value.Underlying().(*types.Signature); callback {
		return []string{""}
	}
	var methods []string
	for _, method := range cleanupMethods {
		if object, _, _ := types.LookupFieldOrMethod(value, false, nil, method); object != nil {
			methods = append(methods, method)
		}
	}
	return methods
}

func conditionalLookup(
	lookup func(ssa.Instruction) (Fact, bool), budget *ssaflow.SearchBudget, onFact func(),
) lifecycle.CompletionSummaryLookup {
	return func(instruction ssa.Instruction, target ssa.Value, method string, invoke bool, condition ssaflow.CallCondition) bool {
		fact, ok := lookup(instruction)
		if !ok || !budget.Spend() {
			return false
		}
		mask := conditionalMask(fact, method, invoke, condition)
		proven := factArgumentMatches(instruction, target, mask, func(argument, target ssa.Value) bool {
			return heapmodel.NewStorage(budget).Same(argument, target).Proven()
		})
		if !proven && !invoke && condition.Outcome == ssaflow.OutcomeAny {
			proven = dischargesMatch(fact.casesSelectedBy(method, condition), instruction, target, method, nil)
		}
		if proven && onFact != nil {
			onFact()
		}
		return proven
	}
}

// conditionalMask returns the parameters a fact guarantees for query: the
// unconditional claims when the query has no result condition, and every
// case whose condition matches the query. A case that settles a path beneath
// a parameter is matched by path through casesSelectedBy, never as a claim on
// the parameter itself.
func conditionalMask(fact Fact, method string, invoke bool, query ssaflow.CallCondition) ParameterMask {
	if invoke {
		method = SynchronousInvokeMethod
	}
	var mask ParameterMask
	if query.Outcome == ssaflow.OutcomeAny {
		mask = fact.MethodMask(method)
	}
	for _, discharge := range fact.caseDischarges() {
		if discharge.Path == "" && discharge.Method == method && discharge.Condition.Matches(query) {
			mask |= parameterMaskFor(discharge.Parameter)
		}
	}
	return mask
}

// casesSelectedBy returns the cases of method with no result condition whose
// assumed arguments the supplied condition satisfies, so they are matched to
// the caller's values exactly as the unconditional discharges are.
func (fact *Fact) casesSelectedBy(method string, supplied ssaflow.CallCondition) []Discharge {
	if method == "" || supplied.Arguments.Bound == 0 && supplied.Nilness.Bound == 0 {
		return nil
	}
	query := ssaflow.CallCondition{Arguments: supplied.Arguments, Nilness: supplied.Nilness}
	var selected []Discharge
	for _, discharge := range fact.caseDischarges() {
		if discharge.Method == method && discharge.Condition.Matches(query) {
			selected = append(selected, discharge)
		}
	}
	return selected
}

// suppliedCondition reports what a call's arguments fix, with known fixing
// the caller's own parameters when the call sits in a body searched under
// fixed values.
func suppliedCondition(instruction ssa.Instruction, known ssaflow.FixedValues) ssaflow.CallCondition {
	return ssaflow.SuppliedCondition(ssaflow.InstructionCall(instruction), known)
}

// CompletionOnEdge combines local and imported result-conditioned guarantees.
// Absence remains unknown; only exact parameter binding can settle the target.
func (evidence *LifecycleEvidence) CompletionOnEdge(from, to *ssa.BasicBlock, request lifecycle.CompletionRequest) CompletionProof {
	usedFact := false
	lookup := conditionalLookup(func(instruction ssa.Instruction) (Fact, bool) {
		return factFor(evidence.pass, instruction)
	}, request.Budget, func() { usedFact = true })
	request.Summarized = lookup
	request.CallContract = resourcemodel.ConditionalReleases(request.Budget)
	proof := CompletionProof{CompletionProof: lifecycle.ProveCompletionOnEdge(from, to, request)}
	if proof.Proven() && usedFact {
		proof.Provenance = ssaflow.EvidenceFromImportedFact
		proof.SummaryReason = reasonConditionalSummary
	}
	if from != nil && len(from.Instrs) != 0 && proof.Proven() {
		evidence.emit(
			EvidenceRequest{Instruction: from.Instrs[len(from.Instrs)-1], Target: request.Target, Completion: &request},
			Proof{Proof: proof.Proof, SummaryReason: proof.SummaryReason},
		)
	}
	return proof
}

func (fact *Fact) conditionalDescriptions() []string {
	var lines []string
	for _, discharge := range fact.caseDischarges() {
		verb := discharge.Method
		if discharge.Path != "" {
			verb += " at " + discharge.Path
		}
		lines = append(lines, fmt.Sprintf("%s parameter %d when %s", verb, discharge.Parameter, discharge.Condition))
	}
	return lines
}
