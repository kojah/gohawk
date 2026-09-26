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
	conditionalVersion      = 3
	conditionalExportBudget = 2000
	maxCaseParameters       = 2
)

// ConditionalSummary is the versioned, serializable part of a lifecycle fact
// containing its cases.
type ConditionalSummary struct {
	Version int
	Effects []ConditionalEffect
}

// ConditionalEffect records a method or synchronous callback invocation on
// Parameters on every normal return of the case Condition names.
type ConditionalEffect struct {
	Condition  ssaflow.CallCondition
	Method     string
	Invoke     bool
	Parameters ParameterMask
	// Path, when set, is where beneath the parameter the method settles,
	// exactly as in a Discharge; closing resp.Body is not closing resp.
	Path string
}

func summarizeConditional(pass *analysis.Pass, function *ssa.Function) *ConditionalSummary {
	conditions := caseConditions(function)
	if len(conditions) == 0 {
		return nil
	}
	budget := ssaflow.NewSearchBudget(conditionalExportBudget)
	lookup := conditionalLookup(func(instruction ssa.Instruction) (Fact, bool) { return importFact(pass, instruction) }, budget, nil)
	summary := &ConditionalSummary{Version: conditionalVersion}
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
					summary.Effects = append(summary.Effects, ConditionalEffect{
						Condition: condition, Method: method, Invoke: request.InvokeTarget, Parameters: parameterMaskFor(index), Path: path,
					})
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
	guarding := guardingParameters(function)
	for _, assignment := range argumentAssignments(guarding) {
		conditions = append(conditions, ssaflow.CallCondition{Arguments: assignment})
		for _, result := range results {
			result.Arguments = assignment
			conditions = append(conditions, result)
		}
	}
	return conditions
}

// guardingParameters returns the positions of up to maxCaseParameters
// Boolean parameters that can decide a branch: the parameter is a branch
// condition, possibly negated, or it flows into a closure or a call whose
// body may branch on it. A Boolean used only as data decides nothing.
func guardingParameters(function *ssa.Function) []int {
	var guarding []int
	for index, parameter := range function.Params {
		if index >= 64 || len(guarding) == maxCaseParameters {
			break
		}
		if basic, ok := parameter.Type().Underlying().(*types.Basic); !ok || basic.Kind() != types.Bool {
			continue
		}
		if slices.ContainsFunc(*parameter.Referrers(), decidesBranch) {
			guarding = append(guarding, index)
		}
	}
	return guarding
}

// decidesBranch reports whether a use of a Boolean parameter can reach a
// branch: a branch or negation, an argument to a call, or a store into the
// cell a closure captures it by.
func decidesBranch(user ssa.Instruction) bool {
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

// argumentAssignments lists every assignment of true and false to one of the
// positions, then to both, most general first.
func argumentAssignments(positions []int) []ssaflow.ArgumentConstants {
	var assignments []ssaflow.ArgumentConstants
	for _, position := range positions {
		bit := uint64(1) << position
		assignments = append(assignments,
			ssaflow.ArgumentConstants{Bound: bit, Values: bit}, ssaflow.ArgumentConstants{Bound: bit})
	}
	if len(positions) == 2 {
		first, second := uint64(1)<<positions[0], uint64(1)<<positions[1]
		for _, values := range []uint64{first | second, first, second, 0} {
			assignments = append(assignments, ssaflow.ArgumentConstants{Bound: first | second, Values: values})
		}
	}
	return assignments
}

func resultConditions(signature *types.Signature) []ssaflow.CallCondition {
	var conditions []ssaflow.CallCondition
	// This exported model covers at most four result slots. Other results are
	// opaque rather than adding unbounded combinations to dependency analysis.
	for index := range min(signature.Results().Len(), 4) {
		result := signature.Results().At(index).Type()
		var outcomes []ssaflow.ResultOutcome
		if basic, ok := result.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
			outcomes = []ssaflow.ResultOutcome{ssaflow.OutcomeTrue, ssaflow.OutcomeFalse}
		} else if syntax.IsErrorType(result) {
			outcomes = []ssaflow.ResultOutcome{ssaflow.OutcomeNil, ssaflow.OutcomeNonNil}
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
			proven = dischargesMatch(fact.caseDischarges(method, condition.Arguments), instruction, target, method, nil)
		}
		if proven && onFact != nil {
			onFact()
		}
		return proven
	}
}

// conditionalMask returns the parameters a fact's cases guarantee for query:
// the unconditional Must claims when the query has no result condition, and
// every case whose result condition matches and whose assumed arguments the
// query's call supplies.
func conditionalMask(fact Fact, method string, invoke bool, query ssaflow.CallCondition) ParameterMask {
	var mask ParameterMask
	if query.Outcome == ssaflow.OutcomeAny {
		if invoke {
			mask = fact.Must.SynchronouslyInvoked
		} else {
			mask = fact.MethodMask(method)
		}
	}
	if fact.Conditional == nil || fact.Conditional.Version != conditionalVersion {
		return mask
	}
	for _, effect := range fact.Conditional.Effects {
		// A case that settles a path beneath a parameter is matched by path
		// through caseDischarges, never as a claim on the parameter itself.
		if effect.Path == "" && effect.Method == method && effect.Invoke == invoke && effect.Condition.Matches(query) {
			mask |= effect.Parameters
		}
	}
	return mask
}

// caseDischarges projects the unconditional cases whose assumed arguments
// the supplied constants satisfy onto discharges of method, with their
// paths, so they are matched to the caller's values exactly as the fact's
// Must discharges are.
func (fact *Fact) caseDischarges(method string, supplied ssaflow.ArgumentConstants) []Discharge {
	if fact.Conditional == nil || fact.Conditional.Version != conditionalVersion || method == "" || supplied.Bound == 0 {
		return nil
	}
	query := ssaflow.CallCondition{Arguments: supplied}
	var discharges []Discharge
	for _, effect := range fact.Conditional.Effects {
		if effect.Invoke || effect.Method != method || !effect.Condition.Matches(query) {
			continue
		}
		for index := range 64 {
			if effect.Parameters&parameterMaskFor(index) != 0 {
				discharges = append(discharges, Discharge{Parameter: index, Method: method, Path: effect.Path})
			}
		}
	}
	return discharges
}

// suppliedConstants reports the Boolean constants a call passes, with known
// fixing the caller's own parameters when the call sits in a body searched
// under constants.
func suppliedConstants(instruction ssa.Instruction, known ssaflow.BooleanConstants) ssaflow.ArgumentConstants {
	return ssaflow.SuppliedConstants(ssaflow.InstructionCall(instruction), known)
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
	if fact.Conditional == nil {
		return nil
	}
	var lines []string
	for _, effect := range fact.Conditional.Effects {
		verb := effect.Method
		if effect.Invoke {
			verb = "invoke"
		}
		line := fmt.Sprintf("conditional result %d outcome %d: %s parameters %#x",
			effect.Condition.Result, effect.Condition.Outcome, verb, uint64(effect.Parameters))
		if arguments := effect.Condition.Arguments; arguments.Bound != 0 {
			line += fmt.Sprintf(" when arguments %#x are %#x", arguments.Bound, arguments.Values)
		}
		lines = append(lines, line)
	}
	return lines
}
