package channelsafety

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syncgraph"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This proof handles two fresh unbuffered channels, not general channel
// protocols. Each goroutine's first action needs the other's later one;
// unmodeled select alternatives or participants leave the result unknown.
type channelCycleProof struct {
	first, parentSecond, workerFirst, workerSecond token.Pos
	outcome                                        analysisTrace.Outcome
	reason                                         string
}

func reportChannelCycle(pass *analysis.Pass, function *ssa.Function) {
	candidate := potentialChannelCycleRoot(function)
	if candidate == token.NoPos {
		return
	}
	probe := analysisTrace.For(pass, "channelsafety", string(check.ChannelDependencyCycle), candidate)
	probe.Candidate(analysisTrace.Step{Reason: "channel-cycle-candidate", Outcome: analysisTrace.OutcomeObserved, Pos: candidate})
	engine, available := summaryKnowledge.Provider(pass).Concurrency()
	if available != summaries.Available || engine == nil {
		probe.Decision(analysisTrace.Step{Reason: "channel-cycle-summary-unavailable", Outcome: analysisTrace.OutcomeUnknown, Pos: candidate})
		return
	}
	root := engine.Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget).Observed(probe.Observer()))
	graphs, reason := syncgraph.Expand(root)
	proof := channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: reason}
	if reason == "" {
		proof = proveEveryChannelCycle(graphs, root.Choices)
	}
	probe.Decision(analysisTrace.Step{Reason: proof.reason, Outcome: proof.outcome, Pos: candidate})
	if proof.outcome != analysisTrace.OutcomeAccepted {
		return
	}
	source := syntax.SourceRange(pass, proof.first)
	check.Report(pass, check.ChannelDependencyCycle, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: "two goroutines wait on each other's later channel operation",
		Related: []analysis.RelatedInformation{
			{Pos: proof.workerFirst, Message: "worker also blocks before reaching its matching operation"},
			{Pos: proof.parentSecond, Message: "parent can match the worker only after its first operation"},
			{Pos: proof.workerSecond, Message: "worker can match the parent only after its first operation"},
		},
	})
}

func proveEveryChannelCycle(graphs []syncgraph.SyncGraph, choices []concurrencyfacts.SelectChoice) channelCycleProof {
	if len(graphs) == 0 {
		return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-alternatives-unknown"}
	}
	if len(graphs) == 1 {
		return proveChannelCycle(graphs[0])
	}
	var common channelCycleProof
	for _, graph := range graphs {
		proof := proveChannelCycle(graph)
		if proof.outcome != analysisTrace.OutcomeAccepted {
			// An arm that can terminate, choose cancellation, or communicate
			// elsewhere is enough to defeat an unavoidable cycle proof.
			return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-alternative-unproven"}
		}
		if common.reason == "" {
			common = proof
			continue
		}
		if proof.first != common.first || proof.parentSecond != common.parentSecond ||
			proof.workerSecond != common.workerSecond {
			return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-alternative-sites-differ"}
		}
	}
	if len(graphs) > 1 {
		if len(choices) != 1 || !choices[0].Site.IsValid() || choices[0].Worker == nil {
			return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-alternative-site-unknown"}
		}
		common.workerFirst = choices[0].Site
	}
	return common
}

func potentialChannelCycleRoot(function *ssa.Function) token.Pos {
	if function == nil {
		return token.NoPos
	}
	var launched bool
	var channels int
	var operation token.Pos
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Go:
				launched = true
			case *ssa.Call:
				// A direct helper may launch a child through a complete summary.
				// This is only a cheap candidate filter, not a launch claim.
				launched = launched || instruction.Common().StaticCallee() != nil
			case *ssa.MakeChan:
				channels++
			case *ssa.Send:
				if operation == token.NoPos {
					operation = instruction.Pos()
				}
			case *ssa.UnOp:
				if instruction.Op == token.ARROW && operation == token.NoPos {
					operation = instruction.Pos()
				}
			}
		}
	}
	if !launched || channels < 2 {
		return token.NoPos
	}
	return operation
}

func proveChannelCycle(graph syncgraph.SyncGraph) channelCycleProof {
	if !graph.Complete() {
		return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: graph.Reason}
	}
	if len(graph.Parent) != 2 || len(graph.Children) == 0 {
		return channelCycleProof{outcome: analysisTrace.OutcomeRejected, reason: "channel-cycle-shape-not-matched"}
	}
	first, second := graph.Parent[0], graph.Parent[1]
	a, aFresh := first.Resource.Value.(*ssa.MakeChan)
	b, bFresh := second.Resource.Value.(*ssa.MakeChan)
	if !aFresh || !bFresh || a == b || first.Resource.Indirect || second.Resource.Indirect ||
		!unbufferedChannel(a) || !unbufferedChannel(b) || !oppositeChannelActions(first.Kind, second.Kind) {
		return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-resource-or-order-unknown"}
	}
	workerFirst, workerSecond, failure := findChannelCycleWorkers(graph.Children, first, second)
	if failure.reason != "" {
		return failure
	}
	if !first.Source.IsValid() || !second.Source.IsValid() ||
		!workerFirst.Source.IsValid() || !workerSecond.Source.IsValid() {
		return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-source-unknown"}
	}
	// Both first actions need the other goroutine's later match; an unbuffered
	// send or receive cannot complete independently, so these edges are exact.
	if !graph.AddDependency(workerSecond.ID, first.ID) ||
		!graph.AddDependency(second.ID, workerFirst.ID) || !graph.HasCycle() {
		return channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-dependencies-unproven"}
	}
	return channelCycleProof{
		first: first.Source, parentSecond: second.Source, workerFirst: workerFirst.Source, workerSecond: workerSecond.Source,
		outcome: analysisTrace.OutcomeAccepted, reason: "channel-cycle-proven",
	}
}

func findChannelCycleWorkers(
	children []syncgraph.SyncChild, first, second syncgraph.SyncEvent,
) (syncgraph.SyncEvent, syncgraph.SyncEvent, channelCycleProof) {
	var witnessFirst, witnessSecond syncgraph.SyncEvent
	for _, child := range children {
		if !child.LaunchKnown() || child.Prefix != 0 {
			return syncgraph.SyncEvent{}, syncgraph.SyncEvent{},
				channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-spawn-order-unknown"}
		}
		if len(child.Events) == 0 {
			continue
		}
		if len(child.Events) != 2 || !crossedChannelActions(first, second, child.Events[0], child.Events[1]) {
			return syncgraph.SyncEvent{}, syncgraph.SyncEvent{},
				channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-other-participant"}
		}
		if !witnessFirst.Source.IsValid() {
			witnessFirst, witnessSecond = child.Events[0], child.Events[1]
		}
	}
	if !witnessFirst.Source.IsValid() {
		return syncgraph.SyncEvent{}, syncgraph.SyncEvent{},
			channelCycleProof{outcome: analysisTrace.OutcomeUnknown, reason: "channel-cycle-partner-unknown"}
	}
	return witnessFirst, witnessSecond, channelCycleProof{}
}

func crossedChannelActions(first, second, workerFirst, workerSecond syncgraph.SyncEvent) bool {
	return !workerFirst.Resource.Indirect && !workerSecond.Resource.Indirect &&
		workerFirst.Kind == first.Kind && workerSecond.Kind == second.Kind &&
		workerFirst.Resource.Value == second.Resource.Value && workerSecond.Resource.Value == first.Resource.Value
}

func oppositeChannelActions(first, second concurrencyfacts.Kind) bool {
	return first == concurrencyfacts.Send && second == concurrencyfacts.Receive ||
		first == concurrencyfacts.Receive && second == concurrencyfacts.Send
}

func unbufferedChannel(channel *ssa.MakeChan) bool {
	size, ok := channel.Size.(*ssa.Const)
	return ok && size.Value != nil && constant.Sign(size.Value) == 0
}
