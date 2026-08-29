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
	block    *ssa.BasicBlock
	held     []string
	deferred []string
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
	unreleasedReturns := map[string][]token.Pos{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := fmt.Sprintf("%d:%s:%s", state.block.Index, strings.Join(state.held, ","), strings.Join(state.deferred, ","))
		if seen[key] {
			continue
		}
		seen[key] = true
		held := slices.Clone(state.held)
		deferred := slices.Clone(state.deferred)
		for _, instruction := range state.block.Instrs {
			if returned, ok := instruction.(*ssa.Return); ok {
				for _, identity := range held {
					if !slices.Contains(deferred, identity) {
						unreleasedReturns[identity] = append(unreleasedReturns[identity], returned.Pos())
					}
				}
			}
			operation, identity, ok := mutexAction(instruction)
			if !ok {
				continue
			}
			switch operation {
			case mutexAcquire:
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
				}
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, lockFlowState{block: successor, held: held, deferred: deferred})
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
			analysisutil.Reportf(pass, position, "lock %s is not released on this return path", identity)
		}
	}
}

func acquireLock(pass *analysis.Pass, instruction ssa.Instruction, held []string, identity string, relations map[lockRelation]token.Pos) []string {
	if slices.Contains(held, identity) {
		analysisutil.Reportf(pass, instruction.Pos(), "lock %s is acquired while already held", identity)
		return held
	}
	for _, owner := range held {
		relation := lockRelation{from: owner, to: identity}
		reverse := lockRelation{from: identity, to: owner}
		if _, exists := relations[reverse]; exists {
			analysisutil.Reportf(pass, instruction.Pos(), "contradictory lock order: %s and %s", identity, owner)
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
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return 0, "", false
	}
	name := analysisutil.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", false
	}
	receiver := analysisutil.CallReceiver(common)
	if receiver == nil || !(analysisutil.NamedType(receiver.Type(), "sync", "Mutex") || analysisutil.NamedType(receiver.Type(), "sync", "RWMutex")) {
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
