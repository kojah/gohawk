package goroutineownership

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	ssautil "github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

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
		return ssautil.SameAsAny(receive.X, signals)
	}
	if selection, ok := instruction.(*ssa.Select); ok {
		for _, state := range selection.States {
			if state.Dir == types.RecvOnly && ssautil.SameAsAny(state.Chan, signals) {
				return true
			}
		}
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if strings.Contains(strings.ToLower(ssautil.CallName(common)), "eventually") && eventuallyObservesAny(common, signals) {
		return true
	}
	if ssautil.CallName(common) == "Wait" && ssautil.SameAsAny(ssautil.CallReceiver(common), groups) {
		return true
	}
	lower := strings.ToLower(ssautil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if ssautil.SameAsAny(argument, signals) || ssautil.SameAsAny(argument, groups) {
			return true
		}
	}
	return false
}

func eventuallyObservesAny(common *ssa.CallCommon, signals []ssa.Value) bool {
	// Eventually owns a polling predicate only when that predicate reaches a
	// source-visible helper which actually observes the completion channel.
	// https://github.com/janosmiko/lfk/blob/ca9760842190011f31f9d2079425d3a313fdd4c2/internal/k8s/portforward_supersede_test.go#L50-L58
	for _, argument := range common.Args {
		closure, ok := argument.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		function, _ := closure.Fn.(*ssa.Function)
		if function == nil {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				called := ssautil.InstructionCall(instruction)
				if called == nil || called.StaticCallee() == nil {
					continue
				}
				callee := called.StaticCallee()
				for argIndex, passed := range called.Args {
					if argIndex >= len(callee.Params) || !functionReceivesParameter(callee, callee.Params[argIndex], map[*ssa.Function]bool{}) {
						continue
					}
					for freeIndex, free := range function.FreeVars {
						if freeIndex >= len(closure.Bindings) || !ssautil.ValueDerivesFrom(passed, free, map[ssa.Value]bool{}) {
							continue
						}
						if ssautil.SameAsAny(ssautil.CapturedBindingValue(closure.Bindings[freeIndex]), signals) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func functionReceivesParameter(function *ssa.Function, parameter ssa.Value, seen map[*ssa.Function]bool) bool {
	if function == nil || seen[function] {
		return false
	}
	seen[function] = true
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch candidate := instruction.(type) {
			case *ssa.UnOp:
				if candidate.Op == token.ARROW && ssautil.ValueDerivesFrom(candidate.X, parameter, map[ssa.Value]bool{}) {
					return true
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssautil.ValueDerivesFrom(state.Chan, parameter, map[ssa.Value]bool{}) {
						return true
					}
				}
			}
			called := ssautil.InstructionCall(instruction)
			if called == nil || called.StaticCallee() == nil {
				continue
			}
			callee := called.StaticCallee()
			for index, argument := range called.Args {
				if index < len(callee.Params) && ssautil.ValueDerivesFrom(argument, parameter, map[ssa.Value]bool{}) && functionReceivesParameter(callee, callee.Params[index], seen) {
					return true
				}
			}
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
		usesSignal = usesSignal || ssautil.SameAsAny(argument, signals)
	}
	if closure != nil {
		for _, binding := range closure.Bindings {
			for _, signal := range signals {
				usesSignal = usesSignal || ssautil.CapturedBindingMatches(binding, signal)
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
					if passedValueAliases(channel, parameter, map[ssa.Value]bool{}) && index < len(common.Args) && ssautil.SameAsAny(common.Args[index], signals) {
						return true
					}
				}
				for index, free := range function.FreeVars {
					if closure != nil && passedValueAliases(channel, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) {
						for _, signal := range signals {
							if ssautil.CapturedBindingMatches(closure.Bindings[index], signal) {
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
	return valueReceivesAnyWithBindings(value, signals, seen, nil)
}

func valueReceivesAnyWithBindings(value ssa.Value, signals []ssa.Value, seen map[ssa.Value]bool, enclosingBindings map[ssa.Value]ssa.Value) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && valueReceivesAnyWithBindings(store.Val, signals, seen, enclosingBindings) {
					return true
				}
			}
		}
	case *ssa.MakeClosure:
		function, _ := typed.Fn.(*ssa.Function)
		if function == nil {
			return false
		}
		// A nested closure captures its parent's FreeVar SSA node, not the
		// concrete binding held by the outer closure. Carry that environment
		// forward so a receive several closures deep can still be tied to the
		// exact completion channel produced by the goroutine.
		bindings := make(map[ssa.Value]ssa.Value, len(enclosingBindings)+len(function.FreeVars))
		for free, binding := range enclosingBindings {
			bindings[free] = binding
		}
		for index, free := range function.FreeVars {
			if index < len(typed.Bindings) {
				bindings[free] = resolveClosureBinding(typed.Bindings[index], enclosingBindings)
			}
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if nested, ok := instruction.(*ssa.MakeClosure); ok && valueReceivesAnyWithBindings(nested, signals, seen, bindings) {
					return true
				}
				common := ssautil.InstructionCall(instruction)
				if common != nil {
					if callee := common.StaticCallee(); callee != nil {
						for argumentIndex, argument := range common.Args {
							if argumentIndex >= len(callee.Params) || !functionReceivesParameter(callee, callee.Params[argumentIndex], map[*ssa.Function]bool{}) {
								continue
							}
							for freeIndex, free := range function.FreeVars {
								if freeIndex >= len(typed.Bindings) || !ssautil.ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) {
									continue
								}
								binding := resolveClosureBinding(bindings[free], bindings)
								for _, signal := range signals {
									if ssautil.SameValue(binding, signal) || ssautil.CapturedBindingMatches(binding, signal) {
										return true
									}
								}
							}
						}
					}
					for index, free := range function.FreeVars {
						if index < len(typed.Bindings) && ssautil.ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) && valueReceivesAnyWithBindings(bindings[free], signals, seen, bindings) {
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
							if ssautil.CapturedBindingMatches(bindings[free], signal) {
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

func resolveClosureBinding(value ssa.Value, enclosingBindings map[ssa.Value]ssa.Value) ssa.Value { //nolint:ireturn // Closure bindings retain their concrete SSA representation.
	seen := make(map[ssa.Value]bool)
	for value != nil && !seen[value] {
		seen[value] = true
		resolved, ok := enclosingBindings[value]
		if !ok {
			return value
		}
		value = resolved
	}
	return value
}

func nestedCallbackReceivesAny(function *ssa.Function, signals []ssa.Value) bool {
	if len(signals) == 0 {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			closure, ok := instruction.(*ssa.MakeClosure)
			if !ok {
				continue
			}
			if valueReceivesAny(closure, signals, map[ssa.Value]bool{}) {
				// A completion channel consumed by a callback has transferred its
				// join obligation to the callback's owner. Without proving that the
				// callback is abandoned, reporting the spawning function is not
				// actionable. Network Doctor uses this for probe callbacks:
				// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/diagnostic/runall_test.go#L112-L126
				return true
			}
		}
	}
	return false
}
