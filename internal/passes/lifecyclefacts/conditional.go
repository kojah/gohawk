package lifecyclefacts

import (
	"fmt"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/resourcemodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Conditional facts preserve positive cleanup guarantees for one result value.
// They deliberately do not encode a complete effect set or negative claims.
// The shared completion engine proves the relation; this package owns bounded
// export, serialization, and binding the parameter mask at importing calls.
const (
	conditionalVersion      = 1
	conditionalExportBudget = 2000
)

// ConditionalSummary is the versioned, serializable part of a lifecycle fact
// containing result-conditioned cleanup guarantees.
type ConditionalSummary struct {
	Version int
	Effects []ConditionalEffect
}

// ConditionalEffect records a method or synchronous callback invocation on
// Parameters whenever Predicate holds at a normal return.
type ConditionalEffect struct {
	Predicate  lifecycle.CompletionPredicate
	Method     string
	Invoke     bool
	Parameters ParameterMask
}

func summarizeConditional(pass *analysis.Pass, function *ssa.Function) *ConditionalSummary {
	predicates := conditionalPredicates(function.Signature)
	if len(predicates) == 0 {
		return nil
	}
	budget := ssaflow.NewSearchBudget(conditionalExportBudget)
	lookup := conditionalLookup(func(instruction ssa.Instruction) (Fact, bool) { return importFact(pass, instruction) }, budget, nil)
	summary := &ConditionalSummary{Version: conditionalVersion}
	for index, parameter := range function.Params {
		for _, method := range conditionalMethods(parameter.Type()) {
			for _, predicate := range predicates {
				if !budget.Spend() {
					return summary
				}
				request := lifecycle.CompletionRequest{
					Target: parameter, Budget: budget, Summarized: lookup, CallContract: resourcemodel.ConditionalReleases(budget),
				}
				request.InvokeTarget = method == ""
				if !request.InvokeTarget {
					request.Methods = []string{method}
				}
				if lifecycle.ProveCompletionForResult(function, predicate, request).Proven() {
					summary.Effects = append(summary.Effects, ConditionalEffect{
						Predicate: predicate, Method: method, Invoke: request.InvokeTarget, Parameters: parameterMaskFor(index),
					})
				}
			}
		}
	}
	return summary
}

func conditionalPredicates(signature *types.Signature) []lifecycle.CompletionPredicate {
	var predicates []lifecycle.CompletionPredicate
	// This exported model covers at most four result slots. Other results are
	// opaque rather than adding unbounded combinations to dependency analysis.
	for index := range min(signature.Results().Len(), 4) {
		result := signature.Results().At(index).Type()
		var outcomes []lifecycle.CompletionOutcome
		if basic, ok := result.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
			outcomes = []lifecycle.CompletionOutcome{lifecycle.CompletionWhenTrue, lifecycle.CompletionWhenFalse}
		} else if syntax.IsErrorType(result) {
			outcomes = []lifecycle.CompletionOutcome{lifecycle.CompletionWhenNil, lifecycle.CompletionWhenNonNil}
		}
		for _, outcome := range outcomes {
			predicates = append(predicates, lifecycle.CompletionPredicate{Result: index, Outcome: outcome})
		}
	}
	return predicates
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
	return func(instruction ssa.Instruction, target ssa.Value, method string, invoke bool, predicate lifecycle.CompletionPredicate) bool {
		fact, ok := lookup(instruction)
		if !ok || !budget.Spend() {
			return false
		}
		mask := conditionalMask(fact, method, invoke, predicate)
		proven := factArgumentMatches(instruction, target, mask, func(argument, target ssa.Value) bool {
			return heapmodel.NewStorage(budget).Same(argument, target).Proven()
		})
		if proven && onFact != nil {
			onFact()
		}
		return proven
	}
}

func conditionalMask(fact Fact, method string, invoke bool, predicate lifecycle.CompletionPredicate) ParameterMask {
	if predicate.Outcome == lifecycle.CompletionAlways {
		if invoke {
			return fact.SynchronouslyInvoked
		}
		return fact.MethodMask(method)
	}
	if fact.Conditional == nil || fact.Conditional.Version != conditionalVersion {
		return 0
	}
	var mask ParameterMask
	for _, effect := range fact.Conditional.Effects {
		if effect.Predicate == predicate && effect.Method == method && effect.Invoke == invoke {
			mask |= effect.Parameters
		}
	}
	return mask
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
		lines = append(lines, fmt.Sprintf("conditional result %d outcome %d: %s parameters %#x",
			effect.Predicate.Result, effect.Predicate.Outcome, verb, uint64(effect.Parameters)))
	}
	return lines
}
