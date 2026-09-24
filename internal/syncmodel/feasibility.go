package syncmodel

import (
	"fmt"
	"go/constant"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Feasibility decides whether the branch choices a graph variant assumes can
// all hold in one execution. It proves contradiction exactly: one condition
// value, evaluated once on an acyclic path of one call, cannot take both
// polarities, and one value cannot equal two different constants. It proves
// feasibility only under a conservative independence policy. Conditions on
// distinct parameters of the root, or on results of calls to distinct
// functions, can vary independently, as gohawk's other checks already treat
// an error result. Two results of one function may be equal. A condition read
// from memory, merged by a phi, or bound from a callee may correlate with any
// other condition, pure ones included (a worker's bound copy of the parent's
// flag is the same variable), so it is feasible only as the sole condition.
// Anything beyond that is unknown, never assumed feasible.

// Feasibility reports whether the variant's conditions can hold together.
func (graph *SyncGraph) Feasibility() Proof {
	atoms := map[string]*atomChoices{}
	for _, condition := range graph.Conditions {
		atom, choice, known := normalizeCondition(condition)
		if !known {
			return queryProof(ssaflow.EvidenceUnknown, ReasonConditionsUnknown)
		}
		if atom.key == constantAtom {
			if !choice.holds {
				return queryProof(ssaflow.EvidenceDisproven, ReasonConditionsContradict)
			}
			continue
		}
		if atoms[atom.key] == nil {
			atoms[atom.key] = &atomChoices{atom: atom}
		}
		if !atoms[atom.key].add(choice) {
			return queryProof(ssaflow.EvidenceDisproven, ReasonConditionsContradict)
		}
	}
	if !independent(atoms) {
		return queryProof(ssaflow.EvidenceUnknown, ReasonConditionsCorrelated)
	}
	return queryProof(ssaflow.EvidenceProven, ReasonConditionsFeasible)
}

// constantAtom keys every constant condition: requiring a constant to be what
// it is not contradicts, and requiring what it is adds nothing.
const constantAtom = "const"

// conditionAtom is the value a branch tests, in one call context. Shared
// atoms may correlate with others; pure atoms are independent inputs.
type conditionAtom struct {
	key    string
	shared bool
	callee *ssa.Function
}

// choice is what a branch requires of its atom: a Boolean value, or equality
// or inequality with a constant.
type choice struct {
	holds    bool
	constant constant.Value
}

type atomChoices struct {
	atom    conditionAtom
	boolean *bool
	equal   constant.Value
	unequal []constant.Value
}

func (choices *atomChoices) add(next choice) bool {
	if next.constant == nil {
		if choices.boolean != nil && *choices.boolean != next.holds {
			return false
		}
		choices.boolean = &next.holds
		return true
	}
	if next.holds {
		if choices.equal != nil && !constant.Compare(choices.equal, token.EQL, next.constant) {
			return false
		}
		if slices.ContainsFunc(choices.unequal, func(value constant.Value) bool {
			return constant.Compare(value, token.EQL, next.constant)
		}) {
			return false
		}
		choices.equal = next.constant
		return true
	}
	if choices.equal != nil && constant.Compare(choices.equal, token.EQL, next.constant) {
		return false
	}
	choices.unequal = append(choices.unequal, next.constant)
	return true
}

// normalizeCondition peels negation and comparison with a constant, folds a
// constant condition, and classifies what remains.
func normalizeCondition(condition concurrencyfacts.Condition) (conditionAtom, choice, bool) {
	value, holds := condition.Value, condition.Holds
	for {
		negation, ok := value.(*ssa.UnOp)
		if !ok || negation.Op != token.NOT {
			break
		}
		value, holds = negation.X, !holds
	}
	next := choice{holds: holds}
	if folded, ok := value.(*ssa.Const); ok && folded.Value != nil && folded.Value.Kind() == constant.Bool {
		// A constant atom is its own evidence: key it by value so a false
		// requirement of true contradicts.
		return conditionAtom{key: constantAtom}, choice{holds: constant.BoolVal(folded.Value) == holds}, true
	}
	aspect := ""
	if comparison, ok := value.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
		operand, compared, isNil := constantComparison(comparison)
		equal := holds == (comparison.Op == token.EQL)
		switch {
		case isNil:
			// Nilness is a Boolean property of the operand, such as err != nil.
			value, aspect, next = operand, "nil", choice{holds: equal}
		case compared != nil:
			value, next = operand, choice{holds: equal, constant: compared}
		}
	}
	if value == nil || value.Type() == nil {
		return conditionAtom{}, choice{}, false
	}
	return classifyAtom(value, condition.Context, aspect), next, true
}

// constantComparison splits a comparison into its variable operand and the
// constant it is compared with, reporting a nil constant separately because
// SSA gives nil no constant value.
func constantComparison(comparison *ssa.BinOp) (ssa.Value, constant.Value, bool) {
	for _, pair := range [][2]ssa.Value{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
		folded, ok := pair[1].(*ssa.Const)
		if !ok {
			continue
		}
		if folded.IsNil() {
			return pair[0], nil, true
		}
		if folded.Value != nil {
			return pair[0], folded.Value, false
		}
	}
	return nil, nil, false
}

// classifyAtom keys a value by identity and call context. A root parameter or
// a call result is a pure input; anything bound from a callee, read from
// memory, or merged is shared.
func classifyAtom(value ssa.Value, context []token.Pos, aspect string) conditionAtom {
	contexts := make([]string, 0, len(context))
	for _, site := range context {
		contexts = append(contexts, strconv.Itoa(int(site)))
	}
	// Identity, not spelling: two values with one name in different
	// functions are different atoms.
	atom := conditionAtom{key: fmt.Sprintf("%p#%s#%s", value, strings.Join(contexts, ","), aspect)}
	if len(context) != 0 {
		atom.shared = true
		return atom
	}
	switch value := value.(type) {
	case *ssa.Parameter:
		return atom
	case *ssa.Call:
		atom.callee = value.Common().StaticCallee()
		atom.shared = atom.callee == nil
		return atom
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			atom.callee = call.Common().StaticCallee()
			atom.shared = atom.callee == nil
			return atom
		}
	}
	atom.shared = true
	return atom
}

// independent applies the policy: a shared atom only as the sole variable
// atom, and no two pure atoms that are results of the same function.
func independent(atoms map[string]*atomChoices) bool {
	shared, variable := 0, 0
	callees := map[*ssa.Function]int{}
	for _, choices := range atoms {
		variable++
		if choices.atom.shared {
			shared++
			continue
		}
		if choices.atom.callee != nil {
			callees[choices.atom.callee]++
		}
	}
	if shared > 0 && variable > 1 {
		return false
	}
	for _, count := range callees {
		if count > 1 {
			return false
		}
	}
	return true
}

// ExecutionGroup is one choice of branch paths. Its graphs share the same
// conditions and differ only in select arms, which the runtime chooses while
// it waits on all of them at once, so a property of the group must hold on
// every graph in it. Different groups are different executions: one feasible
// group is enough to show that some execution has the property.
type ExecutionGroup struct {
	Graphs      []SyncGraph
	Feasibility Proof
}

// ExecutionGroups partitions graph variants by their conditions, in order.
func ExecutionGroups(graphs []SyncGraph) []ExecutionGroup {
	var groups []ExecutionGroup
	index := map[string]int{}
	for _, graph := range graphs {
		key := conditionsKey(graph.Conditions)
		position, seen := index[key]
		if !seen {
			position = len(groups)
			index[key] = position
			groups = append(groups, ExecutionGroup{Feasibility: graph.Feasibility()})
		}
		groups[position].Graphs = append(groups[position].Graphs, graph)
	}
	return groups
}

func conditionsKey(conditions []concurrencyfacts.Condition) string {
	parts := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		parts = append(parts, fmt.Sprintf("%p/%t/%v", condition.Value, condition.Holds, condition.Context))
	}
	return strings.Join(parts, ";")
}

// ExecutionVerdict says whether some feasible execution satisfied a proof.
type ExecutionVerdict uint8

const (
	// ExecutionNotFound means no group proved the property; the returned
	// proof is the first group's failure.
	ExecutionNotFound ExecutionVerdict = iota
	// ExecutionFeasibilityUnknown means only groups whose conditions could
	// not be shown feasible proved it.
	ExecutionFeasibilityUnknown
	// ExecutionFound means a group proved feasible proved the property.
	ExecutionFound
)

// SomeFeasibleExecution is the one decision for "some execution has this
// property": prove runs a check's own every-graph proof on one group, and
// infeasible groups are skipped.
func SomeFeasibleExecution[P any](graphs []SyncGraph, prove func([]SyncGraph) P, proven func(P) bool) (P, ExecutionVerdict) {
	var first, pending P
	verdict, failed := ExecutionNotFound, false
	for _, group := range ExecutionGroups(graphs) {
		if group.Feasibility.State == ssaflow.EvidenceDisproven {
			continue
		}
		proof := prove(group.Graphs)
		switch {
		case proven(proof) && group.Feasibility.Proven():
			return proof, ExecutionFound
		case proven(proof):
			pending, verdict = proof, ExecutionFeasibilityUnknown
		case !failed:
			first, failed = proof, true
		}
	}
	if verdict == ExecutionFeasibilityUnknown {
		return pending, verdict
	}
	return first, ExecutionNotFound
}
