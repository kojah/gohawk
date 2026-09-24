package producerlifecycle

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Complete worker summaries normalize hidden sends into the existing ordered
// producer/receiver proof. Receiver helpers are expanded too: exposing sends
// alone would turn a hidden drain into a false abandoned-producer diagnostic.
func summarizedSends(function *ssa.Function, spawn *ssa.Go, engine *concurrencyfacts.Engine) ([]producerSend, bool) {
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	summary := engine.AtCall(spawn, budget)
	if !summary.Complete() {
		return nil, false
	}
	for _, operation := range summary.Operations {
		if operation.Kind != concurrencyfacts.Send && operation.Kind != concurrencyfacts.Close {
			return nil, false
		}
	}
	// A local worker retains its own call-site positions; an imported worker
	// can only be attributed to the launch in this package.
	// Equal branches fold into one operation that keeps every branch's source.
	local := engine.Function(spawn.Common().StaticCallee(), budget)
	var sends []producerSend
	for i, operation := range summary.Operations {
		channel := operation.Resource.Value
		if operation.Kind != concurrencyfacts.Send || operation.Resource.Indirect || !localUnbufferedChannel(function, channel) {
			continue
		}
		positions := []token.Pos{spawn.Pos()}
		if local.Complete() && len(local.Operations) == len(summary.Operations) {
			positions = append([]token.Pos{local.Operations[i].Site}, local.Operations[i].Alternates...)
		}
		for _, position := range positions {
			sends = append(sends, producerSend{
				instruction: spawn, position: position, channel: channel, spawn: spawn,
				sequence: i, summarized: true,
			})
		}
	}
	return sends, true
}

// A protocol can be incomplete because of unrelated calls or payloads, while
// every use of its channel is still visible. Shared call effects distinguish
// that case from an opaque participant. A send/close mutates the channel;
// receiving is unsupported by that proof and therefore cannot establish this
// absence claim. Retention, asynchronous use, and unknown uses also decline.
func nonReceivingUses(call ssa.CallInstruction, channel ssa.Value, budget *ssaflow.SearchBudget) producerProof {
	unknown := producerProof{Reason: reasonWorkerChannelUsesUnknown}
	function, closure := ssaflow.DirectCallee(call.Common())
	if function == nil || len(function.Blocks) == 0 {
		return unknown
	}
	query := ssaflow.NewCallEffects(budget)
	matched := false
	for _, binding := range ssaflow.CallBindings(call.Common(), function, closure) {
		if !heapmodel.CapturedBindingMatches(binding.Supplied, channel) && !lifecycle.MayContainValue(binding.Supplied, channel) {
			continue
		}
		matched = true
		if ssaflow.ChannelType(binding.Local) {
			if !onlyChannelMutation(query.Value(binding.Local)) {
				return unknown
			}
			continue
		}
		if !readOnlyChannelCell(query, binding.Local) {
			return unknown
		}
	}
	if !matched || budget.Exhausted() {
		return unknown
	}
	return producerProof{State: ssaflow.EvidenceProven, Reason: reasonWorkerChannelUsesComplete}
}

func onlyChannelMutation(proof ssaflow.CallEffectProof) bool {
	return proof.Proven() && proof.Effects & ^(ssaflow.EffectRead|ssaflow.EffectMutate) == 0
}

func readOnlyChannelCell(query *ssaflow.CallEffects, cell ssa.Value) bool {
	if !query.Value(cell).PreservesStorage() || cell.Referrers() == nil {
		return false
	}
	for _, instruction := range *cell.Referrers() {
		if _, debug := instruction.(*ssa.DebugRef); debug {
			continue
		}
		load, ok := instruction.(*ssa.UnOp)
		if !ok || load.Op != token.MUL || !ssaflow.ChannelType(load) || !onlyChannelMutation(query.Value(load)) {
			return false
		}
	}
	return true
}

type receiveProof struct {
	count   int
	unknown bool
	reason  producerReason
}

func helperReceives(
	call ssa.CallInstruction, channel ssa.Value, origin *ssa.Go,
	engine *concurrencyfacts.Engine, budget *ssaflow.SearchBudget,
) receiveProof {
	if call == origin {
		return receiveProof{reason: reasonProducerLaunch}
	}
	common := call.Common()
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return receiveProof{reason: reasonBuiltinNotReceive}
	}
	summary := engine.AtCall(call, budget)
	_, launched := call.(*ssa.Go)
	if !summary.Complete() {
		if launched && nonReceivingUses(call, channel, budget).Proven() {
			return receiveProof{reason: reasonWorkerChannelUsesComplete}
		}
		// An opaque callback may receive later, including registered cleanup.
		// We cannot prove its execution paths from a captured channel alone.
		// https://github.com/kubernetes/registry.k8s.io/blob/b5e7d92a3819fcd24ed35b174db0ce6291e88e7f/cmd/archeio/main_test.go#L73-L80
		consumes := func(value ssa.Value) bool {
			return lifecycle.MayContainValue(value, channel) || heapmodel.CapturedBindingMatches(value, channel)
		}
		uncertain := slices.ContainsFunc(common.Args, consumes)
		if closure, ok := common.Value.(*ssa.MakeClosure); ok {
			uncertain = uncertain || slices.ContainsFunc(closure.Bindings, consumes)
		}
		return receiveProof{unknown: uncertain, reason: reasonReceiverHelperUnknown}
	}
	proof := receiveProof{reason: reasonReceiverHelperComplete}
	storage := heapmodel.NewStorage(budget)
	for _, operation := range summary.Operations {
		if operation.Kind == concurrencyfacts.Receive && !operation.Resource.Indirect && storage.Same(operation.Resource.Value, channel).Proven() {
			if launched {
				return receiveProof{unknown: true, reason: reasonAsynchronousReceiver}
			}
			proof.count++
		}
	}
	return proof
}
