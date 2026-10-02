package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The classifier labels each instruction after an acquisition once, and the
// flow asks one question of the labels: does a release or transfer cover
// every feasible return, and if not, did anything the analysis cannot see
// through consume the resource on that path? A diagnostic needs the second
// answer to be no. Summaries are the model's knowledge: a summarized callee
// that neither releases, stores, nor owns the resource is transparent, so a
// read helper leaves the obligation in place. Unknown is reserved for real
// boundaries: an interface method or function value that receives the
// resource here, a callee with neither body nor summary, a literal capturing
// the resource that is launched or deferred without a proven release, and a
// send or append that hands it to storage the analysis does not track.

type resourceAction uint8

const (
	actionNone resourceAction = iota
	// actionSettled covers both a proven release and a proven transfer; the
	// flow treats them alike and the evidence trace records which one held.
	actionSettled
	actionUnknown
)

// resourceAnalysis holds one acquisition's inputs and its memoized labels.
type resourceAnalysis struct {
	acquisition *ssa.Call
	pass        *analysis.Pass
	evidence    *lifecyclefacts.LifecycleEvidence
	summaries   *summaries.Provider
	function    *ssa.Function
	resource    ssa.Value
	// candidate identifies the acquisition every step of this proof serves, and
	// probe tags every trace event with it so one acquisition's proof can be
	// read without the interleaved steps of the others in the same function.
	candidate token.Pos
	probe     analysisTrace.Probe
	owners    []ssa.Value
	// collection is the local slice the resource was appended to, when every
	// use of it is understood; see local_collections.go.
	collection *localCollection
	// guardedDefers are the deferred literals whose release turns on a named
	// result; see result_guarded_defers.go.
	guardedDefers []lifecycle.ResultGuard
	contract      resourceContract
	optional      optionalAcquisitionProof
	actions       map[ssa.Instruction]resourceAction
	// pool is this acquisition's total across every query its proof asks;
	// see budget.
	pool *ssaflow.SearchBudget
	// stores reuses one destination proof between release and opacity labels.
	stores map[*ssa.Store]resourceStorageProof
	// wrappers shares return classification evidence with owner disposition.
	wrappers map[*ssa.Return]resourceReturnedWrapperProof
	// leak is the return at which the flow walk found the resource owed.
	leak *ssa.Return
}

// resourcePoolBudget bounds the pooled queries of one acquisition proof. The largest single
// question, a release search, may itself cost releaseSearchBudget, so the
// pool allows four of them; a proof that needs more is a pathological
// candidate, and an exhausted pool is unknown exactly as an exhausted
// search is.
const resourcePoolBudget = 4 * releaseSearchBudget

// budget draws one shared query's allowance from this candidate's pool:
// give-ups inside it reach the probe, so a trace of the acquisition shows
// where shared storage, summary, or completion evidence ran out, and these
// queries stay bounded together.
func (analysis *resourceAnalysis) budget(limit int) *ssaflow.SearchBudget {
	if analysis.pool == nil {
		analysis.pool = ssaflow.NewSearchBudget(resourcePoolBudget).Observed(analysis.probe.Observer())
	}
	return analysis.pool.Within(limit)
}

func (analysis *resourceAnalysis) action(instruction ssa.Instruction) resourceAction {
	if action, ok := analysis.actions[instruction]; ok {
		return action
	}
	action, reason := analysis.classify(instruction)
	analysis.actions[instruction] = action
	analysis.emitAction(instruction, action, reason)
	return action
}

// classify labels the instruction and names why. The reason is the label for
// a settled or untouched resource, and for an opaque one it is the boundary
// that stopped the proof, so a reader can tell an interface call from a
// callee with no body without rereading this code.
func (analysis *resourceAnalysis) classify(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason) {
	if action, reason, ok := analysis.collection.label(instruction); ok {
		return action, reason
	}
	if action, reason, ok := analysis.resultGuardedLabel(instruction); ok {
		return action, reason
	}
	if analysis.compressionOutputAbandoned(instruction) {
		return actionUnknown, resourceReasonCompressionOutputMayBeAbandoned
	}
	if closesStatementDatabase(analysis.acquisition, instruction) {
		return actionUnknown, resourceReasonStatementParentClosed
	}
	if finishesRowsTransaction(analysis.acquisition, instruction) {
		return actionUnknown, resourceReasonRowsTransactionFinished
	}
	if cancelsTransactionContext(analysis.acquisition, instruction) {
		return actionUnknown, resourceReasonTransactionContextCanceled
	}
	if action, reason := analysis.releasesResource(instruction); action != actionNone {
		return action, reason
	}
	// A merged receiver or an escaped owner projection may still select this
	// acquisition. Exact storage identity cannot establish that relationship,
	// but absence of a match is not proof that the resource stays open. A body
	// read helper can consume a response before its explicit Body.Close:
	// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/claude_api_key.go#L94-L99
	common := ssaflow.InstructionCall(instruction)
	if !analysis.optional.Proven() && common != nil && slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) &&
		heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), analysis.resource) {
		return actionUnknown, resourceReasonAmbiguousCleanupValue
	}
	if analysis.ambiguousHelperCleanup(instruction, common) {
		return actionUnknown, resourceReasonAmbiguousHelperCleanupValue
	}
	if analysis.pairedErrorHelperCleanup(instruction, common) {
		return actionUnknown, resourceReasonPairedErrorHelperCleanup
	}
	loopRelease := analysis.proveImportedLoopReleaseWithin(instruction, common, analysis.budget(ssaflow.SummaryBudget))
	if loopRelease.State != ssaflow.EvidenceDisproven {
		return actionUnknown, loopRelease.Reason
	}
	if boundary, opaque := analysis.opaqueConsumption(instruction); opaque {
		return actionUnknown, boundary
	}
	return actionNone, resourceReasonUntouched
}

// releaseLabel preserves the distinction between cleanup and an abandoned
// search. Both suppress a leak, but only positive completion evidence settles
// the obligation. A loop-only witness is likewise uncertain; conditional
// helpers with complete path evidence retain the ordinary classification.
func releaseLabel(proof lifecyclefacts.Proof) (resourceAction, resourceLifetimeReason) {
	if proof.Proven() {
		return actionSettled, resourceReasonSettled
	}
	switch proof.Reason {
	case ssaflow.EvidenceBudgetExhausted:
		return actionUnknown, resourceReasonBudgetExhausted
	case ssaflow.EvidenceCompletionInCycle:
		return actionUnknown, resourceReasonHelperCleanupInLoop
	default:
		return actionNone, resourceReasonNone
	}
}

// A cleanup helper may receive a projection of a merged owner. Proving cleanup
// of its actual argument does not prove which acquisition was released, but
// that ambiguous identity cannot establish a leak either. Direct Close calls
// have the same unknown boundary above; read-only helpers do not qualify.
// https://github.com/mr-karan/doggo/blob/7f6b105f240e562a5c7659976b1c16b683c25385/pkg/resolvers/doh.go#L143-L168
func (analysis *resourceAnalysis) ambiguousHelperCleanup(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if analysis.optional.Proven() || common == nil {
		return false
	}
	for _, argument := range common.Args {
		if !mergedCleanupArgument(argument) || !heapmodel.ValueDerivesFrom(argument, analysis.resource) {
			continue
		}
		for _, method := range analysis.contract.cleanup {
			completion := lifecycle.CompletionRequest{
				Instruction: instruction,
				Target:      argument,
				Methods:     []string{method},
				Coverage:    lifecycle.CoverageEveryReturn,
				Budget:      analysis.budget(releaseSearchBudget),
			}
			if analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{
				Instruction: instruction,
				Target:      argument,
				Completion:  &completion,
				SelectMask:  releaseMask(instruction, argument, method),
			}).Proven() {
				return true
			}
		}
	}
	return false
}

// Only merged values are ambiguous here. A helper that closes an overwritten
// field, or conditionally closes its exact argument, must remain diagnostic.
func mergedCleanupArgument(argument ssa.Value) bool {
	if _, merged := argument.(*ssa.Phi); merged {
		return true
	}
	load, loaded := argument.(*ssa.UnOp)
	if !loaded || load.Op != token.MUL {
		return false
	}
	field, projected := load.X.(*ssa.FieldAddr)
	if !projected {
		return false
	}
	_, merged := field.X.(*ssa.Phi)
	return merged
}

// Compressors own finalization, not their output descriptor. An error return
// may abandon that output rather than publish it; this local proof cannot
// decide the caller's policy. Successful returns still owe finalization.
// https://github.com/goreleaser/nfpm/blob/3627b6a6466c0ae3bbe17fe7b98710c36e750907/rpm/srpm.go#L123
func (analysis *resourceAnalysis) compressionOutputAbandoned(instruction ssa.Instruction) bool {
	if analysis.contract.family != "compress" {
		return false
	}
	if returned, ok := instruction.(*ssa.Return); ok {
		for _, result := range returned.Results {
			if types.Identical(result.Type(), types.Universe.Lookup("error").Type()) && !ssaflow.DefinitelyNil(result) {
				return true
			}
		}
	}
	common := ssaflow.InstructionCall(instruction)
	return ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "io", Receiver: "PipeWriter", Name: "CloseWithError",
	})) && len(common.Args) == 2 && !ssaflow.DefinitelyNil(common.Args[1]) &&
		// Possible identity is sufficient for uncertainty, including repeated
		// loads of a captured pipe across a wait. This never proves release.
		heapmodel.MayAlias(ssaflow.CallReceiver(common), analysis.acquisition.Common().Args[0])
}

// opaqueConsumption reports whether the instruction hands the resource to
// something the analysis cannot see through.
func (analysis *resourceAnalysis) opaqueConsumption(instruction ssa.Instruction) (resourceLifetimeReason, bool) {
	switch typed := instruction.(type) {
	case *ssa.Return:
		proof := analysis.returnedWrapperWithin(typed, analysis.budget(ssaflow.SummaryBudget))
		return proof.Reason, proof.State != ssaflow.EvidenceDisproven
	case *ssa.Store:
		proof := analysis.resourceStorage(typed)
		if proof.State == ssaflow.EvidenceUnknown {
			return proof.Reason, true
		}
		// An owner selected from a collection may already be retained elsewhere.
		// The local collection is not evidence that its elements are local owners.
		// https://github.com/cloudflare/artifact-fs/blob/2b87a48691ef4ae82d391b7bbe4976c06c7fadf7/internal/fusefs/fuse_unix.go#L256-L287
		owner := proof.Owner
		_, field := typed.Addr.(*ssa.FieldAddr)
		if field && owner != nil && ssaflow.ElementOfAggregate(owner) {
			return resourceReasonStoredOnCollectionOwner, true
		}
		wrapper := analysis.proveWrapperStoredOnForeignOwnerWithin(typed, analysis.budget(ssaflow.SummaryBudget))
		return wrapper.Reason, wrapper.State != ssaflow.EvidenceDisproven
	case *ssa.Send:
		if proof := analysis.responseBodyAggregateHandoff(typed.X, typed); proof.State == ssaflow.EvidenceUnknown {
			return proof.Reason, true
		}
		return analysis.carriedPayload(typed.X, resourceReasonSentToChannel)
	case *ssa.MapUpdate:
		return analysis.carriedPayload(typed.Value, resourceReasonStoredInMap)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Send != nil {
				if proof := analysis.responseBodyAggregateHandoff(state.Send, typed); proof.State == ssaflow.EvidenceUnknown {
					return proof.Reason, true
				}
			}
			if state.Send != nil {
				if reason, opaque := analysis.carriedPayload(state.Send, resourceReasonSentToChannel); opaque {
					return reason, true
				}
			}
		}
		return resourceReasonNone, false
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.opaqueCall(instruction, ssaflow.InstructionCall(instruction))
	}
	return resourceReasonNone, false
}

func (analysis *resourceAnalysis) opaqueCall(instruction ssa.Instruction, common *ssa.CallCommon) (resourceLifetimeReason, bool) {
	if common == nil {
		return resourceReasonNone, false
	}
	carriedProof := analysis.proveCarriedArgumentsWithin(common, analysis.budget(ssaflow.SummaryBudget))
	if carriedProof.State == ssaflow.EvidenceUnknown {
		return carriedProof.Reason, true
	}
	carried := carriedProof.Proven()
	callback := analysis.provePossiblyRetainedCallbackWithin(instruction, common, analysis.budget(ssaflow.SummaryBudget))
	if callback.State != ssaflow.EvidenceDisproven {
		return callback.Reason, true
	}
	// A helper may expose the exact resource asynchronously without itself
	// being launched. That is opaque ownership, not proven cleanup. Conversely,
	// preserving an address does not prove that a resource loaded through it
	// stays owned here: the lifecycle-specific rules below still decide that.
	if carried && analysis.asynchronouslyExposesResource(instruction, common) {
		return resourceReasonCallEffectsAsynchronousExposure, true
	}
	if common.IsInvoke() {
		// The receiver of an interface method is not consumed by being the
		// receiver; only the resource handed to the method is. The body behind
		// an interface method is chosen at run time, so no summary describes it.
		return resourceReasonInterfaceMethod, carried
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return resourceReasonAppended, builtin.Name() == "append" && carried
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		return analysis.opaqueClosureCall(instruction, closure, carried)
	}
	return analysis.opaqueFunctionCall(instruction, common, carried)
}

// Imported heap summaries retain the same positive asynchronous exposure as
// local call effects. A may-exposure makes ownership unknown; synchronous
// borrowing remains transparent. No missing claim proves an absence of escape.
// https://github.com/golang/debug/blob/ac862fd6552b739f50ba812382eed75745a129b1/cmd/viewcore/main.go#L820-L829
func (analysis *resourceAnalysis) asynchronouslyExposesResource(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	for index, argument := range common.Args {
		if !heapmodel.ValueDerivesFrom(argument, analysis.resource) {
			continue
		}
		if exposed, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimAsynchronouslyExposes); exposed {
			return true
		}
	}
	effects := analysis.evidence.CallEffects(instruction, analysis.resource)
	return effects.Proven() && effects.Effects&ssaflow.EffectAsync != 0
}

// Named and dynamically selected functions share the argument-level boundary;
// literal captures and interface receivers are classified by opaqueCall first.
func (analysis *resourceAnalysis) opaqueFunctionCall(instruction ssa.Instruction, common *ssa.CallCommon, carried bool) (resourceLifetimeReason, bool) {
	callee := common.StaticCallee()
	if callee == nil {
		// A function value: the callee is decided at run time.
		return resourceReasonDynamicCallee, carried
	}
	// A callee proven to store a constructor chain over the resource takes it,
	// whether the chain derives from the resource directly or holds it inside
	// an aggregate; see proveChainKeptWithin.
	chain := analysis.proveChainKeptWithin(instruction, common, analysis.budget(ssaflow.SummaryBudget))
	if chain.State != ssaflow.EvidenceDisproven {
		return chain.Reason, true
	}
	if !carried {
		return resourceReasonNone, false
	}
	// An aggregate owner passed to an incompletely modeled retaining helper
	// can outlive this call even when the helper returns nothing. A read-only
	// helper is not an ownership handoff and must keep the obligation live.
	// https://github.com/goshs-labs/goshs/blob/c65ca19696e87cd5ec2206b2a488c5f5f5b621db/smbserver/session.go#L135-L137
	escape := analysis.proveAggregateOwnerEscapeWithin(instruction, common, analysis.budget(ssaflow.SummaryBudget))
	if escape.State != ssaflow.EvidenceDisproven {
		return escape.Reason, true
	}
	// A resource that reaches the callee only inside an aggregate argument is
	// beyond a parameter-level completion proof: that proof follows the
	// parameter value, not a resource nested in one of its fields. Treat such a
	// call as a boundary only when its own result reaches a return or global, because
	// only then can the resource have been transferred to the value the caller
	// receives; a call whose result is discarded transfers nothing, so a
	// resource left behind in its argument is still leaked. oss-rebuild wraps a
	// zip reader in an fs.FS wrapper and returns the loader's result:
	// https://github.com/google/oss-rebuild/blob/9ce0528dd68bf209b52cc9fdc90bd63742cbb3a0/pkg/sysgraph/sgstorage/loader.go#L173-L179
	aggregate := analysis.proveCarriedAggregateArgumentsWithin(common, analysis.budget(ssaflow.SummaryBudget))
	if aggregate.State == ssaflow.EvidenceUnknown {
		return aggregate.Reason, true
	}
	if aggregate.Proven() && callResultMayTransfer(instruction) {
		return resourceReasonNestedInTransferredArgument, true
	}
	// A summarized callee proven to release, store, or own the resource was
	// classified as settled above; one summarized as doing none of those is
	// transparent. A callee with a body but no summary is judged by its body
	// through the completion proof already consulted; without either, the
	// callee is a boundary.
	return resourceReasonUnsummarizedCallee, !analysis.evidence.CalleeSummarized(instruction) && len(callee.Blocks) == 0
}

// callResultMayTransfer reports whether a non-error call result flows to a return
// or a global aggregate. A fluent builder may publish its nested resource without
// returning it from this function. This is uncertainty, not proof of ownership:
// https://github.com/tair-opensource/RedisShake/blob/014e2f493583d24d2a37166d360bad21fc2a2422/internal/log/init.go#L54-L60
func callResultMayTransfer(instruction ssa.Instruction) bool {
	result, ok := instruction.(ssa.Value)
	if !ok || instruction.Parent() == nil {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](instruction.Parent()) {
		for _, value := range returned.Results {
			if types.Identical(value.Type(), errorType) {
				continue
			}
			if heapmodel.ValueDerivesFrom(value, result) {
				return true
			}
		}
	}
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](instruction.Parent()) {
		if _, global := store.Addr.(*ssa.Global); !global {
			continue
		}
		// Publishing a scalar observation or error does not retain its inputs.
		_, scalar := store.Val.Type().Underlying().(*types.Basic)
		if !scalar && !types.Identical(store.Val.Type(), errorType) &&
			heapmodel.ValueDerivesFrom(store.Val, result) {
			return true
		}
	}
	return false
}

func (analysis *resourceAnalysis) capturesAggregateOwner(closure *ssa.MakeClosure) bool {
	for _, owner := range analysis.owners {
		pointer, ok := owner.Type().Underlying().(*types.Pointer)
		if !ok || heapmodel.MayAlias(owner, analysis.resource) {
			continue
		}
		if syntax.PointerStruct(pointer) == nil {
			continue
		}
		for _, binding := range closure.Bindings {
			if heapmodel.CapturedBindingMatches(binding, owner) {
				return true
			}
		}
	}
	return false
}
