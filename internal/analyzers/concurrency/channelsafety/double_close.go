package channelsafety

// Double-close evidence is an ordered pair of exact channel identities inside
// one basic block. Keeping branches and backedges outside this proof avoids
// combining incompatible paths or successive instances of a loop allocation.

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type closeWitness struct {
	value ssa.Value
	site  token.Pos
}

type doubleCloseProof struct {
	ssaflow.Proof
	first token.Pos
}

func reportDoubleCloses(pass *analysis.Pass, function *ssa.Function, effects map[ssa.Instruction][]concurrencyfacts.Operation) {
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	storage := ssaflow.NewStorage(budget)
	for _, block := range function.Blocks {
		var previous []closeWitness
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return
			}
			reported := false
			for _, operation := range effects[instruction] {
				if operation.Kind != concurrencyfacts.Close {
					continue
				}
				value := operation.Resource.Value
				// A known nil close already panics; it cannot witness a later close.
				if ssaflow.DefinitelyNil(value) {
					continue
				}
				if !reported {
					reported = reportRepeatedClose(pass, storage, budget, closeWitness{value: value, site: instruction.Pos()}, previous)
				}
				previous = append(previous, closeWitness{value: value, site: instruction.Pos()})
			}
		}
	}
}

func reportRepeatedClose(
	pass *analysis.Pass, storage *ssaflow.Storage, budget *ssaflow.SearchBudget, current closeWitness, previous []closeWitness,
) bool {
	if len(previous) == 0 {
		return false
	}
	probe := trace.For(pass, "channelsafety", string(check.ChannelDoubleClose), current.site)
	probe.Candidate(trace.Step{Reason: "close-follows-close", Outcome: trace.OutcomeObserved})
	proof := proveRepeatedClose(storage, budget, current.value, previous)
	outcome := trace.OutcomeUnknown
	if proof.Proven() {
		outcome = trace.OutcomeRejected
	}
	probe.Decision(trace.Step{Reason: string(proof.Reason), Outcome: outcome})
	if !proof.Proven() {
		return false
	}
	source := syntax.SourceRange(pass, current.site)
	first := syntax.SourceRange(pass, proof.first)
	check.Report(pass, check.ChannelDoubleClose, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(), Message: "close follows close of channel",
		Related: []analysis.RelatedInformation{{Pos: first.Pos(), End: first.End(), Message: "channel first closed here"}},
	})
	return true
}

func proveRepeatedClose(storage *ssaflow.Storage, budget *ssaflow.SearchBudget, value ssa.Value, previous []closeWitness) doubleCloseProof {
	for _, earlier := range previous {
		if !budget.Spend() {
			return doubleCloseProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: "double-close-budget-exhausted"}}
		}
		identity := storage.Same(value, earlier.value)
		if !identity.Proven() {
			continue
		}
		return doubleCloseProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "double-close-proven"}, first: earlier.site}
	}
	return doubleCloseProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: "close-channel-identity-not-proven"}}
}
