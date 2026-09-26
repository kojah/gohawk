package cancellationownership

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
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

// deferredCaptureCell reports whether a store puts the cancel function into
// a cell that only directly deferred literals read.
func (classifier *cancellationClassifier) deferredCaptureCell(store *ssa.Store) bool {
	cell, ok := store.Addr.(*ssa.Alloc)
	if !ok || store.Val != classifier.cancel {
		return false
	}
	if stored, once := ssaflow.WrittenOnceCell(cell); !once || stored != classifier.cancel {
		return false
	}
	captured := false
	for _, user := range *cell.Referrers() {
		switch typed := user.(type) {
		case *ssa.Store:
			if typed != store {
				return false
			}
		case *ssa.MakeClosure:
			deferred, ok := deferredDirectly(typed)
			// A literal deferred before the store is registered before the
			// walk starts, so it could not be judged; the store stays opaque.
			if !ok || !ssaflow.InstructionDominates(store, deferred) {
				return false
			}
			captured = true
		default:
			return false
		}
	}
	return captured
}

// deferredDirectly returns the defer that is a literal's only use.
func deferredDirectly(closure *ssa.MakeClosure) (*ssa.Defer, bool) {
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
	if !ok || !classifier.capturesThroughDeferredCell(closure) {
		return cancellationLabel{}, false
	}
	for _, guard := range classifier.guards {
		if guard.Defer == deferred {
			return labelled(cancellationActionNone, reasonLabelResultGuardedDefer), true
		}
	}
	request := classifier.invokeRequest()
	request.Instruction = deferred
	if lifecycle.ProveCompletion(request).Proven() {
		return labelled(cancellationActionRelease, reasonLabelDeferredLiteralRelease), true
	}
	return cancellationLabel{}, false
}

func (classifier *cancellationClassifier) capturesThroughDeferredCell(closure *ssa.MakeClosure) bool {
	for _, binding := range closure.Bindings {
		cell, ok := binding.(*ssa.Alloc)
		if !ok {
			continue
		}
		for _, user := range *cell.Referrers() {
			if store, ok := user.(*ssa.Store); ok && classifier.deferredCaptureCell(store) {
				return true
			}
		}
	}
	return false
}

// resultGuardedReturn labels a return by the result-guarded literals that
// reach it: a release when one cancels given the values this return stores,
// unknown when a literal may or may not be deferred on the way or the answer
// is not known.
func (classifier *cancellationClassifier) resultGuardedReturn(returned *ssa.Return) (cancellationLabel, bool) {
	uncertain := false
	for _, guard := range classifier.guards {
		if !ssaflow.InstructionDominates(guard.Defer, returned) {
			uncertain = uncertain || ssaflow.InstructionMayFollow(guard.Defer, returned)
			continue
		}
		switch guard.CompletesAtReturn(classifier.invokeRequest(), returned, classifier.outcomeOf) {
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

func (classifier *cancellationClassifier) outcomeOf(value ssa.Value) (ssaflow.Outcome, bool) {
	if outcome, ok := ssaflow.ValueOutcome(value); ok {
		return outcome, true
	}
	if classifier.knowledge == nil {
		return ssaflow.OutcomeAny, false
	}
	switch classifier.knowledge.ResultOf(value, classifier.budget()) {
	case resultfacts.AlwaysNil:
		return ssaflow.OutcomeNil, true
	case resultfacts.AlwaysNonNil:
		return ssaflow.OutcomeNonNil, true
	case resultfacts.AlwaysTrue:
		return ssaflow.OutcomeTrue, true
	case resultfacts.AlwaysFalse:
		return ssaflow.OutcomeFalse, true
	case resultfacts.Unknown:
	}
	return ssaflow.OutcomeAny, false
}
