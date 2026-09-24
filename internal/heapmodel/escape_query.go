package heapmodel

import (
	"go/types"
	"sort"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Escape queries describe publication of exact local objects, not ownership
// transfer or compiler allocation decisions. Events are may-path observations;
// none establishes that another participant accepts a cleanup obligation.

// EscapeOutcome distinguishes local confinement, observed publication, and
// uncertainty. Observed does not mean publication happens on every path.
type EscapeOutcome uint8

const (
	EscapeUnknown EscapeOutcome = iota
	EscapeLocal
	EscapeObserved
)

// EscapeScope selects the publication boundaries included in the question.
type EscapeScope uint8

const (
	// EscapeFunction includes returned values and publications during the body.
	EscapeFunction EscapeScope = iota
	// EscapeBody excludes result edges. It cannot prove confinement beyond a
	// normal return, but answers whether another participant can retain a local
	// container during execution, as needed for iteration-local cleanup.
	EscapeBody
)

// EscapeDestination identifies the boundary an object may cross.
type EscapeDestination uint8

const (
	EscapeToResult EscapeDestination = iota + 1
	EscapeToGlobal
	EscapeToField
	EscapeToChannel
	EscapeToCall
	EscapeToGoroutine
	EscapeToOpaqueRepresentation
)

// EscapeReason describes why confinement was established or declined.
type EscapeReason uint8

const (
	EscapeUnavailable EscapeReason = iota
	EscapeIdentityUnknown
	EscapeRecorded
	EscapeConfined
	EscapeOpaqueCall
	EscapeReachabilityUnknown
	EscapeBudgetExhausted
	EscapeRepresentationUnknown
)

// String renders a reason only at the diagnostic or observation boundary.
func (reason EscapeReason) String() string {
	switch reason {
	case EscapeUnavailable:
		return "escape-unavailable"
	case EscapeIdentityUnknown:
		return "escape-identity-unknown"
	case EscapeRecorded:
		return "escape-recorded"
	case EscapeConfined:
		return "escape-confined"
	case EscapeOpaqueCall:
		return "escape-opaque-call"
	case EscapeReachabilityUnknown:
		return "escape-reachability-unknown"
	case EscapeBudgetExhausted:
		return "escape-budget-exhausted"
	case EscapeRepresentationUnknown:
		return "escape-representation-unknown"
	default:
		return "invalid-escape-reason"
	}
}

// EscapeEvent retains the origin of a possible publication. Instruction's
// control-flow context is preserved; the event is not an unconditional effect.
// Events are representative witnesses, not an exhaustive participant inventory.
type EscapeEvent struct {
	Destination EscapeDestination
	Instruction ssa.Instruction
}

// EscapeProof describes one local allocation site's publication within Scope.
// Unknown may still carry useful events. Only EscapeLocal proves confinement;
// neither Observed nor Unknown discharges a lifecycle obligation.
type EscapeProof struct {
	Outcome EscapeOutcome
	Reason  EscapeReason
	Events  []EscapeEvent
	Scope   EscapeScope
}

// QueryEscape uses the existing heap graph for identity and escape effects.
// It currently proves confinement only for one exact local allocation site.
// Loop instances share that site: confinement must hold for every instance,
// while an observed event may concern only one of them.
// Foreign identities, opaque calls, unknown contents and bounded-walk cutoffs
// remain unknown. Returns and select sends are queried separately because they
// are result/alternative edges, not unconditional heap escape effects.
func QueryEscape(value ssa.Value, scope EscapeScope) EscapeProof {
	if scope != EscapeFunction && scope != EscapeBody {
		return EscapeProof{Reason: EscapeUnavailable, Scope: scope}
	}
	graph := regionsOf(value)
	defer graph.lock()()
	set, ok := graph.pointsToUnlocked(value)
	if !ok {
		return EscapeProof{Reason: EscapeUnavailable, Scope: scope}
	}
	if len(set) != 1 {
		return EscapeProof{Reason: EscapeIdentityUnknown, Scope: scope}
	}
	var target slot
	for candidate := range set {
		target = candidate
	}
	if target.region.kind != regionSite || lastStep(target.path) == pathStar {
		return EscapeProof{Reason: EscapeIdentityUnknown, Scope: scope}
	}
	proof := EscapeProof{Outcome: EscapeLocal, Reason: EscapeConfined, Scope: scope}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	for origin, at := range graph.escapeOrigins {
		if !budget.Spend() {
			return EscapeProof{Reason: EscapeBudgetExhausted, Scope: scope}
		}
		if escapeSlotsOverlap(origin.target, target) {
			proof.add(heapEscapeDestination(origin.kind), at)
		}
	}
	graph.resultEscapeEvents(target, &proof, budget)
	sort.Slice(proof.Events, func(i, j int) bool {
		return escapeEventLess(proof.Events[i], proof.Events[j])
	})
	return proof
}

func escapeEventLess(a, b EscapeEvent) bool {
	if a.Instruction.Pos() != b.Instruction.Pos() {
		return a.Instruction.Pos() < b.Instruction.Pos()
	}
	if a.Instruction.Block().Index != b.Instruction.Block().Index {
		return a.Instruction.Block().Index < b.Instruction.Block().Index
	}
	if a.Instruction != b.Instruction {
		for _, instruction := range a.Instruction.Block().Instrs {
			if instruction == a.Instruction || instruction == b.Instruction {
				return instruction == a.Instruction
			}
		}
	}
	return a.Destination < b.Destination
}

func heapEscapeDestination(kind HeapEscape) EscapeDestination {
	switch kind {
	case HeapEscapedGlobal:
		return EscapeToGlobal
	case HeapEscapedField:
		return EscapeToField
	case HeapEscapedSend:
		return EscapeToChannel
	case HeapEscapedAsync:
		return EscapeToGoroutine
	default:
		return EscapeToCall
	}
}

func (proof *EscapeProof) add(destination EscapeDestination, at ssa.Instruction) {
	for _, event := range proof.Events {
		if event.Destination == destination && event.Instruction == at {
			return
		}
	}
	proof.Events = append(proof.Events, EscapeEvent{destination, at})
	switch destination {
	case EscapeToCall:
		proof.Outcome, proof.Reason = EscapeUnknown, EscapeOpaqueCall
	case EscapeToOpaqueRepresentation:
		proof.Outcome, proof.Reason = EscapeUnknown, EscapeRepresentationUnknown
	default:
		if proof.Outcome == EscapeLocal {
			proof.Outcome, proof.Reason = EscapeObserved, EscapeRecorded
		}
	}
}

func escapeSlotsOverlap(left, right slot) bool {
	return left.region == right.region && (slotBeneath(left.path, right.path) || slotBeneath(right.path, left.path))
}

func (graph *regionGraph) resultEscapeEvents(target slot, proof *EscapeProof, budget *ssaflow.SearchBudget) {
	for _, block := range graph.function.Blocks {
		if graph.entry[block] == nil {
			continue
		}
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				proof.Outcome, proof.Reason = EscapeUnknown, EscapeBudgetExhausted
				return
			}
			graph.opaqueRepresentationEscape(instruction, target, proof, budget)
			switch typed := instruction.(type) {
			case *ssa.Return:
				if proof.Scope == EscapeBody {
					continue
				}
				for _, value := range typed.Results {
					graph.exportEscape(value, target, instruction, EscapeToResult, proof, budget)
				}
			case *ssa.Select:
				for _, state := range typed.States {
					if state.Dir == types.SendOnly {
						graph.exportEscape(state.Send, target, instruction, EscapeToChannel, proof, budget)
					}
				}
			case *ssa.Panic:
				graph.exportEscape(typed.X, target, instruction, EscapeToCall, proof, budget)
			}
		}
	}
}

// These language intrinsics can preserve an address in a representation the
// heap graph does not track (notably string and unsafe.Pointer). Silence in
// the graph cannot establish confinement across that boundary. Symbol matching
// distinguishes the actual intrinsics from ordinary similarly named helpers.
var opaqueAddressBuiltins = []syntax.Symbol{
	syntax.Builtin("Add"), syntax.Builtin("Slice"), syntax.Builtin("SliceData"),
	syntax.Builtin("String"), syntax.Builtin("StringData"),
}

func (graph *regionGraph) opaqueRepresentationEscape(at ssa.Instruction, target slot, proof *EscapeProof, budget *ssaflow.SearchBudget) {
	if converted, ok := at.(*ssa.Convert); ok {
		if basic, ok := converted.Type().Underlying().(*types.Basic); ok && (basic.Kind() == types.UnsafePointer || basic.Kind() == types.Uintptr) {
			graph.exportEscape(converted.X, target, at, EscapeToOpaqueRepresentation, proof, budget)
		}
	}
	common := ssaflow.InstructionCall(at)
	if ssaflow.CallMatchesAnySymbol(common, opaqueAddressBuiltins...) {
		for _, argument := range common.Args {
			graph.exportEscape(argument, target, at, EscapeToOpaqueRepresentation, proof, budget)
		}
	}
}

func (graph *regionGraph) exportEscape(
	value ssa.Value, target slot, at ssa.Instruction, destination EscapeDestination, proof *EscapeProof, budget *ssaflow.SearchBudget,
) {
	if !tracked(value.Type()) {
		return
	}
	state := graph.stateAt(at)
	if state == nil {
		proof.Outcome, proof.Reason = EscapeUnknown, EscapeReachabilityUnknown
		return
	}
	found, complete := graph.escapeReachability(state, graph.pointees(value), target, budget)
	if found {
		proof.add(destination, at)
	}
	if !complete {
		proof.Outcome, proof.Reason = EscapeUnknown, EscapeReachabilityUnknown
		if budget.Exhausted() {
			proof.Reason = EscapeBudgetExhausted
		}
	}
}

// escapeReachability reads existing edges. It does not infer ownership or add
// a second heap graph. Missing/backed/foreign contents prevent negative proofs.
func (graph *regionGraph) escapeReachability(state *regionState, roots pointees, target slot, budget *ssaflow.SearchBudget) (bool, bool) {
	seen := map[slot]bool{}
	queue := []slot{}
	complete := true
	found := false
	for root, stale := range roots {
		queue = append(queue, root)
		complete = complete && !stale
	}
	for len(queue) > 0 {
		if !budget.Spend() {
			return found, false
		}
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		if escapeSlotsOverlap(current, target) {
			found = true
			continue
		}
		if current.region.kind == regionNil {
			continue
		}
		if current.region.kind != regionSite && current.region.kind != regionClosure {
			complete = false
			continue
		}
		var known bool
		queue, known = escapeChildren(queue, state, current, budget)
		complete = complete && known
		if budget.Exhausted() {
			return found, false
		}
	}
	return found, complete
}

func escapeChildren(queue []slot, state *regionState, current slot, budget *ssaflow.SearchBudget) ([]slot, bool) {
	complete := true
	for held := range state.backing {
		if !budget.Spend() {
			return queue, false
		}
		if held.region == current.region {
			complete = false
		}
	}
	for held, contents := range state.contents {
		if !budget.Spend() {
			return queue, false
		}
		if held.region != current.region || !slotBeneath(held.path, current.path) {
			continue
		}
		for child, stale := range contents {
			queue = append(queue, child)
			complete = complete && !stale
		}
	}
	return queue, complete
}
