package goroutineownership

import (
	"slices"

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
// The callee joins only when its observation of the value covers every normal
// return, which is what makes a deferred helper or cleanup callback
// equivalent to an inline join. Storing, sending, returning, capturing, or
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
		// Exhaustion cannot establish either a join or the absence of an
		// opaque handoff. Unknown suppresses an unjoined-worker diagnostic.
		if reason == ssaflow.SummaryBudgetExhausted {
			return actionUnknown
		}
		// No join or escape witness was established by the cut itself.
		return actionNone
	})
}

func (search *helperSearch) searchUse(function *ssa.Function, local ssa.Value, kind trackedKind) ownershipAction {
	derives := func(value ssa.Value) bool {
		return ssaflow.ValueDerivesFrom(value, local, map[ssa.Value]bool{})
	}
	joins := func(instruction ssa.Instruction) bool {
		proof := proveSummaryJoin(search.concurrency, instruction, local, kind, search.budget)
		return proof.joined || search.instructionJoins(instruction, kind, derives)
	}
	// A select case can lead directly into a shared return block. Carry the
	// receive on its edge instead of lending it to other incoming paths.
	joinsEdge := func(from, to *ssa.BasicBlock) bool {
		if kind != trackedSignal || !search.budget.Spend() {
			return false
		}
		channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
		return selected && derives(channel)
	}
	joined, escaped := false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !search.budget.Spend() {
				return actionUnknown
			}
			joined = joined || joins(instruction)
			escaped = escaped || search.instructionEscapes(instruction, local, kind, derives)
		}
		for _, successor := range block.Succs {
			joined = joinsEdge(block, successor) || joined
		}
	}
	joinProven := joined && !ssaflow.UnownedReturnFromEntryWithEdges(function, joins, joinsEdge)
	if search.budget.Exhausted() {
		return actionUnknown
	}
	if joinProven {
		return actionJoin
	}
	if escaped {
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
	if _, launched := instruction.(*ssa.Go); launched || callee == nil {
		return false
	}
	return slices.ContainsFunc(ssaflow.CallBindings(common, callee, closure), func(pair ssaflow.CallBinding) bool {
		return search.budget.Spend() && derives(pair.Supplied) && search.use(callee, pair.Local, kind) == actionJoin
	})
}

// receiverJoins recognizes the receiver-side join for each tracked kind: Wait
// on a group, or a lifecycle method on an owner.
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
		return ssaflow.ReturnedValueOwnsValue(typed, local) || slices.ContainsFunc(typed.Results, func(value ssa.Value) bool {
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
	_, launched := instruction.(*ssa.Go)
	if launched || callee == nil || len(callee.Blocks) == 0 {
		return slices.ContainsFunc(common.Args, derives) || closure != nil && slices.ContainsFunc(closure.Bindings, derives)
	}
	return slices.ContainsFunc(ssaflow.CallBindings(common, callee, closure), func(pair ssaflow.CallBinding) bool {
		return !search.budget.Spend() || derives(pair.Supplied) && search.use(callee, pair.Local, kind) == actionUnknown
	})
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
