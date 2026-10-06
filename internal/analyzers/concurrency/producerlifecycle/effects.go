package producerlifecycle

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Complete worker summaries normalize hidden sends into the existing ordered
// producer/receiver proof. Receiver helpers are expanded too: exposing sends
// alone would turn a hidden drain into a false abandoned-producer diagnostic.
func summarizedSends(function *ssa.Function, spawn *ssa.Go, engine *concurrencyfacts.Engine) ([]producerSend, bool) {
	budget := proofs.NewSearchBudget(proofs.SummaryBudget)
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
		// Alternate branch sites are attribution metadata for one execution,
		// not additional sends competing for the caller's receives.
		sends = append(sends, producerSend{
			instruction: spawn, positions: positions, channel: channel, spawn: spawn,
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
func nonReceivingUses(call ssa.CallInstruction, channel ssa.Value, budget *proofs.SearchBudget) producerProof {
	unknown := producerProof{Reason: reasonWorkerChannelUsesUnknown}
	function, closure := ssaflow.DirectCallee(call.Common())
	if function == nil || len(function.Blocks) == 0 {
		return unknown
	}
	query := ssaflow.NewCallEffects(budget)
	matched := false
	for binding := range ssaflow.CallBindingsWithin(call.Common(), function, closure, budget) {
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
	return producerProof{State: proofs.EvidenceProven, Reason: reasonWorkerChannelUsesComplete}
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
	engine *concurrencyfacts.Engine, budget *proofs.SearchBudget,
) receiveProof {
	if call == origin {
		return receiveProof{reason: reasonProducerLaunch}
	}
	common := call.Common()
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return receiveProof{reason: reasonBuiltinNotReceive}
	}
	summary := engine.AtCall(call, budget)
	if budget.Exhausted() {
		return receiveProof{unknown: true, reason: reasonReceiverBudgetExhausted}
	}
	_, launched := call.(*ssa.Go)
	if !summary.Complete() {
		if launched && nonReceivingUses(call, channel, budget).Proven() {
			return receiveProof{reason: reasonWorkerChannelUsesComplete}
		}
		if budget.Exhausted() {
			return receiveProof{unknown: true, reason: reasonReceiverBudgetExhausted}
		}
		// An opaque callback may receive later, including registered cleanup.
		// We cannot prove its execution paths from a captured channel alone.
		// https://github.com/kubernetes/registry.k8s.io/blob/b5e7d92a3819fcd24ed35b174db0ce6291e88e7f/cmd/archeio/main_test.go#L73-L80
		consumes := func(value ssa.Value) bool {
			return lifecycle.MayContainValue(value, channel) || heapmodel.CapturedBindingMatches(value, channel) ||
				lifecycle.ProvePossibleClosureCaptureWithin(value, channel, budget).Proven()
		}
		uncertain := slices.ContainsFunc(common.Args, consumes)
		// A selected callback can drain too. Capture evidence leaves the
		// receiver unknown; it never contributes an exact receive count.
		uncertain = uncertain || lifecycle.ProvePossibleClosureCaptureWithin(common.Value, channel, budget).Proven()
		if budget.Exhausted() {
			return receiveProof{unknown: true, reason: reasonReceiverBudgetExhausted}
		}
		return receiveProof{unknown: uncertain, reason: reasonReceiverHelperUnknown}
	}
	return summaryReceives(summary, channel, launched, budget)
}

// A complete protocol still needs exact channel binding. Losing that identity
// query to the shared allowance cannot establish an absent receive.
func summaryReceives(summary concurrencyfacts.Summary, channel ssa.Value, launched bool, budget *proofs.SearchBudget) receiveProof {
	proof := receiveProof{reason: reasonReceiverHelperComplete}
	storage := heapmodel.NewStorage(budget)
	for _, operation := range summary.Operations {
		if operation.Kind != concurrencyfacts.Receive || operation.Resource.Indirect {
			continue
		}
		same := storage.Same(operation.Resource.Value, channel)
		if budget.Exhausted() {
			return receiveProof{unknown: true, reason: reasonReceiverBudgetExhausted}
		}
		if same.Proven() {
			if launched {
				return receiveProof{unknown: true, reason: reasonAsynchronousReceiver}
			}
			proof.count++
		}
	}
	return proof
}
