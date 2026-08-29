package general

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type lockRelation struct {
	from string
	to   string
}

type lockFlowState struct {
	block          *ssa.BasicBlock
	held           []string
	deferred       []string
	guards         map[string]lockGuard
	condition      string
	conditionValue bool
}

type lockGuard struct {
	condition string
	value     bool
}

type mutexOperation uint8

const (
	mutexAcquire mutexOperation = iota + 1
	mutexRelease
)

func lockOrderAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "lockorder",
		Doc:      "checks contradictory mutex acquisition order and unreleased return paths",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runLockOrder,
	}
}

func runLockOrder(pass *analysis.Pass) (any, error) {
	functions, err := analysisutil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	relations := map[lockRelation]token.Pos{}
	for _, function := range functions {
		walkLockOrder(pass, function, relations)
	}
	return nil, nil
}

func walkLockOrder(pass *analysis.Pass, function *ssa.Function, relations map[lockRelation]token.Pos) {
	if len(function.Blocks) == 0 {
		return
	}
	queue := []lockFlowState{{block: function.Blocks[0]}}
	seen := map[string]bool{}
	released := map[string]bool{}
	acquiredAt := map[string]token.Pos{}
	lockValues := map[string][]ssa.Value{}
	unreleasedReturns := map[string][]token.Pos{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := lockStateKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		held := slices.Clone(state.held)
		deferred := slices.Clone(state.deferred)
		guards := cloneLockGuards(state.guards)
		condition := state.condition
		if len(state.block.Preds) > 1 {
			condition = ""
		}
		for _, instruction := range state.block.Instrs {
			if returned, ok := instruction.(*ssa.Return); ok {
				for _, identity := range held {
					if !slices.Contains(deferred, identity) {
						unreleasedReturns[identity] = append(unreleasedReturns[identity], returned.Pos())
					}
				}
			}
			if _, ok := instruction.(*ssa.Go); ok {
				for _, identity := range slices.Clone(held) {
					for _, value := range lockValues[identity] {
						if analysisutil.ClosureCallsMethodBeforeBranch(instruction, "Unlock", value) || analysisutil.ClosureCallsMethodBeforeBranch(instruction, "RUnlock", value) {
							released[identity] = true
							held = releaseLock(held, identity)
							delete(guards, identity)
							break
						}
					}
				}
			}
			operation, identity, receiver, ok := mutexAction(instruction)
			if !ok {
				continue
			}
			switch operation {
			case mutexAcquire:
				lockValues[identity] = appendLockValue(lockValues[identity], receiver)
				if condition != "" {
					guards[identity] = lockGuard{condition: condition, value: state.conditionValue}
				}
				if acquiredAt[identity] == token.NoPos {
					acquiredAt[identity] = instruction.Pos()
				}
				held = acquireLock(pass, instruction, held, identity, relations)
			case mutexRelease:
				released[identity] = true
				if _, isDefer := instruction.(*ssa.Defer); isDefer {
					deferred = append(deferred, identity)
				} else {
					held = releaseLock(held, identity)
					delete(guards, identity)
				}
			}
		}
		for index, successor := range state.block.Succs {
			nextCondition := ""
			nextValue := false
			if condition, ok := blockCondition(state.block); ok && len(state.block.Succs) == 2 {
				nextCondition = condition
				nextValue = index == 0
				if guardConflicts(held, guards, condition, nextValue) {
					continue
				}
			}
			queue = append(queue, lockFlowState{
				block: successor, held: held, deferred: deferred, guards: guards,
				condition: nextCondition, conditionValue: nextValue,
			})
		}
	}
	functionName := strings.ToLower(function.Name())
	if strings.HasPrefix(functionName, "lock") || strings.HasPrefix(functionName, "unlock") {
		return
	}
	for identity, returns := range unreleasedReturns {
		if !released[identity] {
			continue
		}
		for _, position := range returns {
			if position == token.NoPos {
				position = acquiredAt[identity]
			}
			reportf(pass, checkLockMissingRelease, position, "lock %s is not released on this return path", identity)
		}
	}
}

func acquireLock(pass *analysis.Pass, instruction ssa.Instruction, held []string, identity string, relations map[lockRelation]token.Pos) []string {
	if slices.Contains(held, identity) {
		reportf(pass, checkLockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
		return held
	}
	for _, owner := range held {
		relation := lockRelation{from: owner, to: identity}
		reverse := lockRelation{from: identity, to: owner}
		if _, exists := relations[reverse]; exists {
			reportf(pass, checkLockContradictoryOrder, instruction.Pos(), "contradictory lock order: %s and %s", identity, owner)
		}
		relations[relation] = instruction.Pos()
	}
	return append(held, identity)
}

func releaseLock(held []string, identity string) []string {
	for index, candidate := range slices.Backward(held) {
		if candidate == identity {
			return append(held[:index], held[index+1:]...)
		}
	}
	return held
}

func mutexAction(instruction ssa.Instruction) (mutexOperation, string, ssa.Value, bool) {
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return 0, "", nil, false
	}
	name := analysisutil.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", nil, false
	}
	receiver := analysisutil.CallReceiver(common)
	if receiver == nil || !(analysisutil.NamedType(receiver.Type(), "sync", "Mutex") || analysisutil.NamedType(receiver.Type(), "sync", "RWMutex")) {
		return 0, "", nil, false
	}
	identity := lockIdentity(receiver, map[ssa.Value]bool{})
	return operation, identity, receiver, identity != ""
}

func appendLockValue(values []ssa.Value, candidate ssa.Value) []ssa.Value {
	for _, value := range values {
		if analysisutil.AliasesValue(value, candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func lockStateKey(state lockFlowState) string {
	guards := make([]string, 0, len(state.guards))
	for identity, guard := range state.guards {
		guards = append(guards, fmt.Sprintf("%s:%s=%t", identity, guard.condition, guard.value))
	}
	slices.Sort(guards)
	return fmt.Sprintf("%d:%s:%s:%s:%s=%t", state.block.Index, strings.Join(state.held, ","), strings.Join(state.deferred, ","), strings.Join(guards, ","), state.condition, state.conditionValue)
}

func cloneLockGuards(source map[string]lockGuard) map[string]lockGuard {
	result := make(map[string]lockGuard, len(source))
	for identity, guard := range source {
		result[identity] = guard
	}
	return result
}

func guardConflicts(held []string, guards map[string]lockGuard, condition string, value bool) bool {
	for _, identity := range held {
		guard, ok := guards[identity]
		if ok && guard.condition == condition && guard.value != value {
			return true
		}
	}
	return false
}

func blockCondition(block *ssa.BasicBlock) (string, bool) {
	if len(block.Instrs) == 0 {
		return "", false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return "", false
	}
	return conditionIdentity(branch.Cond)
}

func conditionIdentity(value ssa.Value) (string, bool) {
	comparison, ok := value.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return "", false
	}
	left, leftOK := conditionOperandIdentity(comparison.X)
	right, rightOK := conditionOperandIdentity(comparison.Y)
	if !leftOK || !rightOK {
		return "", false
	}
	if right < left {
		left, right = right, left
	}
	return comparison.Op.String() + ":" + left + ":" + right, true
}

func conditionOperandIdentity(value ssa.Value) (string, bool) {
	switch typed := value.(type) {
	case *ssa.Parameter:
		return fmt.Sprintf("parameter:%p", typed), true
	case *ssa.Const:
		if typed.Value == nil {
			return "constant:nil:" + types.TypeString(typed.Type(), nil), true
		}
		return "constant:" + typed.Value.ExactString(), true
	default:
		return fmt.Sprintf("%T:%p", value, value), true
	}
}

func lockIdentity(value ssa.Value, seen map[ssa.Value]bool) string {
	if value == nil || seen[value] {
		return ""
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Global:
		return typed.Name()
	case *ssa.FieldAddr:
		field := structField(typed.X.Type(), typed.Field)
		if field != nil {
			if owner := lockIdentity(typed.X, seen); owner != "" {
				return owner + "." + field.Name()
			}
			return types.TypeString(typed.X.Type(), nil) + "." + field.Name()
		}
	case *ssa.ChangeInterface:
		return lockIdentity(typed.X, seen)
	case *ssa.ChangeType:
		return lockIdentity(typed.X, seen)
	case *ssa.Convert:
		return lockIdentity(typed.X, seen)
	case *ssa.MakeInterface:
		return lockIdentity(typed.X, seen)
	case *ssa.UnOp:
		return lockIdentity(typed.X, seen)
	case *ssa.Parameter:
		return typed.Parent().String() + "." + typed.Name()
	case *ssa.Alloc:
		return typed.Parent().String() + ":local:" + typed.Comment
	}
	return ""
}

func structField(value types.Type, index int) *types.Var {
	if pointer, ok := value.Underlying().(*types.Pointer); ok {
		value = pointer.Elem()
	}
	structure, ok := value.Underlying().(*types.Struct)
	if !ok || index < 0 || index >= structure.NumFields() {
		return nil
	}
	return structure.Field(index)
}
