package goroutineownership

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

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
			if ssautil.SameValue(candidate.channel, send.channel) && producerSendsCanCooccur(send, candidate) {
				sendCount++
			}
		}
		receiveCount, draining := channelReceives(function, send.channel)
		if receiveCount == 0 || draining || !send.repeated && sendCount <= receiveCount || reported[send.instruction.Pos()] {
			continue
		}
		reported[send.instruction.Pos()] = true
		check.Reportf(pass, check.GoroutineProducerSend, send.instruction.Pos(), "goroutine send can block after the receiver stops waiting")
	}
}

func producerSendsCanCooccur(first, second producerSend) bool {
	if first.spawn != second.spawn {
		return true
	}
	if first.instruction == second.instruction {
		return true
	}
	return ssautil.InstructionMayFollow(first.instruction, second.instruction) || ssautil.InstructionMayFollow(second.instruction, first.instruction)
}

func spawnedValueAtCall(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	value ssa.Value,
) ssa.Value { //nolint:ireturn // SSA values retain their concrete representations.
	if closure != nil {
		for index, free := range function.FreeVars {
			if passedValueAliases(value, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) {
				captured := ssautil.CapturedBindingValue(closure.Bindings[index])
				// Keep the address when the first observed value is nil. The channel
				// may be assigned only after an owner closure is created, as in
				// Kubernetes test-server teardown paths.
				if ssautil.DefinitelyNil(captured) {
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
			if !ok || !ssautil.CapturedBindingMatches(channel, created) {
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
				if candidate.Op == token.ARROW && ssautil.SameValue(candidate.X, channel) {
					count++
					draining = draining || blockInCycle(block)
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssautil.SameValue(state.Chan, channel) {
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
