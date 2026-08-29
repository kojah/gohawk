package ownership

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func goroutineOwnershipAnalyzer() *analysis.Analyzer {
	config := goroutineOwnershipConfig{mode: goroutineModeContext}
	analyzer := &analysis.Analyzer{
		Name:     "goroutineownership",
		Doc:      "checks that explicit goroutines have a recognizable join handle or lifecycle owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.Var(newChoiceValue(&config.mode, goroutineModeContext, goroutineModeLifecycle, goroutineModeJoin), "mode", "ownership policy: context, lifecycle, or join")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runGoroutineOwnership(pass, config)
	}
	return analyzer
}

type goroutineOwnershipConfig struct {
	mode string
}

const (
	goroutineModeContext   = "context"
	goroutineModeLifecycle = "lifecycle"
	goroutineModeJoin      = "join"
)

func runGoroutineOwnership(pass *analysis.Pass, config goroutineOwnershipConfig) (any, error) {
	functions, err := analysisutil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportAbandonedProducerSends(pass, function)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				signals, groups := goroutineJoinValues(spawn)
				owners := goroutineLifecycleValues(spawn)
				// A received channel argument is a stop signal, not a completion
				// handle that the caller must join. Kubernetes informers commonly
				// express context ownership as Run(ctx.Done()):
				// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
				if config.mode == goroutineModeContext && (goroutineHasContextLifecycle(spawn) || goroutineHasStopLifecycle(spawn)) {
					continue
				}
				if (config.mode != goroutineModeJoin && (goroutineTransferredToCaller(function, spawn) || externallyOwnedLifecycle(owners) || externallyOwnedJoin(signals, groups))) || ownershipRegisteredBefore(spawn, signals, groups) {
					continue
				}
				// If spawning and receiving range over the same unchanged bound, any
				// execution that launches a worker also executes at least one join
				// iteration. This covers the common "N workers, then N receives"
				// pattern without assuming unrelated loops have matching counts.
				if matchingCountedJoin(function, spawn, signals) {
					continue
				}
				if analysisutil.UnownedReturn(spawn, func(candidate ssa.Instruction) bool {
					if joinsGoroutine(candidate, signals, groups) || waitsForLifecycleOwner(candidate, owners) {
						return true
					}
					if config.mode == goroutineModeJoin {
						return transfersGoroutineOwnership(candidate, signals, groups, nil)
					}
					return ownsGoroutineLifecycle(candidate, owners) || transfersGoroutineOwnership(candidate, signals, groups, owners)
				}, func(returned *ssa.Return) bool {
					return returnedAliasesAny(returned, signals) || returnedAliasesAny(returned, groups) || config.mode != goroutineModeJoin && returnedAliasesAny(returned, owners)
				}) {
					check := checkGoroutineJoin
					// Without a completion signal or wait group, static analysis cannot
					// reliably distinguish a leak from intentional component work. Keep
					// that heuristic opt-in; reserve the default check for code that
					// exposes a recognizable join mechanism and fails to honor it.
					if config.mode != goroutineModeJoin && len(signals) == 0 && len(groups) == 0 {
						check = checkGoroutineDetached
					}
					reportf(pass, check, spawn.Pos(), "goroutine is not joined on every return path")
				}
			}
		}
	}
	return nil, nil
}

type producerSend struct {
	instruction *ssa.Send
	channel     ssa.Value
	repeated    bool
	spawn       *ssa.Go
}

func reportAbandonedProducerSends(pass *analysis.Pass, function *ssa.Function) {
	var sends []producerSend
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			spawn, ok := instruction.(*ssa.Go)
			if !ok {
				continue
			}
			spawned := spawn.Common().StaticCallee()
			closure, closureOK := spawn.Common().Value.(*ssa.MakeClosure)
			if closureOK {
				spawned, _ = closure.Fn.(*ssa.Function)
			}
			if spawned == nil {
				continue
			}
			for _, spawnedBlock := range spawned.Blocks {
				for _, candidate := range spawnedBlock.Instrs {
					send, ok := candidate.(*ssa.Send)
					if !ok {
						continue
					}
					channel := spawnedValueAtCall(spawn, spawned, closure, send.Chan)
					if channel != nil && localUnbufferedChannel(function, channel) {
						sends = append(sends, producerSend{instruction: send, channel: channel, repeated: blockInCycle(spawnedBlock), spawn: spawn})
					}
				}
			}
		}
	}
	reported := map[token.Pos]bool{}
	for _, send := range sends {
		sendCount := 0
		for _, candidate := range sends {
			if analysisutil.AliasesValue(candidate.channel, send.channel) && producerSendsCanCooccur(send, candidate) {
				sendCount++
			}
		}
		receiveCount, draining := channelReceives(function, send.channel)
		if receiveCount == 0 || draining || !send.repeated && sendCount <= receiveCount || reported[send.instruction.Pos()] {
			continue
		}
		reported[send.instruction.Pos()] = true
		reportf(pass, checkGoroutineProducerSend, send.instruction.Pos(), "goroutine send can block after the receiver stops waiting")
	}
}

func producerSendsCanCooccur(first, second producerSend) bool {
	if first.spawn != second.spawn {
		return true
	}
	if first.instruction == second.instruction {
		return true
	}
	return instructionCanReach(first.instruction, second.instruction) || instructionCanReach(second.instruction, first.instruction)
}

func instructionCanReach(from, to ssa.Instruction) bool {
	if from.Parent() != to.Parent() {
		return false
	}
	if from.Block() == to.Block() {
		return analysisutil.InstructionIndex(from) < analysisutil.InstructionIndex(to)
	}
	seen := map[*ssa.BasicBlock]bool{}
	queue := slices.Clone(from.Block().Succs)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == to.Block() {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}

func spawnedValueAtCall(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) ssa.Value { //nolint:ireturn // SSA values retain their concrete representations.
	if closure != nil {
		for index, free := range function.FreeVars {
			if passedValueAliases(value, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) {
				captured := analysisutil.CapturedBindingValue(closure.Bindings[index])
				// Keep the address when the first observed value is nil. The channel
				// may be assigned only after an owner closure is created, as in
				// Kubernetes test-server teardown paths.
				if analysisutil.DefinitelyNil(captured) {
					return closure.Bindings[index]
				}
				return captured
			}
		}
	}
	for index, parameter := range function.Params {
		if passedValueAliases(value, parameter, map[ssa.Value]bool{}) && index < len(spawn.Common().Args) {
			return spawn.Common().Args[index]
		}
	}
	return nil
}

func passedValueAliases(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || target == nil || seen[value] {
		return false
	}
	if value == target {
		return true
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return passedValueAliases(typed.X, target, seen)
	case *ssa.ChangeType:
		return passedValueAliases(typed.X, target, seen)
	case *ssa.Convert:
		return passedValueAliases(typed.X, target, seen)
	case *ssa.MakeInterface:
		return passedValueAliases(typed.X, target, seen)
	case *ssa.UnOp:
		return typed.Op == token.MUL && passedValueAliases(typed.X, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if passedValueAliases(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

func localUnbufferedChannel(function *ssa.Function, channel ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			created, ok := instruction.(*ssa.MakeChan)
			if !ok || !analysisutil.CapturedBindingAliases(channel, created) {
				continue
			}
			size, ok := created.Size.(*ssa.Const)
			return ok && size.Value != nil && constant.Sign(size.Value) == 0
		}
	}
	return false
}

func channelReceives(function *ssa.Function, channel ssa.Value) (count int, draining bool) {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch candidate := instruction.(type) {
			case *ssa.UnOp:
				if candidate.Op == token.ARROW && analysisutil.AliasesValue(candidate.X, channel) {
					count++
					draining = draining || blockInCycle(block)
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && analysisutil.AliasesValue(state.Chan, channel) {
						count++
						draining = draining || blockInCycle(block)
					}
				}
			}
		}
	}
	return count, draining
}

func blockInCycle(start *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{}
	queue := slices.Clone(start.Succs)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == start {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}

func matchingCountedJoin(function *ssa.Function, spawn *ssa.Go, signals []ssa.Value) bool {
	spawnBound := loopBound(function, spawn.Block())
	if spawnBound == nil {
		return false
	}
	for _, block := range function.Blocks {
		joinBound := loopBound(function, block)
		if joinBound == nil || !analysisutil.AliasesValue(spawnBound, joinBound) {
			continue
		}
		for _, instruction := range block.Instrs {
			receive, ok := instruction.(*ssa.UnOp)
			if ok && receive.Op == token.ARROW && aliasesAny(receive.X, signals) && receive.Pos() > spawn.Pos() {
				return true
			}
		}
	}
	return false
}

func loopBound(function *ssa.Function, body *ssa.BasicBlock) ssa.Value { //nolint:ireturn // Bounds retain their SSA form for alias comparison.
	var selected *ssa.BasicBlock
	for _, header := range function.Blocks {
		if !header.Dominates(body) || !loopHeader(header) {
			continue
		}
		if selected == nil || selected.Dominates(header) {
			selected = header
		}
	}
	if selected == nil {
		return nil
	}
	candidates := []*ssa.BasicBlock{selected}
	for _, predecessor := range selected.Preds {
		if selected.Dominates(predecessor) {
			candidates = append(candidates, predecessor)
		}
	}
	for _, candidate := range candidates {
		if bound := loopComparisonBound(candidate, selected); bound != nil {
			if call, ok := bound.(*ssa.Call); ok && analysisutil.CallName(call.Common()) == "len" && len(call.Common().Args) == 1 {
				return call.Common().Args[0]
			}
			return bound
		}
	}
	return nil
}

func loopComparisonBound(block, header *ssa.BasicBlock) ssa.Value { //nolint:ireturn // Bounds retain their SSA form for alias comparison.
	if len(block.Instrs) == 0 {
		return nil
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok {
		return nil
	}
	if induction := loopInduction(comparison.X, header); induction != nil {
		if constantZero(comparison.Y) {
			if initial := loopInitialValue(induction, header); initial != nil {
				return initial
			}
		}
		return comparison.Y
	}
	if induction := loopInduction(comparison.Y, header); induction != nil {
		if constantZero(comparison.X) {
			if initial := loopInitialValue(induction, header); initial != nil {
				return initial
			}
		}
		return comparison.X
	}
	return nil
}

func loopInitialValue(induction *ssa.Phi, header *ssa.BasicBlock) ssa.Value { //nolint:ireturn // The initial loop value retains its SSA form.
	for index, predecessor := range header.Preds {
		if index < len(induction.Edges) && !header.Dominates(predecessor) {
			return induction.Edges[index]
		}
	}
	return nil
}

func constantZero(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && constant.Sign(literal.Value) == 0
}

func loopInduction(value ssa.Value, header *ssa.BasicBlock) *ssa.Phi {
	if induction, ok := value.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	step, ok := value.(*ssa.BinOp)
	if !ok || step.Op != token.ADD {
		return nil
	}
	if induction, ok := step.X.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	if induction, ok := step.Y.(*ssa.Phi); ok && induction.Block() == header {
		return induction
	}
	return nil
}

func loopHeader(header *ssa.BasicBlock) bool {
	for _, predecessor := range header.Preds {
		if header.Dominates(predecessor) {
			return true
		}
	}
	return false
}

func goroutineHasContextLifecycle(spawn *ssa.Go) bool {
	for _, argument := range spawn.Common().Args {
		if contextValue(argument) {
			return true
		}
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if contextValue(analysisutil.CapturedBindingValue(binding)) {
			return true
		}
	}
	return false
}

func contextValue(value ssa.Value) bool {
	return value != nil && analysisutil.NamedType(value.Type(), "context", "Context")
}

func externallyOwnedLifecycle(owners []ssa.Value) bool {
	for _, owner := range owners {
		if analysisutil.ExternallyOwnedValue(owner) {
			return true
		}
	}
	return false
}

func externallyOwnedJoin(signals, groups []ssa.Value) bool {
	for _, value := range append(slices.Clone(signals), groups...) {
		if analysisutil.ExternallyOwnedValue(value) {
			// A goroutine that completes through a caller-owned channel or wait
			// group transfers its join obligation across the call boundary.
			return true
		}
	}
	return false
}

func goroutineLifecycleValues(spawn *ssa.Go) []ssa.Value {
	var owners []ssa.Value
	receiver := analysisutil.CallReceiver(spawn.Common())
	if lifecycleOwner(receiver) {
		owners = append(owners, receiver)
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return owners
	}
	for _, binding := range closure.Bindings {
		value := analysisutil.CapturedBindingValue(binding)
		if lifecycleOwner(value) {
			owners = append(owners, value)
		}
	}
	return owners
}

func goroutineJoinValues(spawn *ssa.Go) (signals, groups []ssa.Value) {
	// Infer join handles from what the goroutine produces (send, close, Done),
	// rather than treating every channel argument as completion. A channel the
	// goroutine only receives from governs its lifetime instead.
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return signals, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			signal, group := spawnedOwnershipValue(spawn, function, closure, instruction)
			if signal != nil {
				signals = append(signals, signal)
			}
			if group != nil {
				groups = append(groups, group)
			}
		}
	}
	return signals, groups
}

func goroutineHasStopLifecycle(spawn *ssa.Go) bool {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return false
	}
	// Generic calls may point at an instantiated wrapper whose body does not
	// expose the receive. The origin has the same parameter positions and the
	// source body needed to prove that the helper joins the signal.
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch candidate := instruction.(type) {
			case *ssa.UnOp:
				if candidate.Op == token.ARROW && spawnedValueAtCall(spawn, function, closure, candidate.X) != nil {
					return true
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && spawnedValueAtCall(spawn, function, closure, state.Chan) != nil {
						return true
					}
				}
			}
		}
	}
	return false
}

func goroutineTransferredToCaller(function *ssa.Function, spawn *ssa.Go) bool {
	for _, owner := range function.Params {
		if !lifecycleOwner(owner) {
			continue
		}
		if analysisutil.AliasesValue(analysisutil.CallReceiver(spawn.Common()), owner) {
			return true
		}
		closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		for _, binding := range closure.Bindings {
			if analysisutil.AliasesValue(analysisutil.CapturedBindingValue(binding), owner) {
				return true
			}
		}
	}
	return false
}

func lifecycleOwner(value ssa.Value) bool {
	if value == nil {
		return false
	}
	methods := types.NewMethodSet(value.Type())
	for index := range methods.Len() {
		if lifecycleMethod(methods.At(index).Obj().Name()) {
			return true
		}
	}
	return false
}

func ownsGoroutineLifecycle(instruction ssa.Instruction, owners []ssa.Value) bool {
	common := analysisutil.InstructionCall(instruction)
	if common != nil && lifecycleMethod(analysisutil.CallName(common)) && aliasesAny(analysisutil.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		for _, method := range []string{"Close", "Kill", "Shutdown", "Stop", "Wait"} {
			if analysisutil.DeferredClosureCalls(instruction, method, owner) {
				return true
			}
		}
	}
	return false
}

func waitsForLifecycleOwner(instruction ssa.Instruction, owners []ssa.Value) bool {
	common := analysisutil.InstructionCall(instruction)
	if common != nil && analysisutil.CallName(common) == "Wait" && aliasesAny(analysisutil.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		if analysisutil.DeferredClosureCalls(instruction, "Wait", owner) {
			return true
		}
	}
	return false
}

func lifecycleMethod(name string) bool {
	switch strings.ToLower(name) {
	case "close", "kill", "shutdown", "stop", "wait":
		return true
	default:
		return false
	}
}

func ownershipRegisteredBefore(spawn *ssa.Go, signals, groups []ssa.Value) bool {
	index := analysisutil.InstructionIndex(spawn)
	if index < 0 {
		return false
	}
	for _, block := range spawn.Parent().Blocks {
		if !block.Dominates(spawn.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == spawn.Block() {
			limit = index
		}
		for _, instruction := range block.Instrs[:limit] {
			if _, deferred := instruction.(*ssa.Defer); deferred && callReceivesAny(instruction, signals) {
				return true
			}
			common := analysisutil.InstructionCall(instruction)
			name := strings.ToLower(analysisutil.CallName(common))
			if common == nil || !ownershipRegistrationName(name) {
				continue
			}
			for _, argument := range common.Args {
				if aliasesAny(argument, signals) || aliasesAny(argument, groups) {
					return true
				}
			}
		}
	}
	return false
}

func ownershipRegistrationName(name string) bool {
	return name == "add" || strings.Contains(name, "register") || strings.Contains(name, "track") || strings.Contains(name, "own")
}

func transfersGoroutineOwnership(instruction ssa.Instruction, signals, groups, owners []ssa.Value) bool {
	values := append(slices.Clone(signals), groups...)
	values = append(values, owners...)
	for _, value := range values {
		if analysisutil.StoresValueInField(instruction, value) || analysisutil.StoresOwnerOfValueInField(instruction, value) || analysisutil.StoresValueInOwnedMap(instruction, value) || analysisutil.CallTransfersValueToField(instruction, value) {
			return true
		}
	}
	if _, ok := instruction.(*ssa.Go); ok {
		for _, group := range groups {
			if analysisutil.ClosureCallsMethod(instruction, "Wait", group) {
				return true
			}
		}
	}
	common := analysisutil.InstructionCall(instruction)
	if common == nil || !ownershipRegistrationName(strings.ToLower(analysisutil.CallName(common))) {
		return false
	}
	for _, argument := range common.Args {
		if aliasesAny(argument, signals) || aliasesAny(argument, groups) {
			return true
		}
	}
	return false
}

func returnedAliasesAny(returned *ssa.Return, values []ssa.Value) bool {
	for _, result := range returned.Results {
		if aliasesAny(result, values) {
			return true
		}
	}
	return false
}

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
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return nil, nil
	}
	switch analysisutil.CallName(common) {
	case analysisutil.BuiltinClose:
		if len(common.Args) == 1 {
			return spawnedValueAtCall(spawn, function, closure, common.Args[0]), nil
		}
	case "Done":
		receiver := analysisutil.CallReceiver(common)
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
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if analysisutil.CallName(common) == "Wait" && aliasesAny(analysisutil.CallReceiver(common), groups) {
		return true
	}
	lower := strings.ToLower(analysisutil.CallName(common))
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
	common := analysisutil.InstructionCall(instruction)
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
				usesSignal = usesSignal || analysisutil.CapturedBindingAliases(binding, signal)
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
							if analysisutil.CapturedBindingAliases(closure.Bindings[index], signal) {
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
				common := analysisutil.InstructionCall(instruction)
				if common != nil {
					for index, free := range function.FreeVars {
						if index < len(typed.Bindings) && analysisutil.AliasesValue(common.Value, free) && valueReceivesAny(typed.Bindings[index], signals, seen) {
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
							if analysisutil.CapturedBindingAliases(typed.Bindings[index], signal) {
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
		if analysisutil.AliasesValue(value, target) {
			return true
		}
	}
	return false
}
