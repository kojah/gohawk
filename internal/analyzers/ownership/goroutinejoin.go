package ownership

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

func spawnedClosure(spawn *ssa.Go) (*ssa.Function, *ssa.MakeClosure, bool) {
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return nil, nil, false
	}
	function, ok := closure.Fn.(*ssa.Function)
	return function, closure, ok
}

func spawnedOwnershipValue(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, instruction ssa.Instruction) (signal, group ssa.Value) { //nolint:ireturn // Goroutine ownership can flow through channels or synchronization values.
	if send, ok := instruction.(*ssa.Send); ok {
		return spawnedValueAtCall(spawn, function, closure, send.Chan), nil
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return nil, nil
	}
	switch ssautil.CallName(common) {
	case analysisutil.BuiltinClose:
		if len(common.Args) == 1 {
			return spawnedValueAtCall(spawn, function, closure, common.Args[0]), nil
		}
	case "Done":
		receiver := ssautil.CallReceiver(common)
		if receiver != nil && analysisutil.NamedType(receiver.Type(), "sync", "WaitGroup") {
			return nil, spawnedValueAtCall(spawn, function, closure, receiver)
		}
	}
	return nil, nil
}

func joinsGoroutine(instruction ssa.Instruction, signals, groups []ssa.Value) bool {
	if callReceivesAny(instruction, signals) {
		return true
	}
	if receive, ok := instruction.(*ssa.UnOp); ok && receive.Op == token.ARROW {
		return aliasesAny(receive.X, signals)
	}
	if selection, ok := instruction.(*ssa.Select); ok {
		for _, state := range selection.States {
			if state.Dir == types.RecvOnly && aliasesAny(state.Chan, signals) {
				return true
			}
		}
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if ssautil.CallName(common) == "Wait" && aliasesAny(ssautil.CallReceiver(common), groups) {
		return true
	}
	lower := strings.ToLower(ssautil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if aliasesAny(argument, signals) || aliasesAny(argument, groups) {
			return true
		}
	}
	return false
}

func callReceivesAny(instruction ssa.Instruction, signals []ssa.Value) bool {
	// Joining through a local helper closure is semantically the same as an
	// inline receive. Chi uses this shape to collect every request worker:
	// https://github.com/go-chi/chi/blob/36611d24579aaca3250ed9732e17e085c5026334/middleware/throttle_test.go#L282-L315
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return false
	}
	if closure != nil && valueReceivesAny(closure, signals, map[ssa.Value]bool{}) {
		return true
	}
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	usesSignal := false
	for _, argument := range common.Args {
		usesSignal = usesSignal || aliasesAny(argument, signals)
	}
	if closure != nil {
		for _, binding := range closure.Bindings {
			for _, signal := range signals {
				usesSignal = usesSignal || ssautil.CapturedBindingAliases(binding, signal)
			}
		}
	}
	if !usesSignal {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			var channels []ssa.Value
			switch typed := candidate.(type) {
			case *ssa.UnOp:
				if typed.Op == token.ARROW {
					channels = append(channels, typed.X)
				}
			case *ssa.Select:
				for _, state := range typed.States {
					if state.Dir == types.RecvOnly {
						channels = append(channels, state.Chan)
					}
				}
			}
			for _, channel := range channels {
				for index, parameter := range function.Params {
					if passedValueAliases(channel, parameter, map[ssa.Value]bool{}) && index < len(common.Args) && aliasesAny(common.Args[index], signals) {
						return true
					}
				}
				for index, free := range function.FreeVars {
					if closure != nil && passedValueAliases(channel, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) {
						for _, signal := range signals {
							if ssautil.CapturedBindingAliases(closure.Bindings[index], signal) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func valueReceivesAny(value ssa.Value, signals []ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && valueReceivesAny(store.Val, signals, seen) {
					return true
				}
			}
		}
	case *ssa.MakeClosure:
		function, _ := typed.Fn.(*ssa.Function)
		if function == nil {
			return false
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if nested, ok := instruction.(*ssa.MakeClosure); ok && valueReceivesAny(nested, signals, seen) {
					return true
				}
				common := ssautil.InstructionCall(instruction)
				if common != nil {
					for index, free := range function.FreeVars {
						if index < len(typed.Bindings) && ssautil.AliasesValue(common.Value, free) && valueReceivesAny(typed.Bindings[index], signals, seen) {
							return true
						}
					}
				}
				var channels []ssa.Value
				switch receive := instruction.(type) {
				case *ssa.UnOp:
					if receive.Op == token.ARROW {
						channels = append(channels, receive.X)
					}
				case *ssa.Select:
					for _, state := range receive.States {
						if state.Dir == types.RecvOnly {
							channels = append(channels, state.Chan)
						}
					}
				}
				for _, channel := range channels {
					for index, free := range function.FreeVars {
						if index >= len(typed.Bindings) || !passedValueAliases(channel, free, map[ssa.Value]bool{}) {
							continue
						}
						for _, signal := range signals {
							if ssautil.CapturedBindingAliases(typed.Bindings[index], signal) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func aliasesAny(value ssa.Value, targets []ssa.Value) bool {
	for _, target := range targets {
		if ssautil.AliasesValue(value, target) {
			return true
		}
	}
	return false
}
