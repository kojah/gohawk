package general

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
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		reportAbandonedProducerSends(pass, function)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				signals, groups := goroutineJoinValues(spawn)
				owners := goroutineLifecycleValues(spawn)
				if config.mode == goroutineModeContext && goroutineHasContextLifecycle(spawn) {
					continue
				}
				if (config.mode != goroutineModeJoin && (goroutineTransferredToCaller(function, spawn) || externallyOwnedLifecycle(owners))) || ownershipRegisteredBefore(spawn, signals, groups) {
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
					analysisutil.Reportf(pass, spawn.Pos(), "goroutine is not joined on every return path")
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
						sends = append(sends, producerSend{instruction: send, channel: channel, repeated: blockInCycle(spawnedBlock)})
					}
				}
			}
		}
	}
	reported := map[token.Pos]bool{}
	for _, send := range sends {
		sendCount := 0
		for _, candidate := range sends {
			if analysisutil.AliasesValue(candidate.channel, send.channel) {
				sendCount++
			}
		}
		receiveCount, draining := channelReceives(function, send.channel)
		if receiveCount == 0 || draining || !send.repeated && sendCount <= receiveCount || reported[send.instruction.Pos()] {
			continue
		}
		reported[send.instruction.Pos()] = true
		analysisutil.Reportf(pass, send.instruction.Pos(), "goroutine send can block after the receiver stops waiting")
	}
}

func spawnedValueAtCall(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) ssa.Value { //nolint:ireturn // SSA values retain their concrete representations.
	if closure != nil {
		for index, free := range function.FreeVars {
			if analysisutil.AliasesValue(value, free) && index < len(closure.Bindings) {
				return analysisutil.CapturedBindingValue(closure.Bindings[index])
			}
		}
	}
	for index, parameter := range function.Params {
		if analysisutil.AliasesValue(value, parameter) && index < len(spawn.Common().Args) {
			return spawn.Common().Args[index]
		}
	}
	return nil
}

func localUnbufferedChannel(function *ssa.Function, channel ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			created, ok := instruction.(*ssa.MakeChan)
			if !ok || !analysisutil.AliasesValue(channel, created) {
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
	function, closure, ok := spawnedClosure(spawn)
	if !ok {
		for _, argument := range spawn.Common().Args {
			if analysisutil.ChannelType(argument) {
				signals = append(signals, argument)
			}
		}
		return signals, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			signal, group := closureOwnershipValue(function, closure, instruction)
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
	for _, instruction := range spawn.Block().Instrs[:index] {
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
	return false
}

func ownershipRegistrationName(name string) bool {
	return name == "add" || strings.Contains(name, "register") || strings.Contains(name, "track") || strings.Contains(name, "own")
}

func transfersGoroutineOwnership(instruction ssa.Instruction, signals, groups, owners []ssa.Value) bool {
	values := append(slices.Clone(signals), groups...)
	values = append(values, owners...)
	for _, value := range values {
		if analysisutil.StoresValueInField(instruction, value) || analysisutil.StoresValueInOwnedMap(instruction, value) || analysisutil.CallTransfersValueToField(instruction, value) {
			return true
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

func closureOwnershipValue(function *ssa.Function, closure *ssa.MakeClosure, instruction ssa.Instruction) (signal, group ssa.Value) { //nolint:ireturn // Closure ownership can flow through channels or synchronization values.
	if send, ok := instruction.(*ssa.Send); ok {
		return closureBinding(function, closure, send.Chan), nil
	}
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return nil, nil
	}
	switch analysisutil.CallName(common) {
	case analysisutil.BuiltinClose:
		if len(common.Args) == 1 {
			return closureBinding(function, closure, common.Args[0]), nil
		}
	case "Done":
		return nil, closureBinding(function, closure, analysisutil.CallReceiver(common))
	}
	return nil, nil
}

func closureBinding(function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) ssa.Value { //nolint:ireturn // Captures retain their concrete SSA value form.
	for index, free := range function.FreeVars {
		if analysisutil.AliasesValue(value, free) && index < len(closure.Bindings) {
			return analysisutil.CapturedBindingValue(closure.Bindings[index])
		}
	}
	return nil
}

func joinsGoroutine(instruction ssa.Instruction, signals, groups []ssa.Value) bool {
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

func aliasesAny(value ssa.Value, targets []ssa.Value) bool {
	for _, target := range targets {
		if analysisutil.AliasesValue(value, target) {
			return true
		}
	}
	return false
}
