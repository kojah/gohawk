package producerlifecycle

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Complete worker summaries normalize hidden sends into the existing ordered
// producer/receiver proof. Receiver helpers are expanded too: exposing sends
// alone would turn a hidden drain into a false abandoned-producer diagnostic.
func summarizedSends(function *ssa.Function, spawn *ssa.Go, engine *concurrencyfacts.Engine) ([]producerSend, bool) {
	budget := ssaflow.NewSearchBudget(2000)
	summary := engine.AtCall(spawn, budget)
	if summary.Reason != "" {
		return nil, false
	}
	for _, operation := range summary.Operations {
		if operation.Kind != concurrencyfacts.Send && operation.Kind != concurrencyfacts.Close {
			return nil, false
		}
	}
	// A local worker retains its own call-site positions; an imported worker
	// can only be attributed to the launch in this package.
	local := engine.Function(spawn.Common().StaticCallee(), budget)
	var sends []producerSend
	for i, operation := range summary.Operations {
		channel := operation.Resource.Value
		if operation.Kind != concurrencyfacts.Send || operation.Resource.Indirect || !localUnbufferedChannel(function, channel) {
			continue
		}
		position := spawn.Pos()
		if local.Reason == "" && len(local.Operations) == len(summary.Operations) {
			position = local.Operations[i].Site
		}
		sends = append(sends, producerSend{
			instruction: spawn, position: position, channel: channel, spawn: spawn,
			sequence: i, summarized: true,
		})
	}
	return sends, true
}

// A protocol can be incomplete because of unrelated calls or payloads, while
// every use of its channel is still visible. Shared call effects distinguish
// that case from an opaque participant. A send/close mutates the channel;
// receiving is unsupported by that proof and therefore cannot establish this
// absence claim. Retention, asynchronous use, and unknown uses also decline.
func nonReceivingUses(call ssa.CallInstruction, channel ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
	unknown := ssaflow.Proof{Reason: "worker-channel-uses-unknown"}
	function, closure := ssaflow.DirectCallee(call.Common())
	if function == nil || len(function.Blocks) == 0 {
		return unknown
	}
	query := ssaflow.NewCallEffects(budget)
	matched := false
	for _, binding := range ssaflow.CallBindings(call.Common(), function, closure) {
		if !ssaflow.CapturedBindingMatches(binding.Supplied, channel) && !ssaflow.ValueContainsValue(binding.Supplied, channel) {
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
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "worker-channel-uses-complete"}
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
	reason  string
}

func helperReceives(
	call ssa.CallInstruction, channel ssa.Value, origin *ssa.Go,
	engine *concurrencyfacts.Engine, budget *ssaflow.SearchBudget,
) receiveProof {
	if call == origin {
		return receiveProof{reason: "producer-launch"}
	}
	common := call.Common()
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return receiveProof{reason: "builtin-not-receive"}
	}
	summary := engine.AtCall(call, budget)
	_, launched := call.(*ssa.Go)
	if summary.Reason != "" {
		if launched && nonReceivingUses(call, channel, budget).Proven() {
			return receiveProof{reason: "worker-channel-uses-complete"}
		}
		// An opaque callback may receive later, including registered cleanup.
		// We cannot prove its execution paths from a captured channel alone.
		// https://github.com/kubernetes/registry.k8s.io/blob/b5e7d92a3819fcd24ed35b174db0ce6291e88e7f/cmd/archeio/main_test.go#L73-L80
		consumes := func(value ssa.Value) bool {
			return ssaflow.ValueContainsValue(value, channel) || ssaflow.CapturedBindingMatches(value, channel)
		}
		uncertain := slices.ContainsFunc(common.Args, consumes)
		if closure, ok := common.Value.(*ssa.MakeClosure); ok {
			uncertain = uncertain || slices.ContainsFunc(closure.Bindings, consumes)
		}
		return receiveProof{unknown: uncertain, reason: "receiver-helper-unknown"}
	}
	proof := receiveProof{reason: "receiver-helper-complete"}
	storage := ssaflow.NewStorage(budget)
	for _, operation := range summary.Operations {
		if operation.Kind == concurrencyfacts.Receive && !operation.Resource.Indirect && storage.Same(operation.Resource.Value, channel).Proven() {
			if launched {
				return receiveProof{unknown: true, reason: "asynchronous-receiver"}
			}
			proof.count++
		}
	}
	return proof
}
