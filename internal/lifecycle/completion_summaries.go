package lifecycle

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// Completion summaries cache an invocation-specific question while guarding
// each possible callee independently. Missing or recursive bodies cannot prove
// completion; the summary driver owns cache invalidation after truncated work.

// completionKey identifies one completion question. The method and coverage
// are fixed for a search, but a callback search shares the parent's guards
// while answering a different question, so invokeTarget belongs in the key.
// Result conditions also belong in the key: true-edge completion must never
// answer a false-edge or unconditional question about the same invocation.
// So do the constants fixing the enclosing body's parameters: the same call
// inside a helper can complete under one binding and not another.
type completionKey struct {
	bindings     *callbackBindings
	instruction  ssa.Instruction
	target       ssa.Value
	invokeTarget bool
	condition    ssacall.CallCondition
	constants    string
}

type completionAnswer struct {
	launch    launchKind
	proven    bool
	available bool
	// paths says where beneath the target a proven completion settled; see
	// completionPaths. It is meaningful only when proven.
	paths completionPaths
}

func (answer completionAnswer) proof(method string) proofs.CompletionProof {
	return proofs.CompletionProof{
		Proof: proofs.Proof{
			State: proofs.EvidenceProven, Reason: answer.launch.reason(), Method: method, Provenance: proofs.EvidenceFromLocalSSA,
		},
		Path: answer.paths.path, PathKnown: answer.paths.known(),
	}
}

// completes reports whether the instruction's callees all call method on the
// target with the coverage their launch demands. The answer is unavailable
// when no callee body was available to search.
func (search *completionSearch) completes(instruction ssa.Instruction, target ssa.Value) completionAnswer {
	key := completionKey{
		instruction: instruction, target: target, invokeTarget: search.invokeTarget, bindings: search.bindings, condition: search.condition,
		constants: search.constants.Key(instruction.Parent()),
	}
	return search.memo.Compose(key, search.budget, func() completionAnswer {
		return search.searchCompletes(instruction, target)
	}, func(_ ssacall.SummaryUnavailable, partial completionAnswer) completionAnswer {
		partial.proven = false
		return partial
	})
}

func (search *completionSearch) searchCompletes(instruction ssa.Instruction, target ssa.Value) completionAnswer {
	if kind, proven := search.returnedCallCompletes(instruction, target); proven {
		// A completion routed through a returned callback is proven about
		// the target as a whole; the search does not follow its path.
		return completionAnswer{launch: kind, proven: true, available: true}
	}
	if _, synchronous := instruction.(*ssa.Call); synchronous && search.callContract != nil &&
		search.callContract(instruction, target, search.method, search.invokeTarget, search.queryAt(instruction)) {
		return completionAnswer{launch: launchCalled, proven: true, available: true}
	}
	callees, ok := search.boundCallees(instruction)
	if !ok || len(callees) == 0 {
		return completionAnswer{launch: launchNone}
	}
	searched := false
	var paths completionPaths
	for _, callee := range callees {
		callee.invocation = instruction
		if callee.environment == nil {
			callee.environment = search.bindings
		}
		if callee.function == nil || len(callee.function.Blocks) == 0 {
			if _, synchronous := instruction.(*ssa.Call); synchronous && search.summarized != nil &&
				search.summarized(instruction, target, search.method, search.invokeTarget, search.queryAt(instruction)) {
				// A summary settles the target as a whole; where it did so
				// beneath the target is not part of its claim.
				paths.record("", false)
				continue
			}
			return completionAnswer{launch: callee.launch, available: searched}
		}
		answer := search.calleeCompletes(callee, target, instruction)
		if !answer.available {
			return completionAnswer{launch: callee.launch, available: searched}
		}
		searched = true
		if !answer.proven {
			return completionAnswer{launch: callee.launch, available: true}
		}
		paths.merge(answer.paths)
	}
	return completionAnswer{launch: callees[0].launch, proven: true, available: true, paths: paths}
}

func (search *completionSearch) calleeCompletes(callee completionCallee, target ssa.Value, invocation ssa.Instruction) completionAnswer {
	answer := completionAnswer{launch: callee.launch}
	answer.available = search.memo.WithFunction(callee.function, func() {
		// Each body's coverage records its own completing calls, so a
		// nested body's paths do not bleed into the enclosing answer
		// except through the mapping that translates them.
		previous := search.paths
		search.paths = &completionPaths{}
		answer.proven = search.calleeCoverage(callee, target, invocation)
		answer.paths = *search.paths
		search.paths = previous
	})
	if !answer.available {
		// A rejected body visit supplies no negative evidence. In
		// particular, recursive calls must remain unknown rather than
		// turning an incomplete coverage search into a disproof.
		*search.incomplete = true
	}
	return answer
}

// CompletionSummaryLookup supplies positive guarantees for unavailable callees.
// The implementation must bind target to an exact argument, distinguish the
// requested method from callback invocation, and match the query condition:
// the search's result condition, with the constants the call supplies.
// False means no guarantee, never proof that the callee has no effect.
type CompletionSummaryLookup func(ssa.Instruction, ssa.Value, string, bool, ssacall.CallCondition) bool

// queryAt is the question a summary lookup answers for one call: this
// search's result condition, with the constants the call supplies.
func (search *completionSearch) queryAt(instruction ssa.Instruction) ssacall.CallCondition {
	supplied := ssacall.SuppliedCondition(ssaflow.InstructionCall(instruction), search.constants)
	query := search.condition
	query.Arguments, query.Nilness = supplied.Arguments, supplied.Nilness
	return query
}

// ProveCompletionForCase summarizes exact parameter cleanup on the normal
// returns of one case: the returns matching the condition's result test, on
// the paths feasible when its assumed arguments hold. It reuses the
// completion engine and its shared budget, callback bindings, and recursion
// guard; it does not invent a caller or SSA. Without ExactTarget, a method
// completion may settle a field or element beneath the parameter; the proof
// then names that path, and a caller must not credit a claim whose path is
// not known.
func ProveCompletionForCase(function *ssa.Function, condition ssacall.CallCondition, request CompletionRequest) proofs.CompletionProof {
	unknown := proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	parameter, ok := request.Target.(*ssa.Parameter)
	if !ok || function == nil || parameter.Parent() != function || len(function.Blocks) == 0 ||
		!condition.ValidFor(function.Signature) || request.InvokeTarget && len(request.Methods) != 0 {
		return unknown
	}
	constants, ok := condition.Bindings(function)
	if !ok || condition.Unconditional() {
		return unknown
	}
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	locals := []mappedLocal{{local: parameter, supplied: parameter, kind: localExact}}
	resultTest := ssacall.CallCondition{Result: condition.Result, Outcome: condition.Outcome}
	for _, method := range methods {
		search := newCompletionSearch(method, CoverageEveryReturn, request.Budget)
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.exactInvocation, search.invokeTarget = request.InvokeTarget, request.InvokeTarget
		search.summarized = request.Summarized
		search.callContract = request.CallContract
		search.returnedSummaries = request.ReturnedSummaries
		search.constants = constants
		proven := false
		paths := completionPaths{}
		search.paths = &paths
		search.memo.WithFunction(function, func() {
			if resultTest.Outcome == ssacall.OutcomeAny {
				calls := func(candidate ssa.Instruction) bool { return search.instructionCompletes(candidate, locals, parameter) }
				assumptions := ssapath.EntryAssumptions{NonNil: parameter, Constants: constants}
				proven = proveMethodCallCoverageAssumingWithin(function, calls, CoverageEveryReturn, assumptions, request.Budget).Proven()
				return
			}
			proven = search.conditionalCoverage(function, locals, parameter, resultTest)
		})
		if proven && !request.Budget.Exhausted() {
			return proofs.CompletionProof{
				Proof: proofs.Proof{
					State: proofs.EvidenceProven, Reason: proofs.EvidenceCalledCompletion, Method: method, Provenance: proofs.EvidenceFromLocalSSA,
				},
				Path: paths.path, PathKnown: paths.known(),
			}
		}
	}
	if request.Budget.Exhausted() {
		unknown.Reason = proofs.EvidenceBudgetExhausted
	}
	return unknown
}
