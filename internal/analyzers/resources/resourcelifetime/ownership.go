package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Ownership evidence binds cleanup and publication to the caller's resource.
// Wrapper chains establish possible retention only at a foreign storage or
// retaining-call boundary; neither a wrapper's name nor its mere use transfers
// the obligation.

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !resourceExternalStorageProof(instruction, resource).Proven() && !heapmodel.MayAliasAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

// A helper can condition cleanup on an error it receives beside the
// resource. Unconditional completion cannot represent that relation, so a
// witnessed cleanup plus a correlated error is uncertainty, not proof of
// either release or a leak. The error is correlated when it is the one
// paired with this acquisition, or when the caller itself branches on it
// being nil after the call: then the caller's own paths split on the same
// value the helper's cleanup does, as in closeOnError(f, err) followed by
// if err != nil { return nil, err }; return f, nil.
// https://github.com/h44z/wg-portal/blob/eb44c8c4ff120f34c26b2415c47560f4fba0603c/internal/lowlevel/mikrotik.go#L267-L280
// Helpers that merely inspect the pair, receive an error the caller never
// tests again, or condition cleanup on a flag stay visible: a flag the
// caller does not branch on leaves the unreleased path feasible.
func (analysis *resourceAnalysis) pairedErrorHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || !slices.Contains(common.Args, analysis.resource) ||
		!slices.ContainsFunc(common.Args, func(argument ssa.Value) bool { return analysis.correlatedError(instruction, argument) }) {
		return false
	}
	return lifecycle.ProveCompletion(lifecycle.CompletionRequest{
		Instruction: instruction,
		Target:      analysis.resource,
		Methods:     analysis.contract.cleanup,
		Coverage:    lifecycle.CoverageAnywhere,
		Budget:      analysis.budget(releaseSearchBudget),
	}).Proven()
}

// correlatedError reports whether an error handed to the helper call is the
// acquisition's paired error, or one the caller compares with nil after the
// call. Identity, not derivation: a wrapped error is a different value.
func (analysis *resourceAnalysis) correlatedError(call ssa.Instruction, argument ssa.Value) bool {
	if !syntax.IsErrorType(argument.Type()) {
		return false
	}
	if analysis.resource == ssaflow.CallResult(analysis.acquisition, 0) && argument == ssaflow.CallResult(analysis.acquisition, 1) {
		return true
	}
	for _, instruction := range ssaflow.InstructionsReachableAfter(call) {
		branch, ok := instruction.(*ssa.If)
		if !ok {
			continue
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) &&
			(comparison.X == argument && ssaflow.DefinitelyNil(comparison.Y) || comparison.Y == argument && ssaflow.DefinitelyNil(comparison.X)) {
			return true
		}
	}
	return false
}

// An imported helper that releases every element of what it receives inside
// a loop exports that loop as a may-claim, as client-go's CloseAndRemove does
// for its variadic files; a visible helper's loop is found by the completion
// search itself. Either way the call is uncertain, never a release.
// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/staging/src/k8s.io/client-go/util/testing/remove_file.go#L25-L39
func (analysis *resourceAnalysis) importedLoopRelease(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) != 0 {
		return false
	}
	for index, argument := range common.Args {
		if released, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleasesInLoop); released &&
			analysis.carries(argument) {
			return true
		}
	}
	return false
}

// resourceExternalStorageProof distinguishes the destination from the local
// aggregate that holds its address. An unresolved pointer load consumes the
// resource opaquely; it is neither a local owner nor guaranteed cleanup.
// https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99
func resourceExternalStorageProof(instruction ssa.Instruction, resource ssa.Value) resourceProof {
	owner := resourceFieldOwner(instruction, resource)
	if owner == nil {
		return resourceProof{State: ssaflow.EvidenceDisproven}
	}
	if ssaflow.ExternallyOwnedValue(owner) {
		return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonSettled}
	}
	store, stored := instruction.(*ssa.Store)
	if !stored {
		return resourceProof{State: ssaflow.EvidenceDisproven}
	}
	load, indirect := store.Addr.(*ssa.UnOp)
	if !indirect || load.Op != token.MUL {
		return resourceProof{State: ssaflow.EvidenceDisproven}
	}
	object, known := heapmodel.ExclusiveAt(store.Addr, store)
	if !known {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonIndirectDestinationUnknown}
	}
	if object.Local {
		return resourceProof{State: ssaflow.EvidenceDisproven}
	}
	return resourceProof{State: ssaflow.EvidenceProven, Reason: resourceReasonSettled}
}

func resourceFieldOwner(instruction ssa.Instruction, resource ssa.Value) ssa.Value { //nolint:ireturn // Owners retain their concrete SSA value forms.
	store, ok := instruction.(*ssa.Store)
	if !ok || !heapmodel.ValueDerivesFrom(store.Val, resource) && !lifecycle.MayContainValue(store.Val, resource) {
		return nil
	}
	if field, ok := store.Addr.(*ssa.FieldAddr); ok {
		return field.X
	}
	// A store through a pointer the caller supplied, such as appending to the
	// slice a pointer receiver points at, lands in caller-owned storage.
	// rules_img collects output files through a flag value this way:
	// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/validate/layer-presence/flags.go#L83-L94
	return store.Addr
}

func resourceLifecycleMethod(name string) bool {
	switch name {
	case "Close", "Kill", "Shutdown", "Stop", "Wait":
		return true
	default:
		return false
	}
}

// wrapperStoredOnForeignOwner reports whether a store hands a constructor chain
// over the resource to an object this function did not allocate: a field or
// element of a parameter, a global, or an object a call returned. The resource
// then lives as long as that object, so its release is no longer this
// function's to prove. A logger routed into a server's error log is the
// common case. A chain stored into a local allocation stays owed here.
// https://github.com/1parado/grok-build-switch/blob/c1ee703bf6000abd92d8d29ca94a4ed59cb510f2/main.go#L173-L176
func (analysis *resourceAnalysis) wrapperStoredOnForeignOwner(store *ssa.Store) bool {
	if heapmodel.MayAlias(store.Val, analysis.resource) || !analysis.wrapsResource(store.Val, maxWrapperChain, true) {
		return false
	}
	root := store.Addr
	for {
		switch address := root.(type) {
		case *ssa.FieldAddr:
			root = address.X
		case *ssa.IndexAddr:
			root = address.X
		case *ssa.Alloc:
			return false
		default:
			return true
		}
	}
}

// keptThroughChain reports whether a callee proven to store its argument
// receives a chain of constructors that holds the resource. A logger over a
// file is the common case: slog.SetDefault stores the logger that keeps the
// handler that keeps the MultiWriter that keeps the file, so the file now
// belongs to the process. Unlike a single wrapper, a chain counts only at a
// callee proven to store it: a method call on the logger with unknown effects,
// such as Info, does not publish the file, and a chain the function merely
// uses still leaves it owed.
func (analysis *resourceAnalysis) keptThroughChain(instruction ssa.Instruction, index int, argument ssa.Value) bool {
	if !analysis.wrapsResource(argument, maxWrapperChain, false) {
		return false
	}
	// Only the strict stored claim proves the callee keeps the argument; the
	// may-retain claim and retain effects also cover an opaque interface
	// call, which Logger.Info makes with the handler it loads.
	stored, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimStores)
	return stored
}

// maxWrapperChain bounds how many constructors a wrapper chain may nest.
const maxWrapperChain = 4

// wrapsResource reports whether value is a call result that may keep the
// resource: an argument holds it inside an aggregate, or, below the outermost
// call when depth allows a chain, the argument is the resource itself or
// another such wrapper. Every step needs its own callee to keep the argument,
// or to be unknown. A wrapper may receive the resource inside a variadic
// aggregate instead of as a direct operand. This is only an
// opaque-consumption query; it never establishes exact identity or that the
// wrapper owns cleanup.
// https://github.com/Mmx233/BitSrunLoginGo/blob/a744f312b3835f329eb98e45c8d19bc2a5b7d4c0/internal/config/log.go#L58-L59
// https://github.com/inkdust2021/VibeGuard/blob/12a46784a7ebca95f7765178edd8344a974da849/internal/log/log.go#L42-L47
func (analysis *resourceAnalysis) wrapsResource(value ssa.Value, depth int, direct bool) bool {
	call, ok := unwrapWrapper(value).(*ssa.Call)
	if !ok {
		return false
	}
	if _, scalar := call.Type().Underlying().(*types.Basic); scalar || syntax.IsErrorType(call.Type()) {
		return false
	}
	// Append's explicit values live in a compiler-built variadic array. Follow
	// their wrapper chains when this slice is published, without treating an
	// append to a discarded local collection as a handoff. Spread slices remain
	// outside this bounded query.
	// https://github.com/twmb/kcl/blob/5290cb05bcc421a239e327ba11408bc4e27bd2dd/client/client.go#L1445-L1456
	if depth > 0 {
		if values, explicit := ssaflow.AppendedValues(call); explicit && slices.ContainsFunc(values, func(value ssa.Value) bool {
			return analysis.wrapsResource(value, depth-1, true)
		}) {
			return true
		}
	}
	for _, argument := range call.Common().Args {
		exact := heapmodel.MayAlias(argument, analysis.resource)
		held := !exact && analysis.carriesWithin(argument) || direct && exact ||
			depth > 0 && analysis.wrapsResource(argument, depth-1, true)
		if !held {
			continue
		}
		// A visible transformation that does not retain its input is not a
		// wrapper. Missing effects are unknown, never a purity claim.
		effects := analysis.evidence.CallEffects(call, argument)
		if !effects.Proven() || effects.Effects&ssaflow.EffectRetain != 0 {
			return true
		}
	}
	return false
}

// unwrapWrapper peels the interface conversions a wrapper passes through on
// its way to the next constructor, such as a handler boxed as slog.Handler.
func unwrapWrapper(value ssa.Value) ssa.Value {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentMakeInterface
	for {
		inner, ok := ssaflow.UnwrapTransparentValue(value, forms)
		if !ok {
			return value
		}
		value = inner
	}
}
