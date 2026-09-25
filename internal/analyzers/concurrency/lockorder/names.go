package lockorder

import (
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// How a diagnostic names a lock. The analysis keys locks by identities and
// classes built from SSA, such as "(*pkg.Cache).Put.c.mu", which are exact
// but unreadable. A reader knows the lock by the receiver written at its Lock
// call, so diagnostics use that text when the call is in this package and
// fall back to the identity otherwise. Naming never affects what is reported.

func (acquired lockAcquisition) verb() string {
	if acquired.read {
		return "read-locked"
	}
	return "locked"
}

// displayName names an acquisition for a reader: the receiver as written at
// the Lock call, such as `l.mu`, or the lock's class when the call is in
// another package.
func (acquired lockAcquisition) displayName(pass *analysis.Pass) string {
	if text := syntax.CallReceiverText(pass, acquired.position); text != "" {
		return "`" + text + "`"
	}
	return displayClass(pass, acquired.class)
}

// displayClass drops the current package's path from a lock class, so
// "*example.com/shop.Ledger.mu" reads "Ledger.mu" inside package shop.
func displayClass(pass *analysis.Pass, class string) string {
	class = strings.TrimPrefix(class, "*")
	return strings.TrimPrefix(class, pass.Pkg.Path()+".")
}

// lockEvidence cites the whole Lock call, so the evidence at the reported
// acquisition covers the same span as the diagnostic and labels it.
func lockEvidence(pass *analysis.Pass, position token.Pos, message string) analysis.RelatedInformation {
	source := syntax.SourceRange(pass, position)
	return analysis.RelatedInformation{Pos: source.Pos(), End: source.End(), Message: message}
}

// callEvidence widens each helper call on an acquisition's route to the
// span of the call expression.
func callEvidence(pass *analysis.Pass, calls []analysis.RelatedInformation) []analysis.RelatedInformation {
	evidence := make([]analysis.RelatedInformation, 0, len(calls))
	for _, call := range calls {
		evidence = append(evidence, lockEvidence(pass, call.Pos, call.Message))
	}
	return evidence
}

// lockName names the lock with identity by the receiver of its first direct
// Lock or RLock call in this function.
func (flow lockFlowContext) lockName(identity string) string {
	if acquisition := flow.directAcquisition(identity, false); acquisition != nil {
		if text := syntax.CallReceiverText(flow.pass, acquisition.Pos()); text != "" {
			return "`" + text + "`"
		}
	}
	return identity
}

// directAcquisition returns the function's direct Lock or RLock call for
// identity. With only set, it returns the call only when it is the one
// acquisition of that lock in the function, so evidence citing it is the
// acquisition on every path rather than one of several.
func (flow lockFlowContext) directAcquisition(identity string, only bool) ssa.Instruction {
	acquisitions := flow.acquisitions[identity]
	if only && len(acquisitions) != 1 {
		return nil
	}
	for _, acquisition := range acquisitions {
		if _, direct := directMutexEffect(acquisition); direct {
			return acquisition
		}
	}
	return nil
}

// acquisitionEvidence cites the one acquisition of identity, or nothing when
// the function acquires it more than once.
func (flow lockFlowContext) acquisitionEvidence(identity string) []analysis.RelatedInformation {
	acquisition := flow.directAcquisition(identity, true)
	if acquisition == nil {
		return nil
	}
	effect, _ := directMutexEffect(acquisition)
	return []analysis.RelatedInformation{
		lockEvidence(flow.pass, acquisition.Pos(), flow.lockName(identity)+" is "+effect.acquired.verb()+" here"),
	}
}
