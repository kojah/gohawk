package cancellationownership

import (
	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysis/summaries"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/ssa"
)

// CancellationOutcome records the only four conclusions the analyzer may
// draw. In particular, Unknown is not a weaker Lost: ambiguous handoffs
// suppress the default correctness diagnostic.
type CancellationOutcome uint8

const (
	CancellationUnknown CancellationOutcome = iota
	CancellationReleased
	CancellationTransferred
	CancellationLost
)

// CancellationProof is the authoritative cancellationownership decision.
type CancellationProof struct {
	Outcome CancellationOutcome
	Reason  cancellationReason
	// Witness is the return a lost proof reached without a cancel.
	Witness *ssa.Return
}

type cancellationClassifier struct {
	cancel  ssa.Value
	context ssa.Value
	// processLifetime bounds standard-context retention to a single program
	// entry acquisition. It never establishes invocation of the cancel.
	processLifetime bool
	parent          *cancellationClassifier
	actions         map[ssa.Instruction]cancellationAction
	transfers       bool
	// observer hears where the shared storage, effect, and completion queries
	// behind this proof gave up; nil when the candidate is not being traced.
	observer proofs.Observer
	// probe traces this classifier's labels. Only the candidate's own
	// classifier has one; a parent context's classifier stays silent, since
	// its labels reach the trace as the child's.
	probe    analysisTrace.Probe
	evidence *lifecyclefacts.LifecycleEvidence
	// knowledge supplies result guarantees; guards are the directly deferred
	// literals whose call of cancel turns on a named result. See
	// result_guards.go.
	knowledge *summaries.Provider
	guards    []lifecycle.ResultGuard
	// owner is the struct this function allocates and stores the cancel
	// into, directly or through a capturing closure; see owner_structs.go.
	owner *cancellationOwnerProof
	// pool is this cancellation's total across every query its proof asks;
	// see budget.
	pool *proofs.SearchBudget
}

// Exhausted helper searches remain unknown, never evidence of lost cleanup.
const cancellationCompletionBudget = proofs.QueryBudget

// cancellationPoolBudget bounds a whole cancellation proof, a hundred full
// queries, so a candidate in a large function stays bounded; an exhausted
// pool is unknown exactly as an exhausted query is.
const cancellationPoolBudget = 100 * cancellationCompletionBudget

// budget draws one query's allowance from this proof's pool; the pool
// carries the observer, so every give-up reaches the trace.
func (classifier *cancellationClassifier) budget() *proofs.SearchBudget {
	if classifier.pool == nil {
		classifier.pool = proofs.NewSearchBudget(cancellationPoolBudget).Observed(classifier.observer)
	}
	return classifier.pool.Within(cancellationCompletionBudget)
}

func proveCancellation(
	call *ssa.Call, cancel ssa.Value, probe analysisTrace.Probe, evidence *lifecyclefacts.LifecycleEvidence, knowledge *summaries.Provider,
) CancellationProof {
	observer := probe.Observer()
	classifier := &cancellationClassifier{
		probe:    probe,
		cancel:   cancel,
		parent:   parentCancellationClassifier(call, observer),
		actions:  make(map[ssa.Instruction]cancellationAction),
		observer: observer,
		evidence: evidence,
	}
	classifier.knowledge = knowledge
	// Missing guards cannot establish loss when the discovery census or one
	// of its opposing completion questions stopped before deciding.
	request := classifier.invokeRequest()
	discovery := lifecycle.ProveResultGuards(call.Parent(), request)
	if !discovery.Proven() {
		return CancellationProof{Outcome: CancellationUnknown, Reason: reasonCancellationUnknown}
	}
	if !classifier.retainResultGuardsWithin(discovery.Guards, request.Budget).Proven() {
		return CancellationProof{Outcome: CancellationUnknown, Reason: reasonCancellationUnknown}
	}

	if contract, ok := cancellationContractFor(call.Common()); ok && contract.packagePath == "context" {
		classifier.context = ssacall.CallResult(call, 0)
		classifier.processLifetime = ssacall.RunsOnceInProgramEntry(call)
	}
	// One walk carries the classifier's labels to every feasible return. A
	// return no action reaches is loss; a return only an opaque handoff reaches
	// is unknown, and that opacity excuses no other path's early return.
	outcome, witness := ssapath.EvaluateObligationWitness(ssapath.ObligationFlow{
		Start: call, NonNil: cancel, Successors: knowledge.Successors(), Terminates: knowledge.Terminates(),
		Instruction: classifier.obligation, Edge: classifier.edgeObligation,
	})
	switch outcome {
	case ssapath.ObligationViolated:
		return CancellationProof{Outcome: CancellationLost, Reason: reasonCancellationLost, Witness: witness}
	case ssapath.ObligationUncertain:
		return CancellationProof{Outcome: CancellationUnknown, Reason: reasonCancellationUnknown}
	case ssapath.ObligationHonored:
	}
	if classifier.transfers {
		return CancellationProof{Outcome: CancellationTransferred, Reason: reasonCancellationTransferred}
	}
	return CancellationProof{Outcome: CancellationReleased, Reason: reasonCancellationReleased}
}

// obligation and edgeObligation map this classifier's
// labels onto the shared flow lattice: a release or transfer is exact
// evidence, an ambiguous use is opaque, and a selected Done receive is an
// edge-local opaque observation of cancellation.
func (classifier *cancellationClassifier) obligation(instruction ssa.Instruction) ssapath.ObligationAction {
	return cancellationObligation(classifier.action(instruction))
}

func (classifier *cancellationClassifier) edgeObligation(from, to *ssa.BasicBlock) ssapath.ObligationAction {
	request := lifecycle.CompletionRequest{
		Target: classifier.cancel, InvokeTarget: true, Budget: classifier.budget(),
	}
	var completed bool
	if classifier.evidence != nil {
		completed = classifier.evidence.CompletionOnEdge(from, to, request).Proven()
	} else {
		completed = lifecycle.ProveCompletionOnEdge(from, to, request).Proven()
	}
	if completed {
		return ssapath.ObligationExact
	}
	if classifier.selectedDoneEdge(from, to) {
		return ssapath.ObligationUnknown
	}
	return ssapath.ObligationNone
}

func cancellationObligation(action cancellationAction) ssapath.ObligationAction {
	switch action {
	case cancellationActionRelease, cancellationActionTransfer:
		return ssapath.ObligationExact
	case cancellationActionUnknown:
		return ssapath.ObligationUnknown
	case cancellationActionNone:
	}
	return ssapath.ObligationNone
}
