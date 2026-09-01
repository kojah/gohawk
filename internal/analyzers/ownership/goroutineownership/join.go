package goroutineownership

import (
	"go/token"
	"go/types"
	"maps"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

// Join evidence ties a completion signal or wait group back to the goroutine
// that owns it. The search follows concrete closure bindings, helper calls, and
// stored callbacks; unresolved aliasing is not accepted as a proven join.

func spawnedOwnershipValue(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	instruction ssa.Instruction,
) (signal, group ssa.Value) { //nolint:ireturn // Goroutine ownership can flow through channels or synchronization values.
	if send, ok := instruction.(*ssa.Send); ok {
		return ssautil.SpawnedValueAtCall(spawn, function, closure, send.Chan), nil
	}
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return nil, nil
	}
	if ssautil.CallMatchesSymbol(common, analysisutil.Builtin("close")) {
		if len(common.Args) == 1 {
			return ssautil.SpawnedValueAtCall(spawn, function, closure, common.Args[0]), nil
		}
		return nil, nil
	}
	if ssautil.CallMatchesSymbol(common, analysisutil.PackageMethod(analysisutil.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Done"})) {
		receiver := ssautil.CallReceiver(common)
		return nil, ssautil.SpawnedValueAtCall(spawn, function, closure, receiver)
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
		if ok && closureEventuallyObservesAny(closure, signals) {
			return true
		}
	}
	return false
}

func closureEventuallyObservesAny(closure *ssa.MakeClosure, signals []ssa.Value) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if callEventuallyObservesAny(ssautil.InstructionCall(instruction), function, closure, signals) {
				return true
			}
		}
	}
	return false
}

func callEventuallyObservesAny(
	common *ssa.CallCommon,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
) bool {
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	for argumentIndex, argument := range common.Args {
		if argumentIndex >= len(callee.Params) ||
			!functionReceivesParameter(callee, callee.Params[argumentIndex], map[*ssa.Function]bool{}) {
			continue
		}
		if argumentDerivesFromCapturedSignal(argument, function, closure, signals) {
			return true
		}
	}
	return false
}

func argumentDerivesFromCapturedSignal(
	argument ssa.Value,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
) bool {
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) &&
			ssautil.ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) &&
			ssautil.SameAsAny(ssautil.CapturedBindingValue(closure.Bindings[index]), signals) {
			return true
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
				if index < len(callee.Params) && ssautil.ValueDerivesFrom(argument, parameter, map[ssa.Value]bool{}) &&
					functionReceivesParameter(callee, callee.Params[index], seen) {
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
	if !callUsesSignals(common, closure, signals) {
		return false
	}
	return functionReceivesSignals(function, common, closure, signals)
}

func callUsesSignals(common *ssa.CallCommon, closure *ssa.MakeClosure, signals []ssa.Value) bool {
	for _, argument := range common.Args {
		if ssautil.SameAsAny(argument, signals) {
			return true
		}
	}
	if closure == nil {
		return false
	}
	for _, binding := range closure.Bindings {
		for _, signal := range signals {
			if ssautil.CapturedBindingMatches(binding, signal) {
				return true
			}
		}
	}
	return false
}

func functionReceivesSignals(function *ssa.Function, common *ssa.CallCommon, closure *ssa.MakeClosure, signals []ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for _, channel := range receiveChannels(candidate) {
				if receivedChannelMatchesSignals(channel, function, common, closure, signals) {
					return true
				}
			}
		}
	}
	return false
}

func receiveChannels(instruction ssa.Instruction) []ssa.Value {
	switch typed := instruction.(type) {
	case *ssa.UnOp:
		if typed.Op == token.ARROW {
			return []ssa.Value{typed.X}
		}
	case *ssa.Select:
		channels := make([]ssa.Value, 0, len(typed.States))
		for _, state := range typed.States {
			if state.Dir == types.RecvOnly {
				channels = append(channels, state.Chan)
			}
		}
		return channels
	}
	return nil
}

func receivedChannelMatchesSignals(
	channel ssa.Value,
	function *ssa.Function,
	common *ssa.CallCommon,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
) bool {
	for index, parameter := range function.Params {
		if index < len(common.Args) &&
			ssautil.ValueAliases(channel, parameter, map[ssa.Value]bool{}) &&
			ssautil.SameAsAny(common.Args[index], signals) {
			return true
		}
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) &&
			ssautil.ValueAliases(channel, free, map[ssa.Value]bool{}) &&
			bindingMatchesAnySignal(closure.Bindings[index], signals) {
			return true
		}
	}
	return false
}

func bindingMatchesAnySignal(binding ssa.Value, signals []ssa.Value) bool {
	for _, signal := range signals {
		if ssautil.CapturedBindingMatches(binding, signal) {
			return true
		}
	}
	return false
}

func valueReceivesAny(value ssa.Value, signals []ssa.Value, seen map[ssa.Value]bool) bool {
	// Callback joins may be stored before invocation or nested in another
	// closure. Follow only concrete allocation stores and closure environments so
	// an unrelated callback with a similar shape cannot satisfy the obligation.
	return valueReceivesAnyWithBindings(value, signals, seen, nil)
}

func valueReceivesAnyWithBindings(value ssa.Value, signals []ssa.Value, seen map[ssa.Value]bool, enclosingBindings map[ssa.Value]ssa.Value) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Alloc:
		return storedValueReceivesAny(typed, signals, seen, enclosingBindings)
	case *ssa.MakeClosure:
		return closureReceivesAny(typed, signals, seen, enclosingBindings)
	}
	return false
}

func storedValueReceivesAny(
	address ssa.Value,
	signals []ssa.Value,
	seen map[ssa.Value]bool,
	bindings map[ssa.Value]ssa.Value,
) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && valueReceivesAnyWithBindings(store.Val, signals, seen, bindings) {
			return true
		}
	}
	return false
}

func closureReceivesAny(
	closure *ssa.MakeClosure,
	signals []ssa.Value,
	seen map[ssa.Value]bool,
	enclosingBindings map[ssa.Value]ssa.Value,
) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return false
	}
	bindings := closureBindings(function, closure, enclosingBindings)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if closureInstructionReceivesAny(instruction, function, closure, signals, seen, bindings) {
				return true
			}
		}
	}
	return false
}

// A nested closure captures its parent's FreeVar SSA node, not the concrete
// binding held by the outer closure. Carry that environment forward so a
// receive several closures deep remains tied to the exact completion channel.
func closureBindings(
	function *ssa.Function,
	closure *ssa.MakeClosure,
	enclosing map[ssa.Value]ssa.Value,
) map[ssa.Value]ssa.Value {
	bindings := make(map[ssa.Value]ssa.Value, len(enclosing)+len(function.FreeVars))
	maps.Copy(bindings, enclosing)
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) {
			bindings[free] = resolveClosureBinding(closure.Bindings[index], enclosing)
		}
	}
	return bindings
}

func closureInstructionReceivesAny(
	instruction ssa.Instruction,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
	seen map[ssa.Value]bool,
	bindings map[ssa.Value]ssa.Value,
) bool {
	if nested, ok := instruction.(*ssa.MakeClosure); ok && valueReceivesAnyWithBindings(nested, signals, seen, bindings) {
		return true
	}
	if closureCallReceivesAny(ssautil.InstructionCall(instruction), function, closure, signals, seen, bindings) {
		return true
	}
	for _, channel := range receiveChannels(instruction) {
		if closureChannelMatchesSignal(channel, function, closure, signals, bindings) {
			return true
		}
	}
	return false
}

func closureCallReceivesAny(
	common *ssa.CallCommon,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
	seen map[ssa.Value]bool,
	bindings map[ssa.Value]ssa.Value,
) bool {
	if common == nil {
		return false
	}
	if callee := common.StaticCallee(); callee != nil {
		for index, argument := range common.Args {
			if index < len(callee.Params) &&
				functionReceivesParameter(callee, callee.Params[index], map[*ssa.Function]bool{}) &&
				argumentDerivesFromClosureSignal(argument, function, closure, signals, bindings) {
				return true
			}
		}
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) &&
			ssautil.ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) &&
			valueReceivesAnyWithBindings(bindings[free], signals, seen, bindings) {
			return true
		}
	}
	return false
}

func argumentDerivesFromClosureSignal(
	argument ssa.Value,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
	bindings map[ssa.Value]ssa.Value,
) bool {
	for index, free := range function.FreeVars {
		if index >= len(closure.Bindings) || !ssautil.ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) {
			continue
		}
		binding := resolveClosureBinding(bindings[free], bindings)
		if ssautil.SameAsAny(binding, signals) || bindingMatchesAnySignal(binding, signals) {
			return true
		}
	}
	return false
}

func closureChannelMatchesSignal(
	channel ssa.Value,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	signals []ssa.Value,
	bindings map[ssa.Value]ssa.Value,
) bool {
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) &&
			ssautil.ValueAliases(channel, free, map[ssa.Value]bool{}) &&
			bindingMatchesAnySignal(bindings[free], signals) {
			return true
		}
	}
	return false
}

func resolveClosureBinding(
	value ssa.Value,
	enclosingBindings map[ssa.Value]ssa.Value,
) ssa.Value { //nolint:ireturn // Closure bindings retain their concrete SSA representation.
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
