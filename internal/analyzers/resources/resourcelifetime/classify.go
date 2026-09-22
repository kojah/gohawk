package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
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

func (action resourceAction) String() string {
	switch action {
	case actionSettled:
		return "settled"
	case actionUnknown:
		return "opaque-use"
	case actionNone:
	}
	return "none"
}

// resourceAnalysis holds one acquisition's inputs and its memoized labels.
type resourceAnalysis struct {
	acquisition *ssa.Call
	pass        *analysis.Pass
	evidence    *lifecyclefacts.LifecycleEvidence
	function    *ssa.Function
	resource    ssa.Value
	// candidate identifies the acquisition every step of this proof serves, and
	// probe tags every trace event with it so one acquisition's proof can be
	// read without the interleaved steps of the others in the same function.
	candidate token.Pos
	probe     analysisTrace.Probe
	owners    []ssa.Value
	contract  resourceContract
	optional  optionalAcquisitionProof
	actions   map[ssa.Instruction]resourceAction
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
func (analysis *resourceAnalysis) classify(instruction ssa.Instruction) (resourceAction, string) {
	if analysis.compressionOutputAbandoned(instruction) {
		return actionUnknown, "compression-output-may-be-abandoned"
	}
	if closesStatementDatabase(analysis.acquisition, instruction) {
		return actionUnknown, "statement-parent-closed"
	}
	if finishesRowsTransaction(analysis.acquisition, instruction) {
		return actionUnknown, "rows-transaction-finished"
	}
	if releasesResource(analysis.evidence, instruction, analysis.resource, analysis.owners, analysis.contract.cleanup, analysis.optional) {
		return actionSettled, actionSettled.String()
	}
	// A merged receiver or an escaped owner projection may still select this
	// acquisition. Exact storage identity cannot establish that relationship,
	// but absence of a match is not proof that the resource stays open. A body
	// read helper can consume a response before its explicit Body.Close:
	// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/claude_api_key.go#L94-L99
	common := ssaflow.InstructionCall(instruction)
	if !analysis.optional.Proven() && common != nil && slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) &&
		ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), analysis.resource, map[ssa.Value]bool{}) {
		return actionUnknown, "ambiguous-cleanup-value"
	}
	if analysis.ambiguousHelperCleanup(instruction, common) {
		return actionUnknown, "ambiguous-helper-cleanup-value"
	}
	if analysis.pairedErrorHelperCleanup(instruction, common) {
		return actionUnknown, "paired-error-helper-cleanup"
	}
	if boundary, opaque := analysis.opaqueConsumption(instruction); opaque {
		return actionUnknown, boundary
	}
	return actionNone, actionNone.String()
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
		if !mergedCleanupArgument(argument) || !ssaflow.ValueDerivesFrom(argument, analysis.resource, map[ssa.Value]bool{}) {
			continue
		}
		for _, method := range analysis.contract.cleanup {
			completion := ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      argument,
				Methods:     []string{method},
				Coverage:    ssaflow.CoverageEveryReturn,
				Budget:      ssaflow.NewSearchBudget(releaseSearchBudget),
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
		ssaflow.SameValue(ssaflow.CallReceiver(common), analysis.acquisition.Common().Args[0])
}

// opaqueConsumption reports whether the instruction hands the resource to
// something the analysis cannot see through.
func (analysis *resourceAnalysis) opaqueConsumption(instruction ssa.Instruction) (string, bool) {
	switch typed := instruction.(type) {
	case *ssa.Store:
		// An owner selected from a collection may already be retained elsewhere.
		// The local collection is not evidence that its elements are local owners.
		// https://github.com/cloudflare/artifact-fs/blob/2b87a48691ef4ae82d391b7bbe4976c06c7fadf7/internal/fusefs/fuse_unix.go#L256-L287
		owner := resourceFieldOwner(typed, analysis.resource)
		_, field := typed.Addr.(*ssa.FieldAddr)
		return "stored-on-collection-owner", field && owner != nil && ssaflow.ElementOfAggregate(owner)
	case *ssa.Send:
		return "sent-to-channel", analysis.carries(typed.X)
	case *ssa.MapUpdate:
		return "stored-in-map", analysis.carries(typed.Value)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Send != nil && analysis.carries(state.Send) {
				return "sent-to-channel", true
			}
		}
		return "", false
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.opaqueCall(instruction, ssaflow.InstructionCall(instruction))
	}
	return "", false
}

func (analysis *resourceAnalysis) opaqueCall(instruction ssa.Instruction, common *ssa.CallCommon) (string, bool) {
	if common == nil {
		return "", false
	}
	carried := slices.ContainsFunc(common.Args, analysis.carries)
	if analysis.possiblyRetainedCallback(instruction, common) {
		return "captured-by-possibly-retained-callback", true
	}
	// A helper may expose the exact resource asynchronously without itself
	// being launched. That is opaque ownership, not proven cleanup. Conversely,
	// preserving an address does not prove that a resource loaded through it
	// stays owned here: the lifecycle-specific rules below still decide that.
	if carried {
		effects := analysis.evidence.CallEffects(instruction, analysis.resource)
		if effects.Proven() && effects.Effects&ssaflow.EffectAsync != 0 {
			return "call-effects-asynchronous-exposure", true
		}
	}
	if common.IsInvoke() {
		// The receiver of an interface method is not consumed by being the
		// receiver; only the resource handed to the method is. The body behind
		// an interface method is chosen at run time, so no summary describes it.
		return "interface-method", carried
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return "appended", builtin.Name() == "append" && carried
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		// A captured aggregate can be populated after closure creation. The
		// closure observes its fields when called, so an unresolved cleanup of
		// that owner is unknown rather than proof that the acquisition leaks.
		// https://github.com/Autumn-27/ARTEX/blob/bf7f414477832b77d2152539c0723dc691086522/traffic/traffic.go#L1129-L1176
		if analysis.capturesAggregateOwner(closure) {
			return "captured-aggregate-owner", true
		}
		if !carried && !analysis.closureCarries(closure) {
			return "", false
		}
		// A started literal runs on another goroutine, so a release inside it
		// cannot be ordered against this function's returns and the resource
		// is beyond what this flow can judge.
		if _, started := instruction.(*ssa.Go); started {
			return "captured-by-started-literal", true
		}
		// A called or deferred literal runs in this frame, so its body is as
		// readable as a named callee's. A release inside it was already proved
		// before this point, so what is left to ask is whether it keeps the
		// resource: if it does the obligation moved, and if it does not the
		// literal is transparent and this function still owns the resource.
		//
		// That holds only for a body this pass can judge. A capture is a cell
		// the body loads first, so the argument at a call inside the literal is
		// the load rather than the acquired value, and a summary matched on
		// exact identity does not recognize it. block/spirit closes rows
		// through a helper in another package from inside a deferred literal,
		// and the release went uncredited while the literal was called
		// transparent:
		// https://github.com/block/spirit/blob/c554eae/pkg/checksum/single.go#L493-L503
		if analysis.evidence.ClosureHandsValueToUnreadableCallee(closure, analysis.resource) {
			return "captured-by-literal-calling-unreadable-callee", true
		}
		return "captured-by-retaining-literal", analysis.evidence.ClosureRetainsValue(closure, analysis.resource)
	}
	return analysis.opaqueFunctionCall(instruction, common, carried)
}

// Named and dynamically selected functions share the argument-level boundary;
// literal captures and interface receivers are classified by opaqueCall first.
func (analysis *resourceAnalysis) opaqueFunctionCall(instruction ssa.Instruction, common *ssa.CallCommon, carried bool) (string, bool) {
	callee := common.StaticCallee()
	if callee == nil {
		// A function value: the callee is decided at run time.
		return "dynamic-callee", carried
	}
	if !carried {
		return "", false
	}
	// An aggregate owner passed to an incompletely modeled retaining helper
	// can outlive this call even when the helper returns nothing. A read-only
	// helper is not an ownership handoff and must keep the obligation live.
	// https://github.com/goshs-labs/goshs/blob/c65ca19696e87cd5ec2206b2a488c5f5f5b621db/smbserver/session.go#L135-L137
	if analysis.aggregateOwnerMayEscape(instruction, common) {
		return "aggregate-owner-may-escape", true
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
	if analysis.carriedWithinAggregate(common) && callResultMayTransfer(instruction) {
		return "nested-in-transferred-argument", true
	}
	// A summarized callee proven to release, store, or own the resource was
	// classified as settled above; one summarized as doing none of those is
	// transparent. A callee with a body but no summary is judged by its body
	// through the completion proof already consulted; without either, the
	// callee is a boundary.
	return "unsummarized-callee", !analysis.evidence.CalleeSummarized(instruction) && len(callee.Blocks) == 0
}

func (analysis *resourceAnalysis) aggregateOwnerMayEscape(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	for index, argument := range common.Args {
		if ssaflow.SameValue(argument, analysis.resource) || analysis.carriedWithinClosure(argument) ||
			(!analysis.carriesWithin(argument) && !analysis.possibleAggregateWrapper(argument)) {
			continue
		}
		// A parameter-level retention fact also makes its nested contents
		// uncertain. Variadic values stored for later callbacks are a common
		// example; a wrapper around that aggregate can retain it too.
		// A clear retention bit is not a purity proof.
		// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/convert/filecopy_test.go#L92-L106
		// https://github.com/Mmx233/BitSrunLoginGo/blob/a744f312b3835f329eb98e45c8d19bc2a5b7d4c0/internal/config/log.go#L50-L60
		if retained, _ := analysis.evidence.ArgumentRetained(instruction, index); retained {
			return true
		}
		pointer, ok := argument.Type().Underlying().(*types.Pointer)
		if !ok {
			continue
		}
		if _, aggregate := pointer.Elem().Underlying().(*types.Struct); !aggregate {
			continue
		}
		effects := analysis.evidence.CallEffects(instruction, argument)
		if !effects.Proven() || effects.Effects&(ssaflow.EffectRetain|ssaflow.EffectAsync) != 0 {
			return true
		}
	}
	return false
}

// Retaining a callback also retains its captured resource. A known test
// cleanup registration has its own coverage proof; a visible observer that
// neither invokes nor retains the callback is not an ownership boundary.
func (analysis *resourceAnalysis) possiblyRetainedCallback(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return false
	}
	for index, argument := range common.Args {
		if !analysis.carriedWithinClosure(argument) {
			continue
		}
		retained, known := analysis.evidence.ArgumentRetained(instruction, index)
		callee := common.StaticCallee()
		if retained || !known && (callee == nil || len(callee.Blocks) == 0) {
			return true
		}
	}
	return false
}

// carriedWithinAggregate finds a separate owner argument even when another
// argument directly borrows the resource. A reader plus a variadic closer list
// can return ownership through the list without the reader parameter owning it.
func (analysis *resourceAnalysis) carriedWithinAggregate(common *ssa.CallCommon) bool {
	within := false
	for _, argument := range common.Args {
		if ssaflow.SameValue(argument, analysis.resource) {
			continue
		}
		// A closure that captures the resource is not a struct aggregate; the
		// launch and closure analyses already decide its fate, so this rule
		// must not intercept it.
		if analysis.carriedWithinClosure(argument) {
			continue
		}
		within = within || analysis.carriesWithin(argument)
	}
	return within
}

// carriedWithinClosure reports whether the argument is a closure value that
// captures the resource.
func (analysis *resourceAnalysis) carriedWithinClosure(argument ssa.Value) bool {
	value := argument
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	if inner, ok := ssaflow.UnwrapTransparentValue(value, forms); ok {
		value = inner
	}
	closure, ok := value.(*ssa.MakeClosure)
	return ok && analysis.closureCarries(closure)
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
			if ssaflow.ValueDerivesFrom(value, result, map[ssa.Value]bool{}) {
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
			ssaflow.ValueDerivesFrom(store.Val, result, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

// carries reports whether value is the resource, derives from it, or is an
// aggregate holding it or a projection of it. A struct literal wrapping a
// type-asserted response body and handed to a function value is such an
// aggregate; kandev upgrades an SPDY response this way:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/kubernetes/portforward.go#L464-L491
func (analysis *resourceAnalysis) carries(value ssa.Value) bool {
	return analysis.carriesDirectly(value) || analysis.carriesWithin(value) || analysis.possibleAggregateWrapper(value)
}

func (analysis *resourceAnalysis) possibleAggregateWrapper(value ssa.Value) bool {
	// A wrapper may receive the resource inside a variadic aggregate instead
	// of as a direct operand. Keep that possible containment when its result
	// is handed to another callee. This is only an opaque-consumption query;
	// it never establishes exact identity or that the wrapper owns cleanup.
	// https://github.com/Mmx233/BitSrunLoginGo/blob/a744f312b3835f329eb98e45c8d19bc2a5b7d4c0/internal/config/log.go#L58-L59
	call, ok := value.(*ssa.Call)
	if !ok {
		return false
	}
	if _, scalar := value.Type().Underlying().(*types.Basic); scalar || syntax.IsErrorType(value.Type()) {
		return false
	}
	for _, argument := range call.Common().Args {
		if ssaflow.SameValue(argument, analysis.resource) || !analysis.carriesWithin(argument) {
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

// carriesDirectly reports whether value is the resource itself or is produced
// from it by a transparent value step, so a callee receives the resource as an
// argument in its own right.
func (analysis *resourceAnalysis) carriesDirectly(value ssa.Value) bool {
	return ssaflow.SameValue(value, analysis.resource) ||
		ssaflow.ValueDerivesFrom(value, analysis.resource, map[ssa.Value]bool{})
}

// carriesWithin reports whether value is an aggregate that holds the resource
// in one of its fields, so a callee receives the resource only nested inside a
// parameter.
func (analysis *resourceAnalysis) carriesWithin(value ssa.Value) bool {
	if ssaflow.ValueContainsValue(value, analysis.resource) {
		return true
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return ssaflow.NewReachingWalk(forms).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		if _, ok := value.(*ssa.Alloc); !ok {
			return false
		}
		for stored := range ssaflow.StoredInto(value) {
			if ssaflow.ValueDerivesFrom(stored, analysis.resource, map[ssa.Value]bool{}) {
				return true
			}
		}
		return false
	})
}

func (analysis *resourceAnalysis) closureCarries(closure *ssa.MakeClosure) bool {
	for _, binding := range closure.Bindings {
		if ssaflow.CapturedBindingMatches(binding, analysis.resource) || analysis.carries(binding) {
			return true
		}
	}
	return false
}

func (analysis *resourceAnalysis) capturesAggregateOwner(closure *ssa.MakeClosure) bool {
	for _, owner := range analysis.owners {
		pointer, ok := owner.Type().Underlying().(*types.Pointer)
		if !ok || ssaflow.SameValue(owner, analysis.resource) {
			continue
		}
		if _, aggregate := pointer.Elem().Underlying().(*types.Struct); !aggregate {
			continue
		}
		for _, binding := range closure.Bindings {
			if ssaflow.CapturedBindingMatches(binding, owner) {
				return true
			}
		}
	}
	return false
}

// cleanupRegisteredBefore handles a retained callback registered before a
// captured variable is reassigned to this acquisition. The callback observes
// the cell at cleanup time, not its registration-time value. Mutable guards
// prevent proving release, so this is unknown rather than a settled resource.
// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/admin/handler_test.go#L1398-L1447
func (analysis *resourceAnalysis) cleanupRegisteredBefore(acquisition *ssa.Call) bool {
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](analysis.function) {
		if !ssaflow.InstructionDominates(deferred, acquisition) {
			continue
		}
		if analysis.capturedCellCleanup(deferred).Proven() {
			analysis.emitAction(deferred, actionUnknown, "prior-defer-may-clean-captured-cell")
			return true
		}
		if closesStatementDatabase(acquisition, deferred) {
			analysis.emitAction(deferred, actionUnknown, "statement-parent-closed")
			return true
		}
		if finishesRowsTransaction(acquisition, deferred) {
			analysis.emitAction(deferred, actionUnknown, "rows-transaction-finished")
			return true
		}
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](analysis.function) {
		if !ssaflow.InstructionDominates(call, acquisition) ||
			!ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestingCleanup) {
			continue
		}
		if slices.ContainsFunc(call.Common().Args, analysis.carriedWithinClosure) {
			analysis.emitAction(call, actionUnknown, "captured-by-prior-cleanup")
			return true
		}
	}
	return false
}

// A prior defer observes the captured cell at return, not at registration.
// When several acquisitions feed that cell, exact value completion may fail.
// A positive cleanup witness for the cell makes ownership unknown; it does
// not prove which stored value will be closed. Read-only captures and by-value
// deferred arguments do not qualify. Overwritten-cell leaks may be missed.
// https://github.com/wind-c/comqtt/blob/11282b91abb06d5169b857a2c38fad5d54502050/plugin/auth/http/http.go#L89-L127
func (analysis *resourceAnalysis) capturedCellCleanup(deferred *ssa.Defer) ssaflow.Proof {
	if closure, ok := deferred.Common().Value.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		for _, pair := range ssaflow.ClosureBindingPairs(function, closure) {
			if !ssaflow.CapturedBindingMatches(pair.Binding, analysis.resource) {
				continue
			}
			mayClean := ssaflow.MethodCallCoverage(function, func(instruction ssa.Instruction) bool {
				common := ssaflow.InstructionCall(instruction)
				return common != nil && slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) &&
					ssaflow.ValueIsAccessPathFrom(ssaflow.CallReceiver(common), pair.Free)
			}, ssaflow.CoverageAnywhere, nil)
			if mayClean {
				return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: "captured-cell-may-cleanup"}
			}
		}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
}

func (analysis *resourceAnalysis) emitAction(instruction ssa.Instruction, action resourceAction, reason string) {
	if action == actionNone || !analysis.probe.Enabled() {
		return
	}
	analysis.probe.Evidence(analysisTrace.Step{
		Reason:   reason,
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      instruction.Pos(),
		Function: analysis.function.String(),
		Details:  map[string]string{"instruction": instruction.String()},
	})
}
