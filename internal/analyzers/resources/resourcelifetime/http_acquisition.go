package resourcelifetime

import (
	"go/constant"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// HTTP acquisition contracts distinguish known body-bearing operations from
// exact local protocol shapes that may acquire no body. They feed the ordinary
// resource flow; no separate cleanup or reporting decision is made here.

func httpAcquisitionBoundary(pass *analysis.Pass, call *ssa.Call) resourceLifetimeReason {
	if headAcquisitionUncertain(call) {
		return resourceReasonHeadAcquisition
	}
	proof := localHeaderOnlyAcquisition(call)
	if proof.Reason != "" {
		outcome := analysisTrace.OutcomeUnknown
		if proof.Proven() {
			outcome = analysisTrace.OutcomeAccepted
		}
		analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos()).Evidence(analysisTrace.Step{
			Reason: string(proof.Reason), Outcome: outcome, Pos: call.Pos(), Function: call.Parent().String(),
		})
	}
	if proof.Proven() {
		return resourceReasonHeaderOnlyAcquisition
	}
	return ""
}

// A HEAD response through an unchanged zero-value client normally carries
// http.NoBody, not an acquired body. DefaultTransport is replaceable, so this
// is uncertainty about acquisition, never a proof that closing is unnecessary.
// Explicit transports and client timeouts stay outside this boundary: even a
// bodyless response can own a cancelTimerBody when Client.Timeout is nonzero.
// https://github.com/vishen/go-chromecast/blob/5dd70bb91787fe28e3d8946682c66cb2a1d61d21/application/application.go#L723-L732
func headAcquisitionUncertain(call *ssa.Call) bool {
	common := call.Common()
	if !ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http", Receiver: "Client", Name: "Do",
	})) || len(common.Args) != 2 {
		return false
	}
	client, ok := common.Args[0].(*ssa.Alloc)
	if !ok || !onlyHTTPDoUses(client) {
		return false
	}
	request, ok := common.Args[1].(*ssa.Extract)
	if !ok || request.Index != 0 || !onlyHTTPDoUses(request) {
		return false
	}
	constructor, ok := request.Tuple.(*ssa.Call)
	return ok && ssaflow.CallMatchesSymbol(constructor.Common(), syntax.PackageFunction("net/http", "NewRequest")) &&
		constantString(constructor.Common().Args[0]) == "HEAD"
}

// Only direct Do calls may observe these fresh values. Field addresses, aliases
// and helper escapes would hide configuration or method mutation.
func onlyHTTPDoUses(value ssa.Value) bool {
	refs := value.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		call, ok := ref.(*ssa.Call)
		if !ok || !ssaflow.CallMatchesSymbol(call.Common(), syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "net/http", Receiver: "Client", Name: "Do",
		})) {
			return false
		}
	}
	return true
}

// The exact local endpoint supplies positive protocol evidence, not a claim
// that missing effects imply purity. Hidden cross-package mutations of the
// global default client/transport remain an accepted coverage gap; visible
// overrides, writer escapes and uncertain response framing reject this rule.
// https://github.com/go-pkgz/auth/blob/5f6d12c4a12e6cf7b1934dc10d8127969ac7e7b1/token/jwt_test.go#L629-L650
func localHeaderOnlyAcquisition(call *ssa.Call) ssaflow.Proof {
	if !ssaflow.CallMatchesSymbol(call.Common(), syntax.PackageFunction("net/http", "Get")) || len(call.Common().Args) != 1 {
		return ssaflow.Proof{}
	}
	server := localHTTPServer(call.Common().Args[0])
	if server == nil {
		return ssaflow.Proof{Reason: "local-server-identity-unavailable"}
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
		return ssaflow.Proof{Reason: "local-server-handler-unavailable"}
	}
	budget := ssaflow.NewSearchBudget(4000)
	effects := newHTTPWriterEffects()
	if effects.overrides.Function(call.Parent(), budget) || effects.overrides.Function(function, budget) {
		return ssaflow.Proof{Reason: "local-server-client-override-unresolved"}
	}
	if !effects.headerOnly(function.Params[0], budget) {
		return ssaflow.Proof{Reason: "local-server-writer-effects-unavailable"}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "local-server-header-only-effects"}
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
		if !ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "net/http/httptest", Receiver: "Server", Name: "Close",
		})) {
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
					if operand != nil && ssaflow.MayAlias(*operand, writer) && !effects.writerUse(instruction, writer, budget) {
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
		if !ssaflow.MayAlias(binding.Supplied, writer) {
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

func constantString(value ssa.Value) string {
	text, ok := value.(*ssa.Const)
	if !ok || text.Value == nil || text.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(text.Value)
}
