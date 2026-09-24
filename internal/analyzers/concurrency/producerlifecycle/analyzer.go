// Package producerlifecycle implements the producerlifecycle gohawk analyzer.
package producerlifecycle

import (
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "producerlifecycle",
		Doc:      "checks that goroutine producers cannot outlive their receivers",
		Requires: summaryKnowledge.Requires(),
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
	instruction ssa.Instruction
	position    token.Pos
	sequence    int
	summarized  bool
	channel     ssa.Value
	repeated    bool
	spawn       *ssa.Go
}

func reportAbandonedProducerSends(pass *analysis.Pass, function *ssa.Function) {
	engine, _ := summaryKnowledge.Provider(pass).Concurrency()
	sends := producerSends(function, engine)
	reported := map[token.Pos]bool{}
	for _, send := range sends {
		if reported[send.position] {
			continue
		}
		probe := trace.For(pass, "producerlifecycle", string(check.ProducerLifecycleSend), send.position)
		probe.Candidate(trace.Step{Reason: "producer-send", Outcome: trace.OutcomeObserved})
		proof := abandonedProducerSend(function, send, sends, engine)
		outcome := trace.OutcomeUnknown
		if proof.Proven() {
			outcome = trace.OutcomeRejected
			reported[send.position] = true
			check.Reportf(pass, check.ProducerLifecycleSend, send.position, "goroutine send can block after the receiver stops waiting")
		} else if proof.Known() {
			outcome = trace.OutcomeAccepted
		}
		probe.Decision(trace.Step{Reason: string(proof.Reason), Outcome: outcome})
	}
}

func producerSends(function *ssa.Function, engine *concurrencyfacts.Engine) []producerSend {
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
			if summarized, complete := summarizedSends(function, spawn, engine); complete {
				sends = append(sends, summarized...)
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
						sends = append(sends, producerSend{
							instruction: send, position: send.Pos(), channel: channel,
							repeated: ssaflow.BlockInCycle(spawnedBlock), spawn: spawn,
						})
					}
				}
			}
		}
	}
	return sends
}

func abandonedProducerSend(
	function *ssa.Function, send producerSend, sends []producerSend, engine *concurrencyfacts.Engine,
) ssaflow.Proof {
	// No normal caller return puts a continuing or terminated caller outside
	// this finite consumer-count check (for example log.Fatal(<-results)).
	// https://github.com/saljam/webwormhole/blob/abf852af0458ba79772d9c26ef01434165f217d8/cmd/ww/server.go#L458-L470
	if !ssaflow.UnownedReturn(send.spawn, func(ssa.Instruction) bool { return false }, nil) {
		return ssaflow.Proof{Reason: "receiver-does-not-return"}
	}
	// A loop does not establish how many sends are feasible: a map may contain
	// zero or one matching entry, or a state flag may permit only one send.
	// Decline the whole channel count when any contributing send is repeated;
	// otherwise a later send could inherit the same unproven excess count.
	// https://github.com/hashicorp/go-metrics/blob/5a9e5caa3d2779bca6a8ae2218b8f884194855e7/inmem_endpoint_test.go#L157-L177
	sendCount := 0
	for _, candidate := range sends {
		if !heapmodel.MayAlias(candidate.channel, send.channel) {
			continue
		}
		if candidate.repeated {
			return ssaflow.Proof{Reason: "producer-count-unknown"}
		}
		if producerSendMayPrecede(candidate, send) {
			sendCount++
		}
	}
	receives := channelReceives(function, send.channel, send.spawn, engine)
	if receives.unknown {
		return ssaflow.Proof{Reason: ssaflow.EvidenceReason(receives.reason)}
	}
	if receives.count == 0 {
		return ssaflow.Proof{Reason: "receiver-obligation-unknown"}
	}
	if sendCount > receives.count {
		return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "producer-exceeds-receives"}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: "producer-within-receive-count"}
}

func producerSendMayPrecede(first, second producerSend) bool {
	if first.spawn != second.spawn {
		return true
	}
	if first.summarized && second.summarized {
		return first.sequence <= second.sequence
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
			if !ok || !heapmodel.CapturedBindingMatches(channel, created) {
				continue
			}
			size, ok := created.Size.(*ssa.Const)
			return ok && size.Value != nil && constant.Sign(size.Value) == 0
		}
	}
	return false
}

func channelReceives(function *ssa.Function, channel ssa.Value, origin *ssa.Go, engine *concurrencyfacts.Engine) receiveProof {
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	var result receiveProof
	for _, block := range function.Blocks {
		before := result.count
		for _, instruction := range block.Instrs {
			switch candidate := instruction.(type) {
			case ssa.CallInstruction:
				proof := helperReceives(candidate, channel, origin, engine, budget)
				result.count += proof.count
				if proof.unknown {
					return proof
				}
			case *ssa.UnOp:
				if candidate.Op == token.ARROW && heapmodel.MayAlias(candidate.X, channel) {
					result.count++
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && heapmodel.MayAlias(state.Chan, channel) {
						result.count++
					}
				}
			}
		}
		if result.count > before && ssaflow.BlockInCycle(block) {
			return receiveProof{unknown: true, reason: "receiver-may-drain"}
		}
	}
	return result
}
