package resourcelifetime

import (
	"go/token"
	"strings"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// A request to an unchanged local httptest server whose handler writes only
// headers gets a response with no body to close. This file proves that shape
// from the exact server value, the client that sends the request, and every
// effect the handler has on its writer. Anything it cannot see through, such
// as a writer handed to unknown code or a default client changed in a
// visible callee, leaves the ordinary cleanup obligation in place.

// The exact local endpoint supplies positive protocol evidence, not a claim
// that missing effects imply purity. Hidden cross-package mutations of the
// global default client/transport remain an accepted coverage gap; visible
// overrides, writer escapes and uncertain response framing reject this rule.
// https://github.com/go-pkgz/auth/blob/5f6d12c4a12e6cf7b1934dc10d8127969ac7e7b1/token/jwt_test.go#L629-L650
//
// The server's own client, as in `server.Client().Get(server.URL)`, is the
// same request: httptest configures that client with a transport to the
// server and no timeout, so a header-only response has no body either.
func proveLocalHeaderOnlyAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	return findLocalHeaderOnlyAcquisitionWithin(call, budget).within(budget)
}

func findLocalHeaderOnlyAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	url, ok := localGetURL(call.Common())
	if !ok {
		return resourceProof{}
	}
	if !budget.Spend() {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	server := localHTTPServerWithin(url, budget)
	if server == nil || !serverClientOrDefault(call.Common(), server) {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonLocalServerIdentityUnavailable}
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentMakeInterface
	function, ok := ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(forms).Within(budget), server.Common().Args[0],
		func(_ ssaflow.ReachingWalk, handler ssa.Value) (*ssa.Function, bool) {
			if closure, captured := handler.(*ssa.MakeClosure); captured {
				handler = closure.Fn
			}
			function, direct := handler.(*ssa.Function)
			return function, direct
		}, func(function *ssa.Function) *ssa.Function { return function })
	if !ok || len(function.Params) != 2 {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonLocalServerHandlerUnavailable}
	}
	child := budget.Within(httpEffectsBudget)
	effects := newHTTPWriterEffects()
	if effects.overrides.Function(call.Parent(), child) || effects.overrides.Function(function, child) {
		return localHTTPEffectProof(false, resourceReasonLocalServerClientOverrideUnresolved, child)
	}
	if !effects.headerOnly(function.Params[0], child) {
		return localHTTPEffectProof(false, resourceReasonLocalServerWriterEffectsUnavailable, child)
	}
	return localHTTPEffectProof(true, resourceReasonLocalServerHeaderOnlyEffects, child)
}

// Effect-child exhaustion cannot become a complete protocol decline: that
// would let ordinary flow report without knowing whether the body exists.
func localHTTPEffectProof(found bool, reason resourceLifetimeReason, budget *proofs.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	state := proofs.EvidenceDisproven
	if found {
		state = proofs.EvidenceProven
	}
	return resourceProof{State: state, Reason: reason}
}

var (
	httpGet              = syntax.PackageFunction("net/http", "Get")
	httpClientGet        = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Get"})
	httptestServerClient = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http/httptest", Receiver: "Server", Name: "Client"})
)

// localGetURL returns the URL argument of http.Get or Client.Get.
func localGetURL(common *ssa.CallCommon) (ssa.Value, bool) {
	switch {
	case ssacall.CallMatchesSymbol(common, httpGet) && len(common.Args) == 1:
		return common.Args[0], true
	case ssacall.CallMatchesSymbol(common, httpClientGet) && len(common.Args) == 2:
		return common.Args[1], true
	}
	return nil, false
}

// serverClientOrDefault accepts http.Get, which uses the default client, or
// Client.Get on the result of server.Client() for the same server.
func serverClientOrDefault(common *ssa.CallCommon, server *ssa.Call) bool {
	if !ssacall.CallMatchesSymbol(common, httpClientGet) {
		return true
	}
	client, ok := common.Args[0].(*ssa.Call)
	return ok && ssacall.CallMatchesSymbol(client.Common(), httptestServerClient) && ssaflow.CallReceiver(client.Common()) == server
}

func localHTTPServerWithin(url ssa.Value, budget *proofs.SearchBudget) *ssa.Call {
	if path, ok := url.(*ssa.BinOp); ok && path.Op == token.ADD && strings.HasPrefix(constantString(path.Y), "/") {
		url = path.X
	}
	load, ok := url.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !syntax.NamedType(field.X.Type(), "net/http/httptest", "Server") || field.Field != 0 {
		return nil
	}
	server, ok := field.X.(*ssa.Call)
	if !ok || !ssacall.CallMatchesSymbol(server.Common(), syntax.PackageFunction("net/http/httptest", "NewServer")) {
		return nil
	}
	if !unmodifiedHTTPServerWithin(server, budget) {
		return nil
	}
	return server
}

// httptest.Server.URL is its first field. Only reading that field and closing
// the server are permitted; Config/Listener access could replace the endpoint.
func unmodifiedHTTPServerWithin(server *ssa.Call, budget *proofs.SearchBudget) bool {
	if server.Referrers() == nil {
		return false
	}
	for _, ref := range *server.Referrers() {
		if !budget.Spend() {
			return false
		}
		if field, ok := ref.(*ssa.FieldAddr); ok && field.Field == 0 && field.Referrers() != nil {
			for _, use := range *field.Referrers() {
				if !budget.Spend() {
					return false
				}
				load, loaded := use.(*ssa.UnOp)
				if !loaded || load.Op != token.MUL {
					return false
				}
			}
			continue
		}
		common := ssaflow.InstructionCall(ref)
		if client, ok := ref.(*ssa.Call); ok && ssacall.CallMatchesSymbol(common, httptestServerClient) {
			if !onlyClientGetUsesWithin(client, budget) {
				return false
			}
			continue
		}
		if !ssacall.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "net/http/httptest", Receiver: "Server", Name: "Close",
		})) {
			return false
		}
	}
	return true
}

// onlyClientGetUses reports whether the server's client is used only as the
// receiver of Get calls. Any other use, such as setting Timeout or storing
// the client, could give the response a body wrapper.
func onlyClientGetUsesWithin(client *ssa.Call, budget *proofs.SearchBudget) bool {
	refs := client.Referrers()
	if refs == nil {
		return false
	}
	for _, ref := range *refs {
		if !budget.Spend() {
			return false
		}
		call, ok := ref.(*ssa.Call)
		if !ok || !ssacall.CallMatchesSymbol(call.Common(), httpClientGet) || call.Common().Args[0] != client {
			return false
		}
	}
	return true
}
