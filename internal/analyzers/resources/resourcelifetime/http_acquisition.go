package resourcelifetime

import (
	"go/constant"
	"go/token"
	"go/types"

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
	head := headAcquisition(call)
	if head.State == ssaflow.EvidenceUnknown && head.Reason != resourceReasonNone {
		return resourceReasonHeadAcquisition
	}
	if head.Reason != resourceReasonNone {
		// The rule applied to a Client.Do and declined; say which input failed
		// so a trace of the site does not need the source to explain it.
		analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos()).Considered(analysisTrace.Step{
			Reason: head.Reason.String(), Outcome: analysisTrace.OutcomeRejected, Pos: call.Pos(), Function: call.Parent().String(),
		})
	}
	proof := localHeaderOnlyAcquisition(call)
	if proof.Reason != resourceReasonNone {
		outcome := analysisTrace.OutcomeUnknown
		if proof.Proven() {
			outcome = analysisTrace.OutcomeAccepted
		}
		analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos()).Evidence(analysisTrace.Step{
			Reason: proof.Reason.String(), Outcome: outcome, Pos: call.Pos(), Function: call.Parent().String(),
		})
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
func headAcquisition(call *ssa.Call) resourceProof {
	common := call.Common()
	if !ssaflow.CallMatchesSymbol(common, httpClientDo) || len(common.Args) != 2 {
		return resourceProof{}
	}
	// A request that never came from a HEAD constructor is outside this rule,
	// so it stays silent; the rule explains itself only when it applied.
	if !headConstructed(common.Args[1]) {
		return resourceProof{}
	}
	if !headRequest(common.Args[1]) {
		return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonHeadRequestModified}
	}
	if !headClientUnconfigured(common.Args[0], call.Parent()) {
		return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonHeadClientNotUnconfigured}
	}
	return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonHeadAcquisition}
}

// headConstructed reports whether the request traces back, through rebinding
// calls only, to a HEAD constructor. It asks nothing about the uses in between.
func headConstructed(request ssa.Value) bool {
	switch typed := request.(type) {
	case *ssa.Extract:
		constructor, ok := typed.Tuple.(*ssa.Call)
		return ok && typed.Index == 0 && headConstructor(constructor.Common())
	case *ssa.Call:
		return ssaflow.CallMatchesAnySymbol(typed.Common(), httpRequestWithContext, httpRequestClone) &&
			headConstructed(ssaflow.CallReceiver(typed.Common()))
	}
	return false
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

func headClientUnconfigured(client ssa.Value, function *ssa.Function) bool {
	if local, ok := client.(*ssa.Alloc); ok {
		return onlyHTTPDoUses(local)
	}
	load, ok := client.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	if ssaflow.ValueMatchesSymbol(load.X, httpDefaultClient) {
		return onlyHTTPDoUses(load) && !defaultClientVisiblyModified(function)
	}
	cell, ok := load.X.(*ssa.Alloc)
	return ok && zeroClientCell(cell)
}

// zeroClientCell accepts a local variable that holds one fresh zero-value
// client and is only ever loaded for direct Do calls, here or inside a
// literal that captures it. A worker that shares the client can still
// change nothing about it. pmtiles keeps one client in a variable its
// download workers capture:
// https://github.com/protomaps/go-pmtiles/blob/a3e4951ea6a0477b784c27c1dcbfd9c130878c5a/pmtiles/sync.go#L73-L80
func zeroClientCell(cell *ssa.Alloc) bool {
	if cell.Referrers() == nil {
		return false
	}
	stored := false
	for _, use := range *cell.Referrers() {
		switch typed := use.(type) {
		case *ssa.DebugRef:
		case *ssa.Store:
			fresh, ok := typed.Val.(*ssa.Alloc)
			if stored || typed.Addr != cell || !ok || !onlyStoredInto(fresh, typed) {
				return false
			}
			stored = true
		case *ssa.UnOp:
			if typed.Op != token.MUL || !onlyHTTPDoUses(typed) {
				return false
			}
		case *ssa.MakeClosure:
			if !closureLoadsCellForDo(typed, cell) {
				return false
			}
		default:
			return false
		}
	}
	return stored
}

// onlyStoredInto reports whether the fresh allocation's only use is the store
// that puts it in the cell: no field was addressed, so it is zero-valued.
func onlyStoredInto(fresh *ssa.Alloc, store *ssa.Store) bool {
	if fresh.Referrers() == nil {
		return false
	}
	for _, use := range *fresh.Referrers() {
		if _, debug := use.(*ssa.DebugRef); !debug && use != store {
			return false
		}
	}
	return true
}

func closureLoadsCellForDo(closure *ssa.MakeClosure, cell *ssa.Alloc) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for index, binding := range closure.Bindings {
		if binding != cell || index >= len(function.FreeVars) {
			continue
		}
		free := function.FreeVars[index]
		if free.Referrers() == nil {
			return false
		}
		for _, use := range *free.Referrers() {
			load, loaded := use.(*ssa.UnOp)
			if _, debug := use.(*ssa.DebugRef); debug {
				continue
			}
			if !loaded || load.Op != token.MUL || !onlyHTTPDoUses(load) {
				return false
			}
		}
	}
	return true
}

// httpEffectsBudget bounds the walks that look for a transport or client
// override through visible callees. They visit every instruction of every
// reachable body once, so they are given twice a summary question; an
// exhausted walk counts as modified, which is the conservative answer.
const httpEffectsBudget = 4000

// defaultClientVisiblyModified reports any use of the package default client
// or transport, here or in a visible callee, other than loading the client
// for a direct Do call. A store, a field address, or an argument position
// could install the timeout or transport that gives a HEAD response a body
// wrapper. Exhausted searches count as modified.
func defaultClientVisiblyModified(function *ssa.Function) bool {
	budget := ssaflow.NewSearchBudget(httpEffectsBudget)
	overrides := newHTTPWriterEffects().overrides
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return true
			}
			if load, ok := instruction.(*ssa.UnOp); ok && load.Op == token.MUL &&
				ssaflow.ValueMatchesSymbol(load.X, httpDefaultClient) && onlyHTTPDoUses(load) {
				continue
			}
			for _, operand := range instruction.Operands(nil) {
				if operand != nil && ssaflow.ValueMatchesAnySymbol(*operand, httpDefaultClient, httpDefaultTransport) {
					return true
				}
			}
			callee, _ := ssaflow.DirectCallee(ssaflow.InstructionCall(instruction))
			if callee != nil && len(callee.Blocks) != 0 && overrides.Function(callee, budget) {
				return true
			}
		}
	}
	return false
}

// headRequest accepts the direct result of a HEAD constructor, or that result
// rebound through WithContext or Clone, when every use of each intermediate
// preserves Method.
func headRequest(request ssa.Value) bool {
	switch typed := request.(type) {
	case *ssa.Extract:
		constructor, ok := typed.Tuple.(*ssa.Call)
		return ok && typed.Index == 0 && headConstructor(constructor.Common()) && requestUsesPreserveMethod(typed)
	case *ssa.Call:
		return ssaflow.CallMatchesAnySymbol(typed.Common(), httpRequestWithContext, httpRequestClone) &&
			headRequest(ssaflow.CallReceiver(typed.Common())) && requestUsesPreserveMethod(typed)
	}
	return false
}

func headConstructor(common *ssa.CallCommon) bool {
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequest")) {
		return len(common.Args) == 3 && constantString(common.Args[0]) == "HEAD"
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "NewRequestWithContext")) {
		return len(common.Args) == 4 && constantString(common.Args[1]) == "HEAD"
	}
	return false
}

func requestUsesPreserveMethod(request ssa.Value) bool {
	refs := request.Referrers()
	if refs == nil || len(*refs) == 0 {
		return false
	}
	for _, ref := range *refs {
		switch typed := ref.(type) {
		case *ssa.DebugRef:
		case *ssa.Call:
			common := typed.Common()
			if ssaflow.CallMatchesSymbol(common, httpClientDo) && len(common.Args) == 2 && common.Args[1] == request {
				continue
			}
			if ssaflow.CallMatchesAnySymbol(common, httpRequestWithContext, httpRequestClone) &&
				ssaflow.CallReceiver(common) == request && requestUsesPreserveMethod(typed) {
				continue
			}
			return false
		case *ssa.FieldAddr:
			if requestFieldName(typed) != "Header" || !headerUsesAreEdits(typed) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func requestFieldName(field *ssa.FieldAddr) string {
	pointer, ok := field.X.Type().Underlying().(*types.Pointer)
	if !ok || !syntax.NamedType(field.X.Type(), "net/http", "Request") {
		return ""
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok {
		return ""
	}
	return structure.Field(field.Field).Name()
}

func headerUsesAreEdits(field *ssa.FieldAddr) bool {
	if field.Referrers() == nil {
		return false
	}
	for _, use := range *field.Referrers() {
		load, ok := use.(*ssa.UnOp)
		if !ok || load.Op != token.MUL || load.Referrers() == nil {
			return false
		}
		for _, edit := range *load.Referrers() {
			call, ok := edit.(*ssa.Call)
			if !ok || ssaflow.CallReceiver(call.Common()) != load || !ssaflow.CallMatchesAnySymbol(call.Common(), httpHeaderEdits...) {
				return false
			}
		}
	}
	return true
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

func constantString(value ssa.Value) string {
	text, ok := value.(*ssa.Const)
	if !ok || text.Value == nil || text.Value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(text.Value)
}
