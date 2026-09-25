package resourcelifetime

import (
	"go/constant"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
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
func localHeaderOnlyAcquisition(call *ssa.Call) resourceProof {
	url, ok := localGetURL(call.Common())
	if !ok {
		return resourceProof{}
	}
	server := localHTTPServer(url)
	if server == nil || !serverClientOrDefault(call.Common(), server) {
		return resourceProof{Reason: resourceReasonLocalServerIdentityUnavailable}
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentMakeInterface
	function, ok := ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(forms), server.Common().Args[0],
		func(_ ssaflow.ReachingWalk, handler ssa.Value) (*ssa.Function, bool) {
			if closure, captured := handler.(*ssa.MakeClosure); captured {
				handler = closure.Fn
			}
			function, direct := handler.(*ssa.Function)
			return function, direct
		}, func(function *ssa.Function) *ssa.Function { return function })
	if !ok || len(function.Params) != 2 {
		return resourceProof{Reason: resourceReasonLocalServerHandlerUnavailable}
	}
	budget := ssaflow.NewSearchBudget(httpEffectsBudget)
	effects := newHTTPWriterEffects()
	if effects.overrides.Function(call.Parent(), budget) || effects.overrides.Function(function, budget) {
		return resourceProof{Reason: resourceReasonLocalServerClientOverrideUnresolved}
	}
	if !effects.headerOnly(function.Params[0], budget) {
		return resourceProof{Reason: resourceReasonLocalServerWriterEffectsUnavailable}
	}
	return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonLocalServerHeaderOnlyEffects}
}

var (
	httpGet              = syntax.PackageFunction("net/http", "Get")
	httpClientGet        = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Get"})
	httptestServerClient = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http/httptest", Receiver: "Server", Name: "Client"})
)

// localGetURL returns the URL argument of http.Get or Client.Get.
func localGetURL(common *ssa.CallCommon) (ssa.Value, bool) {
	switch {
	case ssaflow.CallMatchesSymbol(common, httpGet) && len(common.Args) == 1:
		return common.Args[0], true
	case ssaflow.CallMatchesSymbol(common, httpClientGet) && len(common.Args) == 2:
		return common.Args[1], true
	}
	return nil, false
}

// serverClientOrDefault accepts http.Get, which uses the default client, or
// Client.Get on the result of server.Client() for the same server.
func serverClientOrDefault(common *ssa.CallCommon, server *ssa.Call) bool {
	if !ssaflow.CallMatchesSymbol(common, httpClientGet) {
		return true
	}
	client, ok := common.Args[0].(*ssa.Call)
	return ok && ssaflow.CallMatchesSymbol(client.Common(), httptestServerClient) && ssaflow.CallReceiver(client.Common()) == server
}

func localHTTPServer(url ssa.Value) *ssa.Call {
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
	if !ok || !ssaflow.CallMatchesSymbol(server.Common(), syntax.PackageFunction("net/http/httptest", "NewServer")) {
		return nil
	}
	if !unmodifiedHTTPServer(server) {
		return nil
	}
	return server
}

// httptest.Server.URL is its first field. Only reading that field and closing
// the server are permitted; Config/Listener access could replace the endpoint.
func unmodifiedHTTPServer(server *ssa.Call) bool {
	if server.Referrers() == nil {
		return false
	}
	for _, ref := range *server.Referrers() {
		if field, ok := ref.(*ssa.FieldAddr); ok && field.Field == 0 && field.Referrers() != nil {
			for _, use := range *field.Referrers() {
				load, loaded := use.(*ssa.UnOp)
				if !loaded || load.Op != token.MUL {
					return false
				}
			}
			continue
		}
		common := ssaflow.InstructionCall(ref)
		if client, ok := ref.(*ssa.Call); ok && ssaflow.CallMatchesSymbol(common, httptestServerClient) {
			if !onlyClientGetUses(client) {
				return false
			}
			continue
		}
		if !ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
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
func onlyClientGetUses(client *ssa.Call) bool {
	refs := client.Referrers()
	if refs == nil {
		return false
	}
	for _, ref := range *refs {
		call, ok := ref.(*ssa.Call)
		if !ok || !ssaflow.CallMatchesSymbol(call.Common(), httpClientGet) || call.Common().Args[0] != client {
			return false
		}
	}
	return true
}

type httpWriterEffects struct {
	writers   *ssaflow.CallGraphMemo[*ssa.Parameter, bool]
	overrides *ssaflow.FunctionSummaries[bool]
}

func newHTTPWriterEffects() *httpWriterEffects {
	effects := &httpWriterEffects{writers: ssaflow.NewCallGraphMemo[*ssa.Parameter, bool]()}
	effects.overrides = ssaflow.NewFunctionSummaries(effects.visibleOverrides, func(ssaflow.SummaryUnavailable) bool { return true })
	return effects
}

func (effects *httpWriterEffects) headerOnly(writer *ssa.Parameter, budget *ssaflow.SearchBudget) bool {
	return effects.writers.Summarize(writer, writer.Parent(), budget, func() bool {
		for _, block := range writer.Parent().Blocks {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					return false
				}
				for _, operand := range instruction.Operands(nil) {
					if operand != nil && heapmodel.MayAlias(*operand, writer) && !effects.writerUse(instruction, writer, budget) {
						return false
					}
				}
			}
		}
		return true
	}, func(ssaflow.SummaryUnavailable, bool) bool { return false })
}

func (effects *httpWriterEffects) writerUse(instruction ssa.Instruction, writer ssa.Value, budget *ssaflow.SearchBudget) bool {
	if _, ok := instruction.(*ssa.ChangeInterface); ok {
		return true
	}
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "SetCookie")) {
		return true
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http", Receiver: "ResponseWriter", Name: "Header",
	})) {
		return benignHTTPHeaders(call, budget)
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http", Receiver: "ResponseWriter", Name: "WriteHeader",
	})) {
		code, ok := common.Args[0].(*ssa.Const)
		if !ok || code.Value == nil || code.Value.Kind() != constant.Int {
			return false
		}
		n, exact := constant.Int64Val(code.Value)
		return exact && n >= 200 && n <= 599 && (n < 300 || n >= 400)
	}
	callee, closure := ssaflow.DirectCallee(common)
	// A visible helper must preserve the same restriction for every parameter
	// receiving the writer. Missing bodies or captured/dynamic dispatch are not
	// evidence of an empty effect set, so they cannot establish empty framing.
	found := false
	for _, binding := range ssaflow.CallBindings(common, callee, closure) {
		if !budget.Spend() {
			return false
		}
		if !heapmodel.MayAlias(binding.Supplied, writer) {
			continue
		}
		parameter, ok := binding.Local.(*ssa.Parameter)
		if !ok || !effects.headerOnly(parameter, budget) {
			return false
		}
		found = true
	}
	return found
}

func benignHTTPHeaders(header *ssa.Call, budget *ssaflow.SearchBudget) bool {
	if header.Referrers() == nil {
		return false
	}
	for _, ref := range *header.Referrers() {
		if !budget.Spend() {
			return false
		}
		call, ok := ref.(*ssa.Call)
		if !ok || len(call.Common().Args) < 2 {
			return false
		}
		if !ssaflow.CallMatchesAnySymbol(call.Common(),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Set"}),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Add"})) {
			return false
		}
		key := strings.ToLower(constantString(call.Common().Args[1]))
		switch key {
		case "", "content-length", "transfer-encoding", "trailer", "connection", "upgrade", "location":
			return false
		}
	}
	return true
}

// This query excludes visible overrides, not unseen global effects. Only
// visible helper bodies are expanded; exhausted or recursive searches reject
// the acquisition boundary instead of interpreting missing work as purity.
func (effects *httpWriterEffects) visibleOverrides(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return true
			}
			for _, operand := range instruction.Operands(nil) {
				if operand != nil && ssaflow.ValueMatchesAnySymbol(*operand,
					syntax.PackageVariable("net/http", "DefaultClient"), syntax.PackageVariable("net/http", "DefaultTransport")) {
					return true
				}
			}
			callee, _ := ssaflow.DirectCallee(ssaflow.InstructionCall(instruction))
			if callee != nil && len(callee.Blocks) != 0 && effects.overrides.Function(callee, budget) {
				return true
			}
		}
	}
	return false
}
