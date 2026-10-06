package resourcelifetime

import (
	"go/constant"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"

	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// HTTP acquisition contracts distinguish known body-bearing operations from
// exact local protocol shapes that may acquire no body. They feed the ordinary
// resource flow; no separate cleanup or reporting decision is made here.

func httpAcquisitionBoundary(pass *analysis.Pass, call *ssa.Call, budget *proofs.SearchBudget) resourceLifetimeReason {
	head := proveHeadAcquisitionWithin(call, budget)
	if head.State == proofs.EvidenceUnknown && head.Reason != resourceReasonNone {
		return head.Reason
	}
	if head.Reason != resourceReasonNone {
		// The rule applied to a Client.Do and declined; say which input failed
		// so a trace of the site does not need the source to explain it.
		probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
		if probe.Enabled() {
			probe.Considered(analysisTrace.Step{
				Reason: head.Reason.String(), Outcome: analysisTrace.OutcomeRejected, Pos: call.Pos(), Function: call.Parent().String(),
			})
		}
	}
	proof := proveLocalHeaderOnlyAcquisitionWithin(call, budget)
	if proof.Reason != resourceReasonNone {
		probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
		if probe.Enabled() {
			outcome := analysisTrace.OutcomeUnknown
			if proof.Proven() {
				outcome = analysisTrace.OutcomeAccepted
			}
			probe.Evidence(analysisTrace.Step{
				Reason: proof.Reason.String(), Outcome: outcome, Pos: call.Pos(), Function: call.Parent().String(),
			})
		}
	}
	if proof.State == proofs.EvidenceUnknown && proof.Reason == resourceReasonBudgetExhausted {
		return proof.Reason
	}
	if proof.Proven() {
		return resourceReasonHeaderOnlyAcquisition
	}
	return resourceReasonNone
}

// A HEAD response through an unconfigured client normally carries
// http.NoBody, not an acquired body: the standard transport reads no body for
// HEAD, and without Client.Timeout there is no cancelTimerBody around it.
// DefaultTransport and DefaultClient are replaceable, so this is uncertainty
// about acquisition, never a proof that closing is unnecessary. The client is
// either a fresh zero-value local used only by Do, or the package default
// client with no visible reconfiguration in this function or a visible callee;
// hidden cross-package mutation of the defaults is the same accepted coverage
// gap as the local-server boundary below. The request must be the direct
// HEAD constructor result, possibly rebound through WithContext or Clone and
// with its Header map edited, none of which can change Method. Any other use
// of the request or the client, such as a helper receiving it, could.
// https://github.com/vishen/go-chromecast/blob/5dd70bb91787fe28e3d8946682c66cb2a1d61d21/application/application.go#L723-L732
// https://github.com/alexellis/arkade/blob/0a0a800fd7554d4eddb1856f9ef8a21214e95bab/pkg/get/get.go#L236-L244
// https://github.com/deweizhu/bookget/blob/2cdbf6d6c3ce70355a5c4411c0faf3450e9ae877/pkg/downloader/downloader.go#L510-L522
func proveHeadAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	return findHeadAcquisitionWithin(call, budget).within(budget)
}

func findHeadAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	common := call.Common()
	if !ssacall.CallMatchesSymbol(common, httpClientDo) || len(common.Args) != 2 {
		return resourceProof{}
	}
	// A request that never came from a HEAD constructor is outside this rule,
	// so it stays silent; the rule explains itself only when it applied.
	if !headConstructedWithin(common.Args[1], budget) {
		return resourceProof{}
	}
	if !headRequestWithin(common.Args[1], budget) {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonHeadRequestModified}
	}
	client := proveHeadClientUnconfiguredWithin(common.Args[0], call.Parent(), budget)
	if client.State == proofs.EvidenceUnknown {
		return client
	}
	if !client.Proven() {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonHeadClientNotUnconfigured}
	}
	return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonHeadAcquisition}
}

var (
	httpClientDo           = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Do"})
	httpDefaultClient      = syntax.PackageVariable("net/http", "DefaultClient")
	httpDefaultTransport   = syntax.PackageVariable("net/http", "DefaultTransport")
	httpRequestWithContext = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Request", Name: "WithContext"})
	httpRequestClone       = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Request", Name: "Clone"})
	// Header edits through the standard map methods cannot change the request
	// method; the map handed anywhere else is rejected so the rule stays small.
	httpHeaderEdits = []syntax.Symbol{
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Set"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Add"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Get"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Del"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Values"}),
	}
)

func constantString(value ssa.Value) string {
	text, ok := value.(*ssa.Const)
	if !ok || text.Value == nil || text.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(text.Value)
}
