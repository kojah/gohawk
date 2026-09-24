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
// all hold in one execution. Condition identity and contradiction come from
// ssaflow's path guards, the same ones lockorder and resourcelifetime prune
// with: taking both arms of a stable guard (a parameter, a constant, or a
// value computed once) proves the variant cannot happen, while both arms of a
// guard read from memory is only unknown, since an unseen store could explain
// it. One value equal to two different constants also contradicts.
//
// Feasibility itself is claimed only under a conservative independence
// policy, which is this package's sufficiency decision rather than a shared
// mechanic. Conditions on distinct root parameters, or on results of calls to
// distinct functions, can vary independently, as gohawk's other checks
// already treat an error result. Two results of one function may be equal. A
// condition read from memory, computed from other values, without an
// identity, or bound from a callee may correlate with any other condition,
// pure ones included (a worker's bound copy of the parent's flag is the same
// variable), so it is feasible only as the sole condition. Anything else is
// unknown, never assumed feasible.

// Feasibility reports whether the variant's conditions can hold together.
func (graph *SyncGraph) Feasibility() Proof {
	atoms := map[string]*atomChoices{}
	for _, condition := range graph.Conditions {
		if folded, ok := condition.Value.(*ssa.Const); ok && folded.Value != nil && folded.Value.Kind() == constant.Bool {
			// Requiring a constant to be what it is not contradicts outright.
			if constant.BoolVal(folded.Value) != condition.Holds {
				return queryProof(ssaflow.EvidenceDisproven, ReasonConditionsContradict)
			}
			continue
		}
		atom, value := guardAtom(condition)
		if atoms[atom.key] == nil {
			atoms[atom.key] = &atomChoices{atom: atom}
		}
		switch atoms[atom.key].add(value, atom.stable) {
		case choiceContradicts:
			return queryProof(ssaflow.EvidenceDisproven, ReasonConditionsContradict)
		case choiceUncertain:
			return queryProof(ssaflow.EvidenceUnknown, ReasonConditionsCorrelated)
		case choiceConsistent:
		}
		if equality, ok := constantEquality(condition); ok {
			if !recordEquality(atoms, equality) {
				return queryProof(ssaflow.EvidenceDisproven, ReasonConditionsContradict)
			}
		}
	}
	if !independent(atoms) {
		return queryProof(ssaflow.EvidenceUnknown, ReasonConditionsCorrelated)
	}
	return queryProof(ssaflow.EvidenceProven, ReasonConditionsFeasible)
}

// conditionAtom is one guard in one call context. Shared atoms may correlate
// with others; pure atoms are independent inputs.
type conditionAtom struct {
	key    string
	stable bool
	shared bool
	callee *ssa.Function
}

type choiceResult uint8

const (
	choiceConsistent choiceResult = iota
	choiceContradicts
	choiceUncertain
)

type atomChoices struct {
	atom    conditionAtom
	value   *bool
	equal   constant.Value
	unequal []constant.Value
}

func (choices *atomChoices) add(value, stable bool) choiceResult {
	switch {
	case choices.value == nil:
		choices.value = &value
		return choiceConsistent
	case *choices.value == value:
		return choiceConsistent
	case stable:
		return choiceContradicts
	default:
		return choiceUncertain
	}
}

// guardAtom names a condition by its shared guard identity within its call
// context. A condition with no guard identity is keyed by itself and shared.
func guardAtom(condition concurrencyfacts.Condition) (conditionAtom, bool) {
	context := contextKey(condition.Context)
	identity, negated, stable, ok := ssaflow.GuardCondition(condition.Value)
	if !ok {
		return conditionAtom{key: fmt.Sprintf("%p#%s", condition.Value, context), shared: true}, condition.Holds
	}
	atom := conditionAtom{key: identity + "#" + context, stable: stable}
	atom.shared, atom.callee = inputKind(condition.Value, len(condition.Context) != 0)
	return atom, condition.Holds != negated
}

// inputKind classifies what a condition depends on: a root parameter is a
// pure input, a comparison of root parameters and constants is too, and a
// call result is pure but remembers its function. Everything else, and any
// condition bound from a callee, is shared.
func inputKind(value ssa.Value, bound bool) (bool, *ssa.Function) {
	if bound {
		return true, nil
	}
	for {
		negation, ok := value.(*ssa.UnOp)
		if !ok || negation.Op != token.NOT {
			break
		}
		value = negation.X
	}
	if comparison, ok := value.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
		shared, callee := operandKind(comparison.X)
		otherShared, otherCallee := operandKind(comparison.Y)
		if callee != nil && otherCallee != nil {
			return true, nil
		}
		if callee == nil {
			callee = otherCallee
		}
		return shared || otherShared, callee
	}
	return operandKind(value)
}

func operandKind(value ssa.Value) (bool, *ssa.Function) {
	switch value := value.(type) {
	case *ssa.Parameter, *ssa.Const:
		return false, nil
	case *ssa.Call:
		callee := value.Common().StaticCallee()
		return callee == nil, callee
	case *ssa.Extract:
		if call, ok := value.Tuple.(*ssa.Call); ok {
			callee := call.Common().StaticCallee()
			return callee == nil, callee
		}
	}
	return true, nil
}

// equality is a comparison of one value with a non-nil constant.
type equality struct {
	subject  string
	constant constant.Value
	equal    bool
}

func constantEquality(condition concurrencyfacts.Condition) (equality, bool) {
	comparison, ok := condition.Value.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return equality{}, false
	}
	subject, folded := comparison.X, comparison.Y
	if _, left := subject.(*ssa.Const); left {
		subject, folded = folded, subject
	}
	literal, ok := folded.(*ssa.Const)
	if !ok || literal.Value == nil {
		return equality{}, false
	}
	return equality{
		subject:  fmt.Sprintf("%p#%s", subject, contextKey(condition.Context)),
		constant: literal.Value, equal: condition.Holds == (comparison.Op == token.EQL),
	}, true
}

// recordEquality rejects a value required to equal two different constants,
// or to equal and not equal the same one.
func recordEquality(atoms map[string]*atomChoices, next equality) bool {
	key := "equality:" + next.subject
	if atoms[key] == nil {
		atoms[key] = &atomChoices{atom: conditionAtom{key: key, stable: true}}
	}
	choices := atoms[key]
	matches := func(value constant.Value) bool { return constant.Compare(value, token.EQL, next.constant) }
	if next.equal {
		if choices.equal != nil && !matches(choices.equal) || slices.ContainsFunc(choices.unequal, matches) {
			return false
		}
		choices.equal = next.constant
		return true
	}
	if choices.equal != nil && matches(choices.equal) {
		return false
	}
	choices.unequal = append(choices.unequal, next.constant)
	return true
}

func contextKey(context []token.Pos) string {
	parts := make([]string, 0, len(context))
	for _, site := range context {
		parts = append(parts, strconv.Itoa(int(site)))
	}
	return strings.Join(parts, ",")
}

// independent applies the policy: a shared atom only as the sole variable
// atom, and no two pure atoms that are results of the same function.
// Equality bookkeeping entries restate atoms already counted.
func independent(atoms map[string]*atomChoices) bool {
	shared, variable := 0, 0
	callees := map[*ssa.Function]int{}
	for key, choices := range atoms {
		if strings.HasPrefix(key, "equality:") {
			continue
		}
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
