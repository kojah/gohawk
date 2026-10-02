package cancellationownership

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Deferred literals that capture the cancel function. Go captures a variable
// by reference, so a literal that uses cancel reads it from a cell the
// constructor's result was stored into. That store is a use the classifier
// can see through only when nothing else reads the cell: the cell holds the
// cancel function once, and every literal that captures it is deferred
// directly, after the store, where the walk will meet the defer. Each such
// literal is then asked of exactly, through the shared completion search.
// One that calls cancel on every return releases it where it is deferred.
// One whose call turns on a named result, as the cancel-on-error idiom calls
// cancel only while err is non-nil, settles nothing at the defer; each
// return it dominates is judged by the value it stores into that result.
// When the function also reads the cell itself, or defers a capturing
// literal before the store, the store stays an opaque use: a call through
// the loaded variable would not be recognized as the release, and an
// earlier defer is never met by the walk.

// proveDeferredCaptureCellWithin establishes the exact once-stored cancel
// whose only readers are literals directly deferred after the store. Unknown
// at cutoff cannot excuse the store as transparent or establish lost cleanup.
func (classifier *cancellationClassifier) proveDeferredCaptureCellWithin(store *ssa.Store, budget *ssaflow.SearchBudget) ssaflow.Proof {
	cell, ok := store.Addr.(*ssa.Alloc)
	if !ok || store.Val != classifier.cancel {
		return deferredCaptureProof(false, budget)
	}
	stored, once := ssaflow.WrittenOnceCellWithin(cell, budget)
	if !once || stored != classifier.cancel {
		return deferredCaptureProof(false, budget)
	}
	captured := false
	for user := range ssaflow.ReferrersWithin(cell, budget) {
		switch typed := user.(type) {
		case *ssa.Store:
			if typed != store {
				return deferredCaptureProof(false, budget)
			}
		case *ssa.MakeClosure:
			deferred, ok := deferredDirectlyWithin(typed, budget)
			// A defer registered before the store is never met by the obligation
			// walk after acquisition, so this store must remain opaque.
			if !ok || !ssaflow.InstructionDominatesWithin(store, deferred, budget) {
				return deferredCaptureProof(false, budget)
			}
			captured = true
		default:
			return deferredCaptureProof(false, budget)
		}
	}
	return deferredCaptureProof(captured, budget)
}

func deferredCaptureProof(proven bool, budget *ssaflow.SearchBudget) ssaflow.Proof {
	if budget.Exhausted() || budget.PoolExhausted() {
		return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	}
	state := ssaflow.EvidenceDisproven
	if proven {
		state = ssaflow.EvidenceProven
	}
	return ssaflow.Proof{State: state, Reason: ssaflow.EvidenceStructuralWalk, Provenance: ssaflow.EvidenceFromLocalSSA}
}

// deferredDirectlyWithin returns the defer that is a literal's only use.
func deferredDirectlyWithin(closure *ssa.MakeClosure, budget *ssaflow.SearchBudget) (*ssa.Defer, bool) {
	if !budget.Spend() || closure.Referrers() == nil {
		return nil, false
	}
	users := *closure.Referrers()
	if len(users) != 1 {
		return nil, false
	}
	deferred, ok := users[0].(*ssa.Defer)
	return deferred, ok && deferred.Call.Value == closure && len(deferred.Call.Args) == 0
}

func (classifier *cancellationClassifier) invokeRequest() lifecycle.CompletionRequest {
	return lifecycle.CompletionRequest{Target: classifier.cancel, InvokeTarget: true, Budget: classifier.budget()}
}

// deferredLiteralLabel labels a directly deferred literal that captures the
// cancel function through a cell only such literals read.
func (classifier *cancellationClassifier) deferredLiteralLabel(deferred *ssa.Defer) (cancellationLabel, bool) {
	closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
	if !ok {
		return cancellationLabel{}, false
	}
	budget := classifier.budget()
	capture := classifier.proveDeferredCaptureWithin(closure, budget)
	if capture.State == ssaflow.EvidenceUnknown {
		return labelled(cancellationActionUnknown, reasonLabelDeferredClosure), true
	}
	if !capture.Proven() {
		return cancellationLabel{}, false
	}
	for _, guard := range classifier.guards {
		if !budget.Spend() {
			return labelled(cancellationActionUnknown, reasonLabelDeferredClosure), true
		}
		if guard.Defer == deferred {
			return labelled(cancellationActionNone, reasonLabelResultGuardedDefer), true
		}
	}
	request := lifecycle.CompletionRequest{Target: classifier.cancel, InvokeTarget: true, Budget: budget, Instruction: deferred}
	proof := lifecycle.ProveCompletion(request)
	if proof.Proven() {
		return labelled(cancellationActionRelease, reasonLabelDeferredLiteralRelease), true
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return labelled(cancellationActionUnknown, reasonLabelDeferredClosure), true
	}
	return cancellationLabel{}, false
}

func (classifier *cancellationClassifier) proveDeferredCaptureWithin(closure *ssa.MakeClosure, budget *ssaflow.SearchBudget) ssaflow.Proof {
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return deferredCaptureProof(false, budget)
		}
		cell, ok := binding.(*ssa.Alloc)
		if !ok {
			continue
		}
		for user := range ssaflow.ReferrersWithin(cell, budget) {
			if store, ok := user.(*ssa.Store); ok {
				proof := classifier.proveDeferredCaptureCellWithin(store, budget)
				if proof.State != ssaflow.EvidenceDisproven {
					return proof
				}
			}
		}
	}
	return deferredCaptureProof(false, budget)
}

// resultGuardedReturn labels a return by the result-guarded literals that
// reach it: a release when one cancels given the values this return stores,
// unknown when a literal may or may not be deferred on the way or the answer
// is not known.
func (classifier *cancellationClassifier) resultGuardedReturn(returned *ssa.Return) (cancellationLabel, bool) {
	uncertain := false
	budget := classifier.budget()
	outcomeOf := func(value ssa.Value) (ssaflow.Outcome, bool) {
		return classifier.outcomeOfWithin(value, budget)
	}
	for _, guard := range classifier.guards {
		reaching := guard.ProveReachesReturn(returned, budget)
		if !reaching.Proven() {
			uncertain = uncertain || reaching.State == ssaflow.EvidenceUnknown
			continue
		}
		request := lifecycle.CompletionRequest{Target: classifier.cancel, InvokeTarget: true, Budget: budget}
		switch guard.CompletesAtReturn(request, returned, outcomeOf) {
		case ssaflow.EvidenceProven:
			return labelled(cancellationActionRelease, reasonLabelResultGuardedRelease), true
		case ssaflow.EvidenceUnknown:
			uncertain = true
		case ssaflow.EvidenceDisproven:
		}
	}
	if uncertain {
		return labelled(cancellationActionUnknown, reasonLabelResultGuardedUnknown), true
	}
	return cancellationLabel{}, false
}

func (classifier *cancellationClassifier) outcomeOfWithin(value ssa.Value, budget *ssaflow.SearchBudget) (ssaflow.Outcome, bool) {
	if outcome, ok := ssaflow.ValueOutcome(value); ok {
		return outcome, true
	}
	if classifier.knowledge == nil {
		return ssaflow.OutcomeAny, false
	}
	return classifier.knowledge.ResultOf(value, budget).Outcome()
}

// retainResultGuardsWithin publishes the filtered census only when every
// captured guard is decided. A truncated list cannot start obligation flow.
func (classifier *cancellationClassifier) retainResultGuardsWithin(guards []lifecycle.ResultGuard, budget *ssaflow.SearchBudget) ssaflow.Proof {
	var retained []lifecycle.ResultGuard
	for _, guard := range guards {
		if !budget.Spend() {
			return deferredCaptureProof(false, budget)
		}
		if closure, ok := guard.Defer.Call.Value.(*ssa.MakeClosure); ok {
			capture := classifier.proveDeferredCaptureWithin(closure, budget)
			if capture.State == ssaflow.EvidenceUnknown {
				return capture
			}
			if capture.Proven() {
				retained = append(retained, guard)
			}
		}
	}
	classifier.guards = retained
	return deferredCaptureProof(true, budget)
}

func (classifier *cancellationClassifier) deferredCaptureStoreLabel(instruction ssa.Instruction) (cancellationLabel, bool) {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return cancellationLabel{}, false
	}
	capture := classifier.proveDeferredCaptureCellWithin(store, classifier.budget())
	if capture.State == ssaflow.EvidenceUnknown {
		return labelled(cancellationActionUnknown, reasonLabelStored), true
	}
	return cancellationLabel{}, capture.Proven()
}
