// Package producerlifecycle implements the producerlifecycle gohawk analyzer.
package producerlifecycle

import (
	"go/constant"
	"go/token"
	"go/types"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "producerlifecycle",
		Doc:      "checks that goroutine producers cannot outlive their receivers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runProducerLifecycle,
	}
}

func runProducerLifecycle(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportAbandonedProducerSends(pass, function)
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
	sends := producerSends(function)
	reported := map[token.Pos]bool{}
	for _, send := range sends {
		if abandonedProducerSend(function, send, sends, reported) {
			reported[send.instruction.Pos()] = true
			check.Reportf(pass, check.ProducerLifecycleSend, send.instruction.Pos(), "goroutine send can block after the receiver stops waiting")
		}
	}
}

func producerSends(function *ssa.Function) []producerSend {
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
					channel := ssautil.SpawnedValueAtCall(spawn, spawned, closure, send.Chan)
					if channel != nil && localUnbufferedChannel(function, channel) {
						sends = append(sends, producerSend{instruction: send, channel: channel, repeated: ssautil.BlockInCycle(spawnedBlock), spawn: spawn})
					}
				}
			}
		}
	}
	return sends
}

func abandonedProducerSend(function *ssa.Function, send producerSend, sends []producerSend, reported map[token.Pos]bool) bool {
	if reported[send.instruction.Pos()] {
		return false
	}
	sendCount := 0
	for _, candidate := range sends {
		if ssautil.SameValue(candidate.channel, send.channel) && producerSendsCanCooccur(send, candidate) {
			sendCount++
		}
	}
	receiveCount, draining := channelReceives(function, send.channel)
	return receiveCount > 0 && !draining && (send.repeated || sendCount > receiveCount)
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
					draining = draining || ssautil.BlockInCycle(block)
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssautil.SameValue(state.Chan, channel) {
						count++
						draining = draining || ssautil.BlockInCycle(block)
					}
				}
			}
		}
	}
	return count, draining
}
