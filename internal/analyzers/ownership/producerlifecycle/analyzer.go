// Package producerlifecycle implements the producerlifecycle gohawk analyzer.
package producerlifecycle

import (
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

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
	functions, err := ssaflow.SourceSSAFunctions(pass)
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
	// Only local unbuffered channels expose a direct producer/receiver count.
	// Buffered or externally supplied channels have capacity and ownership
	// contracts that this analyzer cannot safely infer.
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
					channel := ssaflow.SpawnedValueAtCall(spawn, spawned, closure, send.Chan)
					if channel != nil && localUnbufferedChannel(function, channel) {
						sends = append(sends, producerSend{instruction: send, channel: channel, repeated: ssaflow.BlockInCycle(spawnedBlock), spawn: spawn})
					}
				}
			}
		}
	}
	return sends
}

func abandonedProducerSend(function *ssa.Function, send producerSend, sends []producerSend, reported map[token.Pos]bool) bool {
	// A looped send is potentially unbounded; otherwise compare only sends that
	// can consume a receive before this send. A draining receive loop discharges either
	// form because it continues to service the producer.
	if reported[send.instruction.Pos()] {
		return false
	}
	sendCount := 0
	for _, candidate := range sends {
		if ssaflow.SameValue(candidate.channel, send.channel) && producerSendMayPrecede(candidate, send) {
			sendCount++
		}
	}
	receiveCount, draining := channelReceives(function, send.channel)
	return receiveCount > 0 && !draining && (send.repeated || sendCount > receiveCount)
}

func producerSendMayPrecede(first, second producerSend) bool {
	if first.spawn != second.spawn {
		return true
	}
	if first.instruction == second.instruction {
		return true
	}
	// Later sends in the same worker cannot consume the receive serving this
	// one: an unbuffered send must finish before the worker reaches the next.
	// Different workers still compete, and repeated sends retain the separate
	// unbounded-loop rule above. This narrows attribution, not protocol coverage.
	// https://github.com/kubernetes/registry.k8s.io/blob/b5e7d92a3819fcd24ed35b174db0ce6291e88e7f/cmd/archeio/main_test.go#L69-L83
	return ssaflow.InstructionMayFollow(first.instruction, second.instruction)
}

func localUnbufferedChannel(function *ssa.Function, channel ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			created, ok := instruction.(*ssa.MakeChan)
			if !ok || !ssaflow.CapturedBindingMatches(channel, created) {
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
				if candidate.Op == token.ARROW && ssaflow.SameValue(candidate.X, channel) {
					count++
					draining = draining || ssaflow.BlockInCycle(block)
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssaflow.SameValue(state.Chan, channel) {
						count++
						draining = draining || ssaflow.BlockInCycle(block)
					}
				}
			}
		}
	}
	return count, draining
}
