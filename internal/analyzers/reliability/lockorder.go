package reliability

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/analysisutil/ssa"

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
	functions, err := ssautil.SourceSSAFunctions(pass)
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
	callerOwned := callerOwnedLocks(function)
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
					if !slices.Contains(deferred, identity) && !returnedUnlockOwner(returned, lockValues[identity]) {
						unreleasedReturns[identity] = appendUniquePosition(unreleasedReturns[identity], returned.Pos())
					}
				}
			}
			// An unconditional unlock at the start of a spawned closure transfers
			// the held lock to that goroutine. Requiring it before any branch keeps
			// conditional handoffs from hiding a genuinely unreleased return path.
			if _, ok := instruction.(*ssa.Go); ok {
				for _, identity := range slices.Clone(held) {
					for _, value := range lockValues[identity] {
						if ssautil.ClosureCallsMethodBeforeBranch(instruction, "Unlock", value) || ssautil.ClosureCallsMethodBeforeBranch(instruction, "RUnlock", value) {
							released[identity] = true
							held = releaseLock(held, identity)
							delete(guards, identity)
							break
						}
					}
				}
			}
			// Treat an Unlock inside a deferred closure as return-path cleanup even
			// when guarded by state. This supports early-unlock patterns where the
			// defer handles only earlier returns:
			// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
			if _, ok := instruction.(*ssa.Defer); ok {
				for _, identity := range slices.Clone(held) {
					for _, value := range lockValues[identity] {
						common := ssautil.InstructionCall(instruction)
						if ssautil.DeferredClosureCalls(instruction, "Unlock", value) || ssautil.DeferredClosureCalls(instruction, "RUnlock", value) ||
							common != nil && (ssautil.ValueCallsMethod(common.Value, "Unlock", value) || ssautil.ValueCallsMethod(common.Value, "RUnlock", value)) {
							released[identity] = true
							deferred = appendUniqueString(deferred, identity)
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
				// A mutex selected from a map, slice, or loop-carried value may
				// represent a different runtime lock on every iteration. Collapsing
				// those values into one SSA identity creates both recursive-acquire
				// and missing-release false positives, so require a stable identity
				// before reasoning about its lifecycle. ccLoad coordinates a dynamic
				// set of pending calls this way:
				// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/sdk_runner.go#L410-L470
				if dynamicIndexedMutex(receiver) {
					continue
				}
				// A release before the first acquisition means this helper borrowed
				// a caller-held lock. Reacquiring restores the caller's state; it does
				// not make the helper responsible for a subsequent unlock. Kubernetes
				// drops a device-manager mutex around an RPC using this pattern:
				// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/cm/devicemanager/manager.go#L1065-L1075
				if callerOwned[identity] {
					continue
				}
				if condition != "" {
					guards[identity] = lockGuard{condition: condition, value: state.conditionValue}
				}
				if acquiredAt[identity] == token.NoPos {
					acquiredAt[identity] = instruction.Pos()
				}
				held = acquireLock(pass, instruction, held, identity, relations, false)
			case mutexRelease:
				released[identity] = true
				if _, isDefer := instruction.(*ssa.Defer); isDefer {
					// A deferred unlock remains effective on every later trip around a
					// control-flow loop. Keeping identities unique lets the dataflow
					// state reach a fixed point instead of growing without bound.
					deferred = appendUniqueString(deferred, identity)
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
	// Lock/Unlock helpers commonly and intentionally return while transferring
	// the critical section to their caller. Without interprocedural call-site
	// evidence, reporting those helpers would trade precision for recall.
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

func callerOwnedLocks(function *ssa.Function) map[string]bool {
	type firstAction struct {
		operation mutexOperation
		position  token.Pos
	}
	first := map[string]firstAction{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			operation, identity, _, ok := mutexAction(instruction)
			if !ok || instruction.Pos() == token.NoPos {
				continue
			}
			current, exists := first[identity]
			if !exists || instruction.Pos() < current.position {
				first[identity] = firstAction{operation: operation, position: instruction.Pos()}
			}
		}
	}
	result := map[string]bool{}
	for identity, action := range first {
		result[identity] = action.operation == mutexRelease
	}
	return result
}

func acquireLock(pass *analysis.Pass, instruction ssa.Instruction, held []string, identity string, relations map[lockRelation]token.Pos, dynamicIndex bool) []string {
	if slices.Contains(held, identity) {
		// Re-entering one acquisition site while ranging over a lock slice can
		// represent a different lock on every iteration. Kubernetes' GlobalLock
		// intentionally acquires every stripe this way:
		// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/images/pullmanager/locks.go#L56-L65
		if !dynamicIndex {
			reportf(pass, checkLockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
		}
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

func appendUniquePosition(positions []token.Pos, candidate token.Pos) []token.Pos {
	if !slices.Contains(positions, candidate) {
		return append(positions, candidate)
	}
	return positions
}

func appendUniqueString(values []string, candidate string) []string {
	if !slices.Contains(values, candidate) {
		return append(values, candidate)
	}
	return values
}

func returnedUnlockOwner(returned *ssa.Return, values []ssa.Value) bool {
	for _, result := range returned.Results {
		for _, value := range values {
			if ssautil.ValueCallsMethod(result, "Unlock", value) || ssautil.ValueCallsMethod(result, "RUnlock", value) {
				return true
			}
		}
	}
	return false
}

func dynamicIndexedMutex(value ssa.Value) bool {
	return dynamicIndexedMutexSeen(value, make(map[ssa.Value]bool))
}

func dynamicIndexedMutexSeen(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil {
		return false
	}
	if seen[value] {
		// Phi nodes and interface conversions can form SSA cycles. Revisiting a
		// value adds no new evidence; the other reachable edges still determine
		// whether the mutex originated from a dynamic collection selection.
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.IndexAddr:
		_, constant := typed.Index.(*ssa.Const)
		return !constant
	case *ssa.Index, *ssa.Lookup:
		return true
	case *ssa.Extract:
		return dynamicIndexedMutexSeen(typed.Tuple, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if dynamicIndexedMutexSeen(edge, seen) {
				return true
			}
		}
	case *ssa.FieldAddr:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.ChangeInterface:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.ChangeType:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.Convert:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.MakeInterface:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.UnOp:
		return dynamicIndexedMutexSeen(typed.X, seen)
	}
	return false
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
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return 0, "", nil, false
	}
	name := ssautil.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", nil, false
	}
	receiver := ssautil.CallReceiver(common)
	receiver = concreteMutexReceiver(receiver, map[ssa.Value]bool{})
	if receiver == nil {
		return 0, "", nil, false
	}
	identity := lockIdentity(receiver, map[ssa.Value]bool{})
	return operation, identity, receiver, identity != ""
}

// concreteMutexReceiver unwraps interface values only when every possible SSA
// origin proves the same concrete sync mutex identity.
func concreteMutexReceiver(value ssa.Value, seen map[ssa.Value]bool) ssa.Value { //nolint:ireturn // SSA values have several concrete forms.
	if value == nil || seen[value] {
		return nil
	}
	seen[value] = true
	if analysisutil.NamedType(value.Type(), "sync", "Mutex") || analysisutil.NamedType(value.Type(), "sync", "RWMutex") {
		return value
	}
	switch typed := value.(type) {
	case *ssa.MakeInterface:
		return concreteMutexReceiver(typed.X, seen)
	case *ssa.ChangeInterface:
		return concreteMutexReceiver(typed.X, seen)
	case *ssa.Phi:
		var resolved ssa.Value
		var identity string
		for _, edge := range typed.Edges {
			candidate := concreteMutexReceiver(edge, maps.Clone(seen))
			candidateIdentity := lockIdentity(candidate, map[ssa.Value]bool{})
			if candidate == nil || candidateIdentity == "" || identity != "" && candidateIdentity != identity {
				return nil
			}
			resolved = candidate
			identity = candidateIdentity
		}
		return resolved
	default:
		return nil
	}
}

func appendLockValue(values []ssa.Value, candidate ssa.Value) []ssa.Value {
	for _, value := range values {
		if ssautil.AliasesValue(value, candidate) {
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
	case *ssa.IndexAddr:
		owner := lockIdentity(typed.X, seen)
		index := lockIdentity(typed.Index, seen)
		if owner != "" && index != "" {
			return owner + "[" + index + "]"
		}
	case *ssa.Index:
		owner := lockIdentity(typed.X, seen)
		index := lockIdentity(typed.Index, seen)
		if owner != "" && index != "" {
			return owner + "[" + index + "]"
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
	case *ssa.FreeVar:
		return ""
	case *ssa.Alloc:
		return typed.Parent().String() + ":local:" + typed.Comment
	case *ssa.Const:
		if typed.Value != nil {
			return "constant:" + typed.Value.ExactString()
		}
	}
	if parent := value.Parent(); parent != nil && value.Name() != "" {
		// Dynamic values of the same type can still identify different lock
		// instances. Keep their SSA identities distinct instead of collapsing
		// them to the field type. Prometheus transfers state while holding locks
		// on two alertmanagerSet values of the same type:
		// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/notifier/manager.go#L165-L180
		return parent.String() + ":value:" + value.Name()
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
