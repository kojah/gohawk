package lockorder

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

	proofs "github.com/kojah/gohawk/internal/proof"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Lock states retain bounded exact branch evidence separately from possible
// release witnesses. Mutable loaded guards can justify uncertainty, never a
// stable value or a guaranteed unlock on a later path.

// A return can merge paths that retained a lock with paths that released it.
// Possible retention identifies violations; only definite retention can
// establish a held-for-caller contract. Both come from the same flow states.
type lockReturnState struct {
	possible []string
	definite []string
}

func lockStateKey(state lockFlowState, budget *proofs.SearchBudget) string {
	for _, values := range [][]string{state.readHeld, state.deferred} {
		for range values {
			if !budget.Spend() {
				return ""
			}
		}
	}
	predecessor := -1
	if state.predecessor != nil {
		predecessor = state.predecessor.Index
	}
	var origins strings.Builder
	for _, identity := range state.held {
		if !budget.Spend() {
			return ""
		}
		origin := state.origins[identity]
		fmt.Fprintf(&origins, "%s:%d:%t;", identity, origin.position, origin.read)
	}
	// Map iteration must not distinguish equivalent paths. Canonical guard
	// order keeps revisits finite; a cutoff key cannot stand for this state.
	guards := make([]string, 0, len(state.guards))
	for identity, guard := range state.guards {
		if !budget.Spend() {
			return ""
		}
		guards = append(guards, fmt.Sprintf("%s:%s=%t", identity, guard.condition, guard.value))
	}
	slices.Sort(guards)
	for _, binding := range state.constants {
		if !budget.Spend() {
			return ""
		}
		fmt.Fprintf(&origins, "phi:%p=%s;", binding.value, binding.literal.Value.ExactString())
	}
	stable := state.constraints.KeyWithin(budget)
	if budget.Exhausted() {
		return ""
	}
	fmt.Fprintf(&origins, "stable:%s;", stable)
	return fmt.Sprintf(
		"%d:%d:%s:%s:%s:%s:%s=%t:%s",
		state.block.Index,
		predecessor,
		strings.Join(state.held, ","),
		strings.Join(state.readHeld, ","),
		strings.Join(state.deferred, ","),
		strings.Join(guards, ","),
		state.condition,
		state.conditionValue,
		origins.String(),
	)
}

// Carry a bounded set of exact Boolean/integer literals, not assumptions about
// mutable receiver fields. A release flag can pass through unrelated blocks
// before its next test; discarding its selected phi edge invents lock states.
// Refresh all phis simultaneously on entry, including forgetting an old value
// when a loop supplies an unknown input. Four bindings bound extra path state.
// https://github.com/buchgr/bazel-remote/blob/a69b6b5ed933234d93b489ffd216bee5bb74aa06/cache/disk/disk.go#L450-L564
// A loop phase can retain a literal while a lock is held and become unknown
// after release. This tracks that exact value, not arithmetic or loop counts:
// https://github.com/tidwall/uhaha/blob/5ea77162763891837176b90e111fcac74678143e/uhaha.go#L4173-L4217
func lockPhiConstants(state lockFlowState, budget *proofs.SearchBudget) []lockScalarConstant {
	const maxConstants = 4
	for range state.constants {
		if !budget.Spend() {
			return nil
		}
	}
	next := slices.Clone(state.constants)
	for _, instruction := range state.block.Instrs {
		if !budget.Spend() {
			return nil
		}
		phi, ok := instruction.(*ssa.Phi)
		if !ok {
			break
		}
		next = slices.DeleteFunc(next, func(binding lockScalarConstant) bool { return binding.value == phi })
		for predecessor, incoming := range ssaflow.PhiIncoming(phi) {
			if !budget.Spend() {
				return nil
			}
			if predecessor != state.predecessor {
				continue
			}
			if literal := lockLiteralValue(incoming, state.constants); literal != nil && len(next) < maxConstants {
				next = append(next, lockScalarConstant{value: phi, literal: literal})
			}
		}
	}
	return next
}

func lockBooleanValue(value ssa.Value, constants []lockScalarConstant) (bool, bool) {
	if literal := lockLiteralValue(value, constants); literal != nil && literal.Value.Kind() == constant.Bool {
		return constant.BoolVal(literal.Value), true
	}
	comparison, ok := value.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	left, right := lockLiteralValue(comparison.X, constants), lockLiteralValue(comparison.Y, constants)
	if left == nil || right == nil || left.Value.Kind() != right.Value.Kind() {
		return false, false
	}
	return constant.Compare(left.Value, comparison.Op, right.Value), true
}

func lockLiteralValue(value ssa.Value, constants []lockScalarConstant) *ssa.Const {
	if literal, ok := value.(*ssa.Const); ok && literal.Value != nil {
		if kind := literal.Value.Kind(); kind == constant.Bool || kind == constant.Int {
			return literal
		}
	}
	for _, binding := range constants {
		if value == binding.value {
			return binding.literal
		}
	}
	return nil
}

func constantFalse(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && literal.Value.Kind() == constant.Bool && !constant.BoolVal(literal.Value)
}

// Stable comparisons of parameters and constants survive unrelated branches
// and merges, so a later branch on the same comparison that takes the other
// arm is infeasible. The shared path-guard engine decides identity; this
// walk keeps only stable guards, because a loaded guard's contradiction is
// uncertainty rather than infeasibility, and never excludes a path on it.
// Exceeding the guard limit forgets the new fact, never excludes a path.
// https://github.com/yandex-cloud/geesefs/blob/dd847771b29b26f3246edaf3227acbc430f4548d/core/file.go#L1901-L1972
func extendLockConstraints(constraints ssaflow.PathGuards, block *ssa.BasicBlock, truth bool, budget *proofs.SearchBudget) (ssaflow.PathGuards, bool) {
	if len(block.Succs) != 2 {
		return constraints, true
	}
	successor := block.Succs[1]
	if truth {
		successor = block.Succs[0]
	}
	next, contradiction := constraints.ExtendWithin(block, successor, func(guard ssaflow.PathGuard) bool { return guard.Stable }, budget)
	return next, contradiction != ssaflow.GuardStableContradiction
}

func guardConflicts(held []string, guards map[string]lockGuard, condition string, value bool, budget *proofs.SearchBudget) bool {
	for _, identity := range held {
		if !budget.Spend() {
			return false
		}
		guard, ok := guards[identity]
		if ok && guard.condition == condition && guard.value != value {
			return true
		}
	}
	return false
}

func blockCondition(block *ssa.BasicBlock, budget *proofs.SearchBudget) (string, bool) {
	if len(block.Instrs) == 0 {
		return "", false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return "", false
	}
	return conditionIdentity(branch.Cond, budget)
}

func conditionIdentity(value ssa.Value, budget *proofs.SearchBudget) (string, bool) {
	if !budget.Spend() {
		return "", false
	}
	// A computed Boolean outside a cycle is evaluated once. Repeating that
	// exact SSA value cannot change its truth, including a short-circuit phi.
	// Do not correlate a loop instruction across iterations: its next dynamic
	// evaluation may differ even though its SSA node is the same.
	// https://github.com/pb33f/libopenapi/blob/07795ddc2c097af8581138ef290d6cf964110d74/index/extract_refs_lookup.go#L199-L220
	// Bare Boolean parameters are stable for the invocation too. Only exact
	// SSA identities are correlated: other loads or iterations remain distinct.
	_, parameter := value.(*ssa.Parameter)
	_, comparisonValue := value.(*ssa.BinOp)
	instruction, computed := value.(ssa.Instruction)
	stableComputed := computed && !comparisonValue && !ssaflow.BlockInCycleWithin(instruction.Block(), budget)
	if budget.Exhausted() {
		return "", false
	}
	if parameter || stableComputed {
		basic, boolean := value.Type().Underlying().(*types.Basic)
		if boolean && basic.Info()&types.IsBoolean != 0 {
			return "boolean:" + conditionOperandIdentity(value), true
		}
	}
	comparison, ok := value.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return "", false
	}
	left := conditionOperandIdentity(comparison.X)
	right := conditionOperandIdentity(comparison.Y)
	if right < left {
		left, right = right, left
	}
	return comparison.Op.String() + ":" + left + ":" + right, true
}

func conditionOperandIdentity(value ssa.Value) string {
	switch typed := value.(type) {
	case *ssa.Parameter:
		return fmt.Sprintf("parameter:%p", typed)
	case *ssa.Const:
		if typed.Value == nil {
			return "constant:nil:" + types.TypeString(typed.Type(), nil)
		}
		return "constant:" + typed.Value.ExactString()
	default:
		return fmt.Sprintf("%T:%p", value, value)
	}
}

func traceInfeasibleLockBranch(pass *analysis.Pass, block *ssa.BasicBlock, reason lockReason) {
	checkID := string(check.LockMissingRelease)
	if !analysisTrace.Enabled("lockorder", checkID) || len(block.Instrs) == 0 {
		return
	}
	branch := block.Instrs[len(block.Instrs)-1]
	position := branch.Pos()
	if position == token.NoPos {
		position = branch.Parent().Pos()
	}
	analysisTrace.For(pass, "lockorder", checkID, position).Evidence(analysisTrace.Step{
		Reason:   reason.String(),
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      position,
		Function: branch.Parent().String(),
	})
}

func traceLockStateBudget(pass *analysis.Pass, function *ssa.Function) {
	probe := analysisTrace.For(pass, "lockorder", string(check.LockMissingRelease), function.Pos())
	if !probe.Enabled() {
		return
	}
	probe.Decision(analysisTrace.Step{
		Reason: lockReasonLockStateBudgetExhausted.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: function.Pos(),
		Function: function.String(),
	})
}

func traceFreshMutexIdentity(pass *analysis.Pass, instruction ssa.Instruction, receiver ssa.Value) {
	checkID := string(check.LockContradictoryOrder)
	if !analysisTrace.Enabled("lockorder", checkID) {
		return
	}
	if proof := possibleFreshMutexField(receiver); proof.possible {
		analysisTrace.For(pass, "lockorder", checkID, instruction.Pos()).Decision(analysisTrace.Step{
			Reason: proof.reason.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: instruction.Pos(),
		})
	}
}
