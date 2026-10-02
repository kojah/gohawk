package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Helper-use evidence follows one tracked value into a source-visible callee
// through the exact parameter or captured variable that carries it, and
// reports what the callee does with it. Only static call chains are followed;
// a dynamic callee, a launched goroutine, or an invoked callback derived from
// the value ends the proof as an escape.

// helperUse classifies how a callee treats one parameter or captured variable.
// Positive coverage requires the requested effect on every normal return.
// Channel receives and group waits observe completion. Owner coverage means
// invoking a lifecycle method; the worker classifier consumes that as unknown
// shutdown participation, while retained-owner queries use it only to identify
// cleanup. Storing, sending, returning, capturing, or
// passing the value to an opaque call ends the proof as unknown. A callee that
// merely reads the value, or joins it on some paths, proves nothing.
// helperSearch answers one helper-use question. The memo owns the cycle guard
// and the rule that an answer cut short by it is not retained.
type helperSearch struct {
	concurrency *concurrencyfacts.Engine
	memo        *ssaflow.CallGraphMemo[helperKey, ownershipAction]
	budget      *ssaflow.SearchBudget
}

const helperUseBudget = 1000

type helperKey struct {
	function *ssa.Function
	local    ssa.Value
	kind     trackedKind
}

func newHelperSearch() *helperSearch {
	return &helperSearch{
		memo: ssaflow.NewCallGraphMemo[helperKey, ownershipAction](), budget: ssaflow.NewSearchBudget(helperUseBudget),
	}
}

func (search *helperSearch) use(function *ssa.Function, local ssa.Value, kind trackedKind) ownershipAction {
	key := helperKey{function: function, local: local, kind: kind}
	return search.memo.Summarize(key, function, search.budget, func() ownershipAction {
		return search.searchUse(function, local, kind)
	}, func(reason ssaflow.SummaryUnavailable, _ ownershipAction) ownershipAction {
		// A recursive or exhausted search cannot establish the absence of a
		// completion handoff. Its caller supplied the tracked value, so the
		// cutoff is opaque consumption, never a missing-join proof. A later
		// independent exact observation can still cover every return.
		if reason == ssaflow.SummaryBudgetExhausted || reason == ssaflow.SummaryRecursive {
			return actionUnknown
		}
		// Body availability is handled by the call classifier before asking
		// for helper coverage; no positive cleanup witness is supplied here.
		return actionNone
	})
}

func (search *helperSearch) searchUse(function *ssa.Function, local ssa.Value, kind trackedKind) ownershipAction {
	derives := func(value ssa.Value) bool {
		return heapmodel.ValueDerivesFrom(value, local)
	}
	storage := heapmodel.NewStorage(search.budget)
	exact := func(value ssa.Value) bool { return storage.Same(value, local).Proven() }
	// Owner coverage identifies possible lifecycle participation only. Its
	// consumers already project it as unknown worker ownership. Completion
	// channels and groups require exact identity inside the helper as well.
	if kind == trackedOwner {
		exact = derives
	}
	joins := func(instruction ssa.Instruction) bool {
		proof := proveSummaryJoin(search.concurrency, instruction, local, kind, search.budget)
		return proof.joined || search.instructionJoins(instruction, kind, exact)
	}
	// A select case can lead directly into a shared return block. Carry the
	// receive on its edge instead of lending it to other incoming paths.
	joinsEdge := func(from, to *ssa.BasicBlock) bool {
		return search.receiveEdgeAction(from, to, kind, exact, derives) == actionJoin
	}
	joined, escaped, joinedInCycle := false, false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return actionUnknown
			}
			joinedHere := joins(instruction)
			if joinedHere {
				joined = true
				joinedInCycle = joinedInCycle || ssaflow.BlockInCycle(block)
			}
			// A possible receive/Wait or nested binding cannot establish a
			// join. Preserve its former acceptance as uncertainty, including
			// overwritten storage and mixed phi alternatives.
			escaped = escaped || !joinedHere && search.instructionJoins(instruction, kind, derives)
			escaped = escaped || search.instructionEscapes(instruction, local, kind, derives)
		}
		for _, successor := range block.Succs {
			action := search.receiveEdgeAction(block, successor, kind, exact, derives)
			joined = action == actionJoin || joined
			escaped = action == actionUnknown || escaped
		}
	}
	joinProven := joined && ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Entry: function, Owns: joins, OwnsEdge: joinsEdge}) == nil
	if search.budget.Exhausted() {
		return actionUnknown
	}
	if joinProven {
		return actionJoin
	}
	// A helper that joins every worker it was handed inside a loop is not
	// covered on every return, and which worker an iteration joins is
	// decided by iteration: uncertainty, not a missing join.
	if escaped || joinedInCycle {
		return actionUnknown
	}
	return actionNone
}

// A selected arm observes only the handle selected on that edge. Possible
// identity keeps the helper opaque without lending completion to other arms.
func (search *helperSearch) receiveEdgeAction(
	from, to *ssa.BasicBlock, kind trackedKind, exact, possible func(ssa.Value) bool,
) ownershipAction {
	if kind != trackedSignal || !search.budget.Spend() {
		return actionNone
	}
	channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
	if !selected {
		return actionNone
	}
	if exact(channel) {
		return actionJoin
	}
	if possible(channel) {
		return actionUnknown
	}
	return actionNone
}

func (search *helperSearch) instructionJoins(instruction ssa.Instruction, kind trackedKind, derives func(ssa.Value) bool) bool {
	// Charge the coverage query too: it can revisit an instruction after the
	// initial scan, and its nested helpers must share this same budget.
	if !search.budget.Spend() {
		return false
	}
	if _, launched := instruction.(*ssa.Go); launched {
		return false
	}
	if kind == trackedSignal && guaranteedReceive(instruction, derives) {
		return true
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if receiverJoins(common, kind, derives) {
		return true
	}
	callee, closure := ssaflow.DirectCallee(common)
	if callee == nil {
		return false
	}
	return slices.ContainsFunc(ssaflow.CallBindings(common, callee, closure), func(pair ssaflow.CallBinding) bool {
		return search.budget.Spend() && derives(pair.Supplied) && search.use(callee, pair.Local, kind) == actionJoin
	})
}

// receiverJoins recognizes the requested receiver effect: Wait on a group or
// a lifecycle call on an owner. The owner result is cleanup coverage, not a
// worker join; its caller must preserve that distinction.
func receiverJoins(common *ssa.CallCommon, kind trackedKind, derives func(ssa.Value) bool) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !derives(receiver) {
		return false
	}
	switch kind {
	case trackedGroup:
		return ssaflow.CallMatchesSymbol(common, waitGroupWait)
	case trackedOwner:
		return lifecycleMethod(ssaflow.CallName(common))
	case trackedSignal:
	}
	return false
}

func (search *helperSearch) instructionEscapes(
	instruction ssa.Instruction,
	local ssa.Value,
	kind trackedKind,
	derives func(ssa.Value) bool,
) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		return derives(typed.Val)
	case *ssa.Send:
		return derives(typed.X)
	case *ssa.Select:
		// A helper offering a send on a field of the supplied owner may
		// participate in its shutdown protocol. It is not a channel join;
		// preserve the aggregate handoff as unknown without borrowing another
		// select arm's receive. A bare completion-channel select does not qualify.
		return slices.ContainsFunc(typed.States, func(state *ssa.SelectState) bool {
			return state.Send != nil && ssaflow.ValueIsAccessPathFrom(state.Chan, local)
		})
	case *ssa.MapUpdate:
		return derives(typed.Value)
	case *ssa.MakeClosure:
		return slices.ContainsFunc(typed.Bindings, derives)
	case *ssa.Return:
		// Returning a channel selected from the tracked owner exposes its join
		// handle. The accessor itself is not a join, but its caller may drain
		// the stream; losing that relationship cannot prove an unjoined worker.
		// https://github.com/raviqqe/muffet/blob/ea33f85e5644c609a114b00e1f4dfc757b15c8ee/page_checker_test.go#L39-L46
		return lifecycle.ReturnedValueOwnsValue(typed, local) || slices.ContainsFunc(typed.Results, func(value ssa.Value) bool {
			return ssaflow.ChannelType(value) && derives(value)
		})
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return search.callEscapes(instruction, kind, derives)
	default:
		return false
	}
}

func (search *helperSearch) callEscapes(instruction ssa.Instruction, kind trackedKind, derives func(ssa.Value) bool) bool {
	common := ssaflow.InstructionCall(instruction)
	// Receiver bookkeeping is transparent only in the caller's invocation.
	// A launched Wait/cleanup may own shutdown on another goroutine; it is
	// opaque handoff, never positive completion or a read-only receiver use.
	if _, launched := instruction.(*ssa.Go); launched {
		_, closure := ssaflow.DirectCallee(common)
		return helperCallCarries(common, closure, derives)
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name() == "append" && slices.ContainsFunc(common.Args, derives)
	}
	if derives(common.Value) {
		// Invoking a callback derived from the value runs code this proof does
		// not follow.
		return true
	}
	if receiverCallRetainsNothing(common, kind, derives) {
		return false
	}
	callee, closure := ssaflow.DirectCallee(common)
	if callee == nil || len(callee.Blocks) == 0 {
		return helperCallCarries(common, closure, derives)
	}
	return slices.ContainsFunc(ssaflow.CallBindings(common, callee, closure), func(pair ssaflow.CallBinding) bool {
		return !search.budget.Spend() || derives(pair.Supplied) && search.use(callee, pair.Local, kind) == actionUnknown
	})
}

// Opaque and launched calls expose the same possible handoff through their
// evaluated arguments or lexical captures. No completion is implied.
func helperCallCarries(common *ssa.CallCommon, closure *ssa.MakeClosure, derives func(ssa.Value) bool) bool {
	return slices.ContainsFunc(common.Args, derives) || closure != nil && slices.ContainsFunc(closure.Bindings, derives)
}

// receiverCallRetainsNothing recognizes the documented sync.WaitGroup methods
// and lifecycle methods used as acceptance evidence: they observe or settle the
// receiver without letting it escape.
func receiverCallRetainsNothing(common *ssa.CallCommon, kind trackedKind, derives func(ssa.Value) bool) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !derives(receiver) {
		return false
	}
	switch kind {
	case trackedGroup:
		return waitGroupMethod(common)
	case trackedOwner:
		return lifecycleMethod(ssaflow.CallName(common))
	case trackedSignal:
	}
	return false
}
