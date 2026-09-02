package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Guarded-join evidence recognizes a join that the function skips under a
// guard decided by the same control flow that decided to launch: a Boolean
// flag or an integer counter assigned a constant, or stepped by one, in a
// block the spawn dominates, is dominated by, or shares, and then read by a
// branch that selects between the join and skipping it. The proof is purely
// structural. It never compares how many workers were launched with how many
// joins run, and its only outcome is Unknown: a guarded join suppresses the
// diagnostic because the correlation is not modeled, it does not prove the
// join runs. A counted receive loop is the same shape read at the loop
// header, where the loop bound, not the induction variable, is the counter.

// flagGuardedJoin reports whether a join reachable from the spawn sits under a
// branch on a local Boolean flag that receives a constant on a path through
// the spawn. The assignment may run before or after the launch; what matters
// is that the guard's value is decided by the same control flow that decided
// to launch.
func (analysis *spawnAnalysis) flagGuardedJoin() bool {
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if deferred, ok := instruction.(*ssa.Defer); ok && analysis.deferredClosureFlagGuardsJoin(deferred) {
				return true
			}
			branch, ok := instruction.(*ssa.If)
			if !ok || !ssaflow.InstructionMayFollow(analysis.spawn, branch) || !analysis.branchGuardsJoin(branch) {
				continue
			}
			if flag := booleanFlagVariable(branch.Cond); flag != nil && analysis.flagAssignedAroundSpawn(flag) {
				return true
			}
			if counter := integerCounterVariable(branch); counter != nil && analysis.counterAssignedAroundSpawn(counter, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

// deferredClosureFlagGuardsJoin recognizes the same correlation inside a
// deferred closure: `defer func() { if wait4compl { wg.Wait() } }()` with
// wait4compl assigned beside the launch. gocoin verifies transaction scripts
// this way:
// https://github.com/piotrnar/gocoin/blob/467131d21dd0c9252f99c00d99ba9ca74f60ca1e/lib/chain/chain_accept.go#L125-L133
func (analysis *spawnAnalysis) deferredClosureFlagGuardsJoin(deferred *ssa.Defer) bool {
	callee, closure := calledFunction(deferred.Common())
	if closure == nil || callee == nil {
		return false
	}
	pairs := suppliedValues(deferred.Common(), callee, closure)
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			branch, ok := instruction.(*ssa.If)
			if !ok {
				continue
			}
			flag := capturedFlagVariable(branch.Cond, pairs)
			if flag == nil || !analysis.flagAssignedAroundSpawn(flag) {
				continue
			}
			for _, successor := range branch.Block().Succs {
				if analysis.closureBlockJoins(successor, pairs) {
					return true
				}
			}
		}
	}
	return false
}

// capturedFlagVariable maps a branch on a loaded free variable back to the
// Boolean local it was bound to.
func capturedFlagVariable(condition ssa.Value, pairs []suppliedValue) ssa.Value { //nolint:ireturn // Flags are allocations.
	if negation, ok := condition.(*ssa.UnOp); ok && negation.Op == token.NOT {
		condition = negation.X
	}
	load, ok := condition.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	for _, pair := range pairs {
		if pair.local == load.X {
			if flag, ok := pair.supplied.(*ssa.Alloc); ok && isBooleanPointer(flag.Type()) {
				return flag
			}
		}
	}
	return nil
}

// closureBlockJoins reports whether block joins a tracked value through one of
// the closure's captured variables.
func (analysis *spawnAnalysis) closureBlockJoins(block *ssa.BasicBlock, pairs []suppliedValue) bool {
	for _, pair := range pairs {
		for _, tracked := range analysis.tracked {
			if !bindingCarries(pair.supplied, tracked.value) {
				continue
			}
			derives := func(value ssa.Value) bool {
				return ssaflow.ValueDerivesFrom(value, pair.local, map[ssa.Value]bool{})
			}
			for _, instruction := range block.Instrs {
				if instructionJoins(instruction, tracked.kind, derives, map[*ssa.Function]bool{}) {
					return true
				}
			}
		}
	}
	return false
}

// branchGuardsJoin reports whether a successor of branch contains a join,
// transfer, or opaque handoff of a tracked value that the other successor can
// skip. A launched waiter counts: the guard decides whether the parent hands
// the group to it.
func (analysis *spawnAnalysis) branchGuardsJoin(branch *ssa.If) bool {
	for _, successor := range branch.Block().Succs {
		for _, instruction := range successor.Instrs {
			if instruction != analysis.spawn && analysis.action(instruction) != actionNone {
				return true
			}
		}
	}
	return false
}

// integerCounterVariable returns the integer local a comparison reads, such
// as the pending count in `if pending == 0` or the loop bound in
// `i < started`. A counter incremented beside the spawn correlates the guarded
// join with the launch the same way a Boolean flag does. sidecar counts its
// watchers and skips the launched waiter when none started:
// https://github.com/marcus/sidecar/blob/9b8739f753ab235dda2630676833e9b46a52696c/internal/plugins/conversations/plugin_loading.go#L568-L602
func integerCounterVariable(branch *ssa.If) ssa.Value { //nolint:ireturn // Counters are phis, loads, or arithmetic.
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok {
		return nil
	}
	switch comparison.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return nil
	}
	for _, operand := range []ssa.Value{comparison.X, comparison.Y} {
		// The loop's own induction variable is a phi in the branch's block; it
		// counts iterations, not launches, so it never correlates with the spawn.
		if phi, ok := operand.(*ssa.Phi); ok && phi.Block() == branch.Block() {
			continue
		}
		if isIntegerLocal(operand) {
			return operand
		}
	}
	return nil
}

func isIntegerLocal(value ssa.Value) bool {
	basic, ok := value.Type().Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsInteger == 0 {
		return false
	}
	switch typed := value.(type) {
	case *ssa.Phi, *ssa.BinOp:
		return true
	case *ssa.UnOp:
		_, addressable := typed.X.(*ssa.Alloc)
		return typed.Op == token.MUL && addressable
	}
	return false
}

// counterAssignedAroundSpawn reports whether the counter is set or stepped by
// a constant in a block that the spawn dominates, is dominated by, or shares.
func (analysis *spawnAnalysis) counterAssignedAroundSpawn(counter ssa.Value, seen map[ssa.Value]bool) bool {
	if seen[counter] {
		return false
	}
	seen[counter] = true
	switch typed := counter.(type) {
	case *ssa.Phi:
		for index, edge := range typed.Edges {
			if index < len(typed.Block().Preds) && isConstantStep(edge) && analysis.blockAroundSpawn(typed.Block().Preds[index]) {
				return true
			}
			if analysis.counterAssignedAroundSpawn(edge, seen) {
				return true
			}
		}
	case *ssa.BinOp:
		if isConstantStep(typed) && analysis.blockAroundSpawn(typed.Block()) {
			return true
		}
		return analysis.counterAssignedAroundSpawn(typed.X, seen) || analysis.counterAssignedAroundSpawn(typed.Y, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() == nil {
			return false
		}
		for _, reference := range *typed.X.Referrers() {
			store, ok := reference.(*ssa.Store)
			if ok && store.Addr == typed.X && isConstantStep(store.Val) && analysis.blockAroundSpawn(store.Block()) {
				return true
			}
		}
	}
	return false
}

// isConstantStep accepts an integer constant or an addition or subtraction of
// one, the forms `count = 0` and `count++` lower to.
func isConstantStep(value ssa.Value) bool {
	if _, ok := value.(*ssa.Const); ok {
		return true
	}
	step, ok := value.(*ssa.BinOp)
	if !ok || step.Op != token.ADD && step.Op != token.SUB {
		return false
	}
	_, left := step.X.(*ssa.Const)
	_, right := step.Y.(*ssa.Const)
	return left || right
}

// booleanFlagVariable returns the Boolean flag a branch condition reads,
// following one negation. A flag that no closure captures is an SSA phi of
// constants; a captured one is an addressable local.
func booleanFlagVariable(condition ssa.Value) ssa.Value { //nolint:ireturn // Flags are phis or allocations.
	if negation, ok := condition.(*ssa.UnOp); ok && negation.Op == token.NOT {
		condition = negation.X
	}
	if phi, ok := condition.(*ssa.Phi); ok && phiCarriesBooleanConstant(phi, map[*ssa.Phi]bool{}) {
		return phi
	}
	load, ok := condition.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	if flag, ok := load.X.(*ssa.Alloc); ok && isBooleanPointer(flag.Type()) {
		return flag
	}
	return nil
}

// phiCarriesBooleanConstant reports whether a Boolean constant reaches phi,
// possibly through the nested phis a loop with continue statements creates.
func phiCarriesBooleanConstant(phi *ssa.Phi, seen map[*ssa.Phi]bool) bool {
	if seen[phi] {
		return false
	}
	seen[phi] = true
	return slices.ContainsFunc(phi.Edges, func(edge ssa.Value) bool {
		if inner, ok := edge.(*ssa.Phi); ok {
			return phiCarriesBooleanConstant(inner, seen)
		}
		return isBooleanConstant(edge)
	})
}

func isBooleanConstant(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil {
		return false
	}
	basic, ok := literal.Type().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func isBooleanPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	basic, ok := pointer.Elem().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

// flagAssignedAroundSpawn reports whether a constant reaches the flag from a
// block that the spawn dominates, is dominated by, or shares.
func (analysis *spawnAnalysis) flagAssignedAroundSpawn(flag ssa.Value) bool {
	if phi, ok := flag.(*ssa.Phi); ok {
		return analysis.phiConstantAroundSpawn(phi, map[*ssa.Phi]bool{})
	}
	if flag.Referrers() == nil {
		return false
	}
	for _, reference := range *flag.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == flag && isBooleanConstant(store.Val) && analysis.blockAroundSpawn(store.Block()) {
			return true
		}
	}
	return false
}

func (analysis *spawnAnalysis) phiConstantAroundSpawn(phi *ssa.Phi, seen map[*ssa.Phi]bool) bool {
	if seen[phi] {
		return false
	}
	seen[phi] = true
	for index, edge := range phi.Edges {
		if index >= len(phi.Block().Preds) {
			break
		}
		if inner, ok := edge.(*ssa.Phi); ok && analysis.phiConstantAroundSpawn(inner, seen) {
			return true
		}
		if isBooleanConstant(edge) && analysis.blockAroundSpawn(phi.Block().Preds[index]) {
			return true
		}
	}
	return false
}

func (analysis *spawnAnalysis) blockAroundSpawn(block *ssa.BasicBlock) bool {
	spawnBlock := analysis.spawn.Block()
	return block == spawnBlock || block.Dominates(spawnBlock) || spawnBlock.Dominates(block)
}

func (analysis *spawnAnalysis) countedJoin() bool {
	if !ssaflow.BlockInCycle(analysis.spawn.Block()) {
		return false
	}
	for _, block := range analysis.function.Blocks {
		if !ssaflow.BlockInCycle(block) {
			continue
		}
		for _, instruction := range block.Instrs {
			if ssaflow.InstructionMayFollow(analysis.spawn, instruction) && analysis.action(instruction) == actionJoin {
				return true
			}
		}
	}
	return false
}
