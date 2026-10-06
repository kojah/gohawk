package lifecycle

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// Completion evidence proves that the callee an instruction launches calls a
// lifecycle method on the caller's target. There is one search: resolve the
// callee, map the caller's target onto the callee's parameters and captured
// variables, and check how much of the callee's control flow a matching call
// covers. Every launch form demands the same coverage: the callee must call
// before each of its normal returns, on the paths feasible when the mapped
// local is non-nil. An ordinary query cannot credit conditional release;
// the separate result-edge query restricts completion to a tested result.
// Nested launches inside the
// callee are proved by the same search, so helper chains, deferred callbacks,
// cleanup registrations, and launched waiters need no separate rule.

// CompletionCoverage selects how much of a callee's control flow a lifecycle
// call must cover before it counts as completion.
type CompletionCoverage uint8

const (
	// CoverageEveryReturn, the default, accepts a call that precedes every
	// normal return; a callee that never returns or never calls proves
	// nothing. It answers whether the callee must complete the target.
	CoverageEveryReturn CompletionCoverage = iota
	// CoverageAnywhere accepts a call on any path. It answers only whether
	// the callee may complete the target, which suppresses a diagnostic that
	// would otherwise assume the obligation is still open.
	CoverageAnywhere
)

// MethodCallCoverage reports whether calls holds over function's normal paths
// with the requested coverage. nonNil, when set, restricts every-return
// analysis to paths feasible when that value is non-nil at entry.
func MethodCallCoverage(function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value) bool {
	return methodCallCoverageAssuming(function, calls, coverage, ssapath.EntryAssumptions{NonNil: nonNil})
}

// methodCallCoverageAssuming is MethodCallCoverage restricted to the paths
// feasible under the entry assumptions. Bound constants narrow both forms:
// a call only on an arm the constants rule out covers nothing, whether the
// question is every return or anywhere.
func methodCallCoverageAssuming(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, assumptions ssapath.EntryAssumptions,
) bool {
	return proveMethodCallCoverageAssumingWithin(function, calls, coverage, assumptions, nil).Proven()
}

// launchKind is how an instruction runs its callee.
type launchKind uint8

const (
	launchNone launchKind = iota
	// launchDeferred covers defer statements and testing Cleanup
	// registrations: the callee runs when the parent returns.
	launchDeferred
	// launchCalled is a synchronous call whose callee returns to the parent.
	launchCalled
	// launchStarted covers go statements and sync.WaitGroup.Go: the callee
	// runs on its own goroutine.
	launchStarted
	// launchCallback is a body examined independently of its launch; when
	// it runs is the caller's concern. It is never resolved from an
	// instruction, so merely creating or storing a literal proves nothing.
	launchCallback
)

func (launch launchKind) reason() proofs.EvidenceReason {
	switch launch {
	case launchDeferred:
		return proofs.EvidenceDeferredCompletion
	case launchCalled:
		return proofs.EvidenceCalledCompletion
	case launchStarted:
		return proofs.EvidenceStartedCompletion
	case launchCallback:
		return proofs.EvidenceCallbackCompletion
	case launchNone:
	}
	return proofs.EvidenceNone
}

// completionCallee is one resolved body to search. A callee reached through a
// closure value maps the caller's target through its bindings; a callee
// reached through a call also maps it through the call's arguments.
type completionCallee struct {
	environment *callbackBindings
	invocation  ssa.Instruction
	launch      launchKind
	common      *ssa.CallCommon
	closure     *ssa.MakeClosure
	function    *ssa.Function
}

var waitGroupGoMethod = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})

// mappedLocal is one callee parameter or captured variable that stands for
// the caller's target, together with the caller value supplied to it.
type mappedLocal struct {
	local    ssa.Value
	supplied ssa.Value
	kind     localKind
	// path is where the target sits beneath the supplied aggregate when the
	// mapping came from containment (localExact). For localProjection it is
	// the supplied value's proven path beneath the target. A receiver that is a
	// proper projection of the local must then be at this path: closing
	// j.other is not closing the file stored in j.out.
	path []string
}

// localKind is how the supplied caller value relates to the target, which
// decides which callee receivers stand for the target.
type localKind uint8

const (
	// localExact: the supplied value is the target, or an aggregate that
	// stores it, so any receiver derived from the local completes it.
	localExact localKind = iota
	// localProjection: the supplied value is a stable field or index path
	// beneath the target. Only the local itself may discharge the
	// obligation, because further projections, phis, and loads inside the
	// callee could select a different owner.
	localProjection
	// localOwner: the target is a stable path beneath the supplied value.
	// The receiver must select the mirrored path beneath the local.
	localOwner
	// localCallback: the supplied value is a callback bound to the method on
	// the target, so invoking the local is the completion.
	localCallback
)

// completionSearch proves one method for one instruction and target. Its memo
// stops recursion through helper cycles and keeps the search over the call
// graph rather than over every call path through it.
type completionSearch struct {
	condition         ssacall.CallCondition
	summarized        CompletionSummaryLookup
	callContract      CompletionSummaryLookup
	returnedSummaries ReturnedCleanupLookup
	returnedMemo      *ssacall.CallGraphMemo[returnedCleanupKey, bool]
	// Exact invocation excludes aggregate containment and may-alias mappings:
	// calling one function stored in an owner does not invoke every function.
	exactInvocation bool
	exactTarget     bool
	incomplete      *bool
	// inCycle records that a completing instruction was found inside a
	// cycle; shared by nested searches like incomplete. When coverage then
	// fails, the request reports EvidenceCompletionInCycle rather than a
	// disproof, because a loop that settles every element cannot be told
	// apart from one that settles some by path coverage alone.
	inCycle *bool
	// bindings are scoped to this invocation, never merged across callers.
	bindings *callbackBindings
	method   string
	coverage CompletionCoverage
	// Callback origins share one reaching fold across mappings in this request.
	// Revisit invalidates its enclosing memo answer, never proving absence.
	callbackValues ssaflow.ReachingWalk
	// invokeTarget marks a nested search whose target is a callback already
	// known to call the method: invoking a local mapped exactly to it is
	// then the completion, wherever the invocation sits.
	invokeTarget bool
	// memo owns the cycle guard for callee bodies and the rule that an answer
	// the guard cut short is not retained.
	memo *ssacall.CallGraphMemo[completionKey, completionAnswer]
	// paths collects where the completing calls of the body being covered
	// touched the target; nil outside a body's coverage.
	paths *completionPaths
	// budget, when set, bounds this question; nil leaves the search unbounded.
	budget *proofs.SearchBudget
	// constants fixes Boolean parameters and captures of the body being
	// searched, bound from the constant arguments of the call that reached it.
	// Like bindings, they are scoped to one invocation.
	constants ssacall.FixedValues
}

// forCallback returns a nested search for a callback value that shares the
// cycle guards of this search.
func (search *completionSearch) forCallback() *completionSearch {
	nested := *search
	nested.invokeTarget = true
	return &nested
}

func newCompletionSearch(method string, coverage CompletionCoverage, budget *proofs.SearchBudget) *completionSearch {
	search := &completionSearch{
		incomplete:   new(bool),
		inCycle:      new(bool),
		method:       method,
		coverage:     coverage,
		budget:       budget,
		memo:         ssacall.NewCallGraphMemo[completionKey, completionAnswer](),
		returnedMemo: ssacall.NewCallGraphMemo[returnedCleanupKey, bool](),
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType |
		ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	search.callbackValues = ssaflow.NewReachingWalk(forms).Within(budget).OnRevisit(search.memo.Incomplete)
	return search
}

func (search *completionSearch) calleeCoverage(callee completionCallee, target ssa.Value, invocation ssa.Instruction) bool {
	condition := search.condition
	search.condition = ssacall.CallCondition{}
	defer func() { search.condition = condition }()
	locals := search.mappedLocals(callee, target, invocation)
	if len(locals) == 0 {
		return false
	}
	if search.exactTarget && callee.launch == launchStarted {
		*search.incomplete = true
		return false
	}
	previous := search.bindings
	search.bindings = search.bindCallbackArguments(callee)
	defer func() { search.bindings = previous }()
	// A constant argument decides the callee's branches on that parameter, so
	// a helper that closes only behind a flag completes the target at a call
	// fixing the flag to the closing arm, and at no call fixing it otherwise.
	outer := search.constants
	fixed := ssacall.ProveFixedArgumentsWithin(callee.common, callee.closure, callee.function, outer, search.budget)
	if !fixed.Proven() {
		return false
	}
	search.constants = fixed.Values
	defer func() { search.constants = outer }()
	var nonNil ssa.Value
	var concrete types.Type
	for _, local := range locals {
		if local.kind == localExact || local.kind == localProjection {
			nonNil = local.local
			// The local holds the target itself, not an aggregate around
			// it, so the target's static type is what an assertion sees.
			if local.kind == localExact && len(local.path) == 0 && heapmodel.DefinitelySameValue(local.supplied, target) {
				concrete = target.Type()
			}
			break
		}
	}
	calls := func(candidate ssa.Instruction) bool {
		return search.instructionCompletes(candidate, locals, target)
	}
	if condition.Outcome != ssacall.OutcomeAny {
		return search.conditionalCoverage(callee.function, locals, target, condition)
	}
	assumptions := ssapath.EntryAssumptions{NonNil: nonNil, Constants: search.constants}
	if concrete != nil && search.coverage == CoverageEveryReturn {
		assumptions.NonNilType = concrete
		witnessAssumptions := ssapath.EntryAssumptions{Constants: search.constants}
		anywhere := proveMethodCallCoverageAssumingWithin(callee.function, calls, CoverageAnywhere, witnessAssumptions, search.budget)
		// Preserve the existing concrete-type contract: an anywhere witness
		// precedes coverage under the exact type assumption. It is not the
		// ordinary independent return witness on the unrefined body.
		return anywhere.Proven() && proveMethodReturnCoverageWithin(callee.function, calls, assumptions, search.budget).Proven()
	}
	return proveMethodCallCoverageAssumingWithin(callee.function, calls, search.coverage, assumptions, search.budget).Proven()
}

// instructionCompletes reports whether one callee instruction discharges the
// obligation: it calls the method on a mapped local, it invokes a callback
// parameter that was bound to the method on the target, or it launches a
// nested callee that completes the local by the same proof.
func (search *completionSearch) instructionCompletes(candidate ssa.Instruction, locals []mappedLocal, target ssa.Value) bool {
	if !search.budget.Spend() {
		// Every coverage form treats a false here as "this instruction does not
		// complete", so an exhausted budget can only fail to find a completion,
		// never invent one. The memo must not retain an answer shortened this
		// way, exactly as it does not retain one the cycle guard cut.
		return false
	}
	called := ssaflow.InstructionCall(candidate)
	if _, started := candidate.(*ssa.Go); started && search.exactTarget && !search.exactInvocation {
		*search.incomplete = true
		return false
	}
	if search.startsTarget(candidate, locals) {
		*search.incomplete = true
		return false
	}
	if search.methodCompletes(candidate, called, locals, target) {
		return true
	}
	for _, local := range locals {
		// A callback bound to the method on the target, such as rows.Close,
		// completes when the callee invokes that exact local, directly or in
		// a nested launch. crabbox hands cmd.Wait to a helper that invokes it
		// on a waiter goroutine:
		// https://github.com/openclaw/crabbox/blob/3ef3f98cbe27e6ddc814c11fde15b89c1639bcbe/internal/cli/ssh_forward_startup.go#L15-L22
		if local.kind == localCallback {
			if called != nil && invokesLocal(called.Value, local.local) {
				search.paths.record("", false)
				return true
			}
			if search.forCallback().completes(candidate, local.local).proven {
				search.paths.record("", false)
				return true
			}
			continue
		}
		if search.invokeTarget && local.kind == localExact && called != nil && search.invokesTargetLocal(called.Value, local.local) {
			search.paths.record("", false)
			return true
		}
		if answer := search.completes(candidate, local.local); answer.proven {
			// The nested answer's path is beneath the local; translate it
			// onto the target through the local's mapping.
			path, known := search.mappedPath(local, target, ssaflow.SplitAccessPath(answer.paths.path), answer.paths.known())
			if !known && local.requiresNestedPath() {
				// The local owns the target at a path, so completing the local
				// is not completing the target unless the nested answer names
				// that path. libovsdb's monitor locks and unlocks rpc while its
				// caller holds monitors, beneath the same client:
				// https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L286-L299
				// A contained field target likewise needs that exact path:
				// forwarding cleanup of a sibling does not settle the target.
				continue
			}
			search.paths.record(path, known)
			return true
		}
	}
	return false
}

// methodCompletes reports whether the candidate calls the sought method on a
// mapped local, and remembers when that call lies inside a cycle.
func (search *completionSearch) methodCompletes(candidate ssa.Instruction, called *ssa.CallCommon, locals []mappedLocal, target ssa.Value) bool {
	if called == nil || ssaflow.CallName(called) != search.method {
		return false
	}
	receiver := ssaflow.CallReceiver(called)
	for _, local := range locals {
		match := search.receives(local, receiver, target)
		if match.Possible && cfg.BlockInCycle(candidate.Block()) {
			// A dynamic aggregate element can witness possible loop cleanup,
			// never exact completion of the caller's field target.
			*search.inCycle = true
		}
		if match.Proven() {
			if cfg.BlockInCycle(candidate.Block()) {
				*search.inCycle = true
			}
			search.paths.record(search.receiverPath(local, receiver, target))
			return true
		}
	}
	return false
}

func (search *completionSearch) receives(local mappedLocal, receiver, target ssa.Value) receiverProof {
	if search.exactTarget {
		return receiverMatchProof(search.invokesTargetLocal(receiver, local.local))
	}
	return local.receives(receiver, target, search.budget)
}

func (search *completionSearch) startsTarget(candidate ssa.Instruction, locals []mappedLocal) bool {
	if !search.exactInvocation {
		return false
	}
	started, ok := candidate.(*ssa.Go)
	return ok && slices.ContainsFunc(locals, func(local mappedLocal) bool {
		return search.invokesTargetLocal(started.Common().Value, local.local)
	})
}
