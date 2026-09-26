package resourcelifetime

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/syntax"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Released uses extend use-after-release from acquisitions to values a
// function is handed. The lifecycle summary proves the structure, a method
// called on a parameter after the function released it; this file decides
// the contract: the use must be an operation the value's type documents as
// failing after release, and the release one that invalidates it. A released
// use with no condition is the function's own defect and is reported at the
// use. One conditioned on the function's Boolean parameters is latent and is
// reported only at a call whose constant arguments trigger it, never inside
// the function, whose other callers may pass the other constant. A Commit is
// not a release here: it can fail before it finishes the transaction.

func reportReleasedUses(pass *analysis.Pass, evidence *lifecyclefacts.LifecycleEvidence, function *ssa.Function) {
	for _, proof := range evidence.ManifestReleasedUses(function) {
		if !releasedUseInvalidates(function.Params[proof.Parameter], proof.ReleasedUse) {
			continue
		}
		emitReleasedUse(pass, proof.UseCall, resourceReasonManifestReleasedUse)
		release := syntax.SourceRange(pass, proof.ReleaseCall.Pos())
		use := syntax.SourceRange(pass, proof.UseCall.Pos())
		check.Report(pass, check.ResourceUseAfterRelease, analysis.Diagnostic{
			Pos: use.Pos(), End: use.End(),
			Message: fmt.Sprintf("parameter %s is used after %s", function.Params[proof.Parameter].Name(), proof.Release),
			Related: []analysis.RelatedInformation{{Pos: release.Pos(), End: release.End(), Message: "parameter released here"}},
		})
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || call.Common().StaticCallee() == nil {
				continue
			}
			for _, latent := range evidence.ReleasedUsesAt(call) {
				if latent.Parameter >= len(call.Common().Args) || !releasedUseInvalidates(call.Common().Args[latent.Parameter], latent) {
					continue
				}
				emitReleasedUse(pass, call, resourceReasonLatentReleasedUse)
				source := syntax.SourceRange(pass, call.Pos())
				check.Report(pass, check.ResourceUseAfterRelease, analysis.Diagnostic{
					Pos: source.Pos(), End: source.End(),
					Message: fmt.Sprintf("%s calls %s after %s on this argument", call.Common().StaticCallee().Name(), latent.Use, latent.Release),
				})
				break
			}
		}
	}
}

// releasedUseInvalidates reports whether the value's type documents the
// release as invalidating it and the use as failing afterwards. The table
// names Rollback, not Commit, for a transaction: Commit can fail first.
func releasedUseInvalidates(value ssa.Value, use lifecyclefacts.ReleasedUse) bool {
	contract, ok := invalidationContract(value)
	return ok && slices.Contains(contract.releases, use.Release) && slices.Contains(contract.methods, use.Use)
}

func emitReleasedUse(pass *analysis.Pass, call *ssa.Call, reason resourceLifetimeReason) {
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), call.Pos())
	if !probe.Enabled() {
		return
	}
	probe.Decision(analysisTrace.Step{
		Reason: reason.String(), Outcome: analysisTrace.OutcomeRejected, Pos: call.Pos(),
		Details: map[string]string{"instruction": call.String()},
	})
}
