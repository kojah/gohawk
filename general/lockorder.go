package general

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/checkutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type lockRelation struct {
	from string
	to   string
}

type lockFlowState struct {
	block *ssa.BasicBlock
	held  []string
}

type mutexOperation uint8

const (
	mutexAcquire mutexOperation = iota + 1
	mutexRelease
)

func lockOrderAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "lockorder",
		Doc:      "checks contradictory mutex acquisition order",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runLockOrder,
	}
}

func runLockOrder(pass *analysis.Pass) (any, error) {
	relations := map[lockRelation]token.Pos{}
	for _, function := range checkutil.SourceSSAFunctions(pass) {
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
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := fmt.Sprintf("%d:%s", state.block.Index, strings.Join(state.held, ","))
		if seen[key] {
			continue
		}
		seen[key] = true
		held := slices.Clone(state.held)
		for _, instruction := range state.block.Instrs {
			operation, identity, ok := mutexAction(instruction)
			if !ok {
				continue
			}
			switch operation {
			case mutexAcquire:
				held = acquireLock(pass, instruction, held, identity, relations)
			case mutexRelease:
				held = releaseLock(held, identity)
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, lockFlowState{block: successor, held: held})
		}
	}
}

func acquireLock(pass *analysis.Pass, instruction ssa.Instruction, held []string, identity string, relations map[lockRelation]token.Pos) []string {
	if slices.Contains(held, identity) {
		pass.Reportf(instruction.Pos(), "lock %s is acquired while already held", identity)
		return held
	}
	for _, owner := range held {
		relation := lockRelation{from: owner, to: identity}
		reverse := lockRelation{from: identity, to: owner}
		if _, exists := relations[reverse]; exists {
			pass.Reportf(instruction.Pos(), "contradictory lock order: %s and %s", identity, owner)
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

func mutexAction(instruction ssa.Instruction) (mutexOperation, string, bool) {
	if _, deferred := instruction.(*ssa.Defer); deferred {
		return 0, "", false
	}
	common := checkutil.InstructionCall(instruction)
	if common == nil {
		return 0, "", false
	}
	name := checkutil.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", false
	}
	receiver := checkutil.CallReceiver(common)
	if receiver == nil || !(checkutil.NamedType(receiver.Type(), "sync", "Mutex") || checkutil.NamedType(receiver.Type(), "sync", "RWMutex")) {
		return 0, "", false
	}
	identity := lockIdentity(receiver, map[ssa.Value]bool{})
	return operation, identity, identity != ""
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
