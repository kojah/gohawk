package lifecyclefacts

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// LifecycleEvidence combines memoized local SSA evidence with lifecycle summaries
// imported through the prerequisite analyzer. One evidence context belongs to one source
// function and is not safe for concurrent use.
type LifecycleEvidence struct {
	pass     *analysis.Pass
	analyzer string
	check    string
	// probe attributes each traced proof step to the candidate being judged.
	// It starts unattributed so an analyzer that never scopes still traces.
	probe analysisTrace.Probe
	local lifecycle.LocalEvidence
	// retentions answers retention questions about function literals, which
	// carry no summary of their own. It is built on first use because most
	// analyzers never ask.
	retentions *retentionCache
	// localReleasedUses memoizes the released uses of this package's
	// unexported helpers, which have no exported summary.
	localReleasedUses map[*ssa.Function][]ReleasedUse
}

// ClosureRetainsValue reports whether a function literal may keep the value it
// captured, judged by reading its body.
//
// A named callee answers this from its summary, but a literal has no object
// and is never summarized, so the same question has to be asked of the body
// directly. The loose walk is the right one: this decides whether to suppress,
// so a literal that may keep the value must say yes. A literal with no body to
// read says yes for the same reason.
func (evidence *LifecycleEvidence) ClosureRetainsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return true
	}
	retentions := evidence.retentionQueries()
	for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
		if !heapmodel.CapturedBindingMatches(captured.Binding, target) &&
			!heapmodel.ValueDerivesFrom(captured.Binding, target, map[ssa.Value]bool{}) {
			continue
		}
		for _, held := range capturedUses(captured.Free) {
			if retentions.retainedAnywhere(evidence.pass, function, held) {
				return true
			}
		}
	}
	return false
}

func (evidence *LifecycleEvidence) retentionQueries() *retentionCache {
	if evidence.retentions == nil {
		evidence.retentions = newRetentionCache()
		// A consumer owns the prerequisite result, not the prerequisite's
		// object-fact namespace. Fix this lookup for the cache's whole life.
		evidence.retentions.lookup = func(call ssa.Instruction) (Fact, bool) { return factFor(evidence.pass, call) }
	}
	return evidence.retentions
}

// ClosureHandsValueToUnreadableCallee reports whether a literal passes the
// value it captured to a callee whose body this pass cannot read.
//
// Reading a literal's body decides whether the literal is transparent, and a
// transparent literal leaves the resource owned by the enclosing function. That
// is only sound for a body the pass can judge. A capture is a cell the body
// loads first, so the argument at such a call is the load rather than the
// value the caller acquired, and a summary matched on exact value identity
// never recognizes it: block/spirit closes rows through utils.CloseAndLog
// inside a deferred literal, and the release went uncredited while the literal
// was judged transparent.
//
// Rather than credit a release it cannot see, say the literal is not readable
// and let the caller keep the old opaque answer.
func (evidence *LifecycleEvidence) ClosureHandsValueToUnreadableCallee(
	closure *ssa.MakeClosure, target ssa.Value,
) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return true
	}
	for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
		if !heapmodel.CapturedBindingMatches(captured.Binding, target) &&
			!heapmodel.ValueDerivesFrom(captured.Binding, target, map[ssa.Value]bool{}) {
			continue
		}
		held := capturedUses(captured.Free)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if callHandsValueToUnreadableCallee(instruction, held) {
					return true
				}
			}
		}
	}
	return false
}

// callHandsValueToUnreadableCallee reports whether the instruction passes one
// of the held values to a callee with no body here. A dynamic callee and a
// callee in another package both qualify: go vet analyses one package at a
// time, so an imported body is absent and only its summary is available.
func callHandsValueToUnreadableCallee(instruction ssa.Instruction, held []ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	callee := common.StaticCallee()
	if callee != nil && len(callee.Blocks) > 0 {
		return false
	}
	for _, argument := range common.Args {
		for _, value := range held {
			if heapmodel.MayAlias(argument, value) {
				return true
			}
		}
	}
	return false
}

// capturedUses returns the values a literal's body actually handles for a
// captured variable. A capture is a cell, and the body loads it before use, so
// asking the retention walk about the cell alone finds nothing: that walk
// matches values exactly, unlike the traversal that follows a value forward.
func capturedUses(free *ssa.FreeVar) []ssa.Value {
	uses := []ssa.Value{free}
	if free.Referrers() == nil {
		return uses
	}
	for _, reference := range *free.Referrers() {
		if load, ok := reference.(*ssa.UnOp); ok && load.Op == token.MUL && load.X == free {
			uses = append(uses, load)
		}
	}
	return uses
}

// factOwnsImmutableCapturedArgument maps an imported fact back through one
// literal capture. The capture cell must contain only target: if the enclosing
// function can replace it before the literal runs, the load in the literal no
// longer proves which value the imported callee receives. block/spirit closes
// rows through an imported CloseAndLog helper inside a deferred literal:
// https://github.com/block/spirit/blob/c554eae8c56166ad9199fc73556b29ed581ca575/pkg/checksum/single.go#L493-L503
func factOwnsImmutableCapturedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer ssaflow.Observer) bool {
	if instruction == nil {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	function := instruction.Parent()
	if common == nil || function == nil || function.Parent() == nil {
		return false
	}
	for _, block := range function.Parent().Blocks {
		for _, candidate := range block.Instrs {
			closure, ok := candidate.(*ssa.MakeClosure)
			if !ok || closure.Fn != function {
				continue
			}
			for _, captured := range ssaflow.ClosureBindingPairs(function, closure) {
				if !immutableCapturedTarget(captured.Binding, target, closure, observer) {
					continue
				}
				for index, argument := range common.Args {
					if mask.contains(index) && slices.ContainsFunc(capturedUses(captured.Free), func(held ssa.Value) bool {
						return heapmodel.DefinitelySameValue(argument, held)
					}) {
						return true
					}
				}
			}
		}
	}
	return false
}

func immutableCapturedTarget(binding, target ssa.Value, observation ssa.Instruction, observer ssaflow.Observer) bool {
	storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer))
	if storage.Same(binding, target).Proven() {
		return true
	}
	stored := storage.StableContent(binding, observation)
	return stored.Proven() && storage.Same(stored.Value, target).Proven()
}

func (evidence *LifecycleEvidence) capturedImportedCompletion(request EvidenceRequest) Proof {
	if request.Completion == nil || request.SelectMask == nil {
		return Proof{}
	}
	common := ssaflow.InstructionCall(request.Instruction)
	if common == nil {
		return Proof{}
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return Proof{}
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) == 0 {
		return Proof{}
	}
	completes := func(instruction ssa.Instruction) bool {
		fact, summarized := factFor(evidence.pass, instruction)
		return summarized && factOwnsImmutableCapturedArgument(instruction, request.Target, request.SelectMask(fact), evidence.probe.Observer())
	}
	if !lifecycle.MethodCallCoverage(function, completes, request.Completion.Coverage, nil) {
		return Proof{}
	}
	return importedProof(reasonLifecycleSummaryCapturedArgument, requestedMethod(request))
}

// NewLifecycleEvidence constructs evidence whose accepted, rejected, and unknown
// results use the supplied analyzer identity for structured tracing.
func NewLifecycleEvidence(pass *analysis.Pass, analyzer, check string) *LifecycleEvidence {
	evidence := &LifecycleEvidence{
		pass: pass, analyzer: analyzer, check: check,
		probe: analysisTrace.ForPackage(pass, analyzer, check),
	}
	evidence.local = lifecycle.NewLocalEvidenceWithReturnedCleanup(evidence.returnedCleanupLookup())
	return evidence
}

// ForCandidate attributes the evidence traced from here on to candidate, so a
// trace selector retrieves the whole proof built for it. Analyzers call this
// once before judging each candidate.
func (evidence *LifecycleEvidence) ForCandidate(candidate token.Pos) {
	evidence.probe = analysisTrace.For(evidence.pass, evidence.analyzer, evidence.check, candidate)
}

// ArgumentRetained reports whether the summary of the call's static callee
// marks the argument at index as retained. The second result is false when
// no summary is available, which callers must treat as unknown.
func (evidence *LifecycleEvidence) ArgumentRetained(instruction ssa.Instruction, index int) (bool, bool) {
	return evidence.CalleeClaims(instruction, index, ClaimRetains)
}

// EvidenceRequest describes local and imported relationships that can settle
// one lifecycle obligation. Completion and Transfer are independently
// optional; imported selectors are consulted only when local evidence does not
// prove the obligation.
type EvidenceRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Completion  *lifecycle.CompletionRequest
	Transfer    *lifecycle.OwnershipTransferRequest
	Local       *ssaflow.Proof
	SelectMask  func(Fact) ParameterMask
	// StrictImportedProjection lets one analyzer map a summary parameter to an
	// exact, stable field/index path beneath its target. Ordinary fact matching
	// remains identity/containment-only.
	StrictImportedProjection bool
	ReceiverStore            bool
}

// Prove returns one lifecycle proof with explicit provenance. Missing imported
// summaries produce Unknown rather than being conflated with a disproved local
// relationship.
func (evidence *LifecycleEvidence) Prove(request EvidenceRequest) Proof {
	local := evidence.localProof(request)
	if local.Proven() {
		evidence.emit(request, local)
		return local
	}

	if abandonedSearch(local) {
		// The local walk was abandoned before it could decide, or found its
		// only completion inside a loop, so an imported summary that disproves
		// the release would turn a boundary the analysis gave up on into a
		// decision. Keep the undecided answer; a caller that must not report
		// on a guess checks for this reason.
		evidence.emit(request, local)
		return local
	}

	if imported, consulted := evidence.importedProof(request); consulted {
		evidence.emit(request, imported)
		return imported
	}
	evidence.emit(request, local)
	return local
}

func (evidence *LifecycleEvidence) importedProof(request EvidenceRequest) (Proof, bool) {
	// Imported summaries are consulted only after local source-visible evidence
	// fails, and each accepted mask must map back to the exact caller value.
	// Projection matching is a stricter opt-in because a field selected from an
	// owner can be reassigned or exposed independently.
	fact, summarized := factFor(evidence.pass, request.Instruction)
	if request.SelectMask != nil && summarized {
		if proof, decided := evidence.selectedMaskProof(request, fact); decided {
			return proof, true
		}
	}
	if request.ReceiverStore && summarized && factOwnsArgument(request.Instruction, request.Target, fact.ReceiverStore(), evidence.probe.Observer()) {
		receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(request.Instruction))
		if receiver != nil && (ssaflow.ExternallyOwnedValue(receiver) || lifecycle.ValueHasTransferUse(receiver)) {
			return importedProof(reasonReceiverStoreTransfer, requestedMethod(request)), true
		}
		return Proof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceDisproven, Provenance: ssaflow.EvidenceFromImportedFact,
		}, SummaryReason: reasonReceiverDoesNotEscape}, true
	}

	importedRequested := request.SelectMask != nil || request.ReceiverStore
	if importedRequested && !summarized {
		return Proof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}, true
	}
	if importedRequested && summarized {
		return Proof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound,
			Provenance: ssaflow.EvidenceFromImportedFact,
		}}, true
	}
	return Proof{}, false
}

// selectedMaskProof answers a request that selected a mask: a cleanup method
// is matched by its discharge path, other masks by identity or containment,
// an immutable capture or a strict projection by their own rules, and a
// possible alias by uncertainty. It reports false when nothing matched.
func (evidence *LifecycleEvidence) selectedMaskProof(request EvidenceRequest, fact Fact) (Proof, bool) {
	mask := request.SelectMask(fact)
	// A cleanup method is matched by its discharge path: the target must
	// be what the caller stored where the callee cleans up. Other masks
	// keep their identity-or-containment policy.
	method := requestedMethod(request)
	if method != "" && fact.dischargesArgument(request.Instruction, request.Target, method, evidence.probe.Observer()) {
		return importedProof(reasonLifecycleSummary, method), true
	}
	if factOwnsArgument(request.Instruction, request.Target, mask&^fact.MethodMask(method), evidence.probe.Observer()) {
		return importedProof(reasonLifecycleSummary, method), true
	}
	if factOwnsImmutableCapturedArgument(request.Instruction, request.Target, mask, evidence.probe.Observer()) {
		return importedProof(reasonLifecycleSummaryCapturedArgument, requestedMethod(request)), true
	}
	if request.StrictImportedProjection && factOwnsProjectedArgument(request.Instruction, request.Target, mask, evidence.probe.Observer()) {
		return importedProof(reasonLifecycleSummaryProjectedArgument, requestedMethod(request)), true
	}
	if evidence.argumentCaseCompletes(request, fact) {
		return importedProof(reasonArgumentCase, requestedMethod(request)), true
	}
	if factArgumentMatches(request.Instruction, request.Target, mask, heapmodel.MayAlias) {
		// The summary is known, but which value receives its guarantee is
		// not. This is neither completion nor evidence of missing cleanup.
		return Proof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}, true
	}
	return Proof{}, false
}

// argumentCaseCompletes reports whether a case of the callee's summary, one
// whose assumed Boolean arguments this call supplies as constants, completes
// the exact target on every normal return. Only the requested completion is
// matched; an argument case never answers a transfer or retention question.
func (evidence *LifecycleEvidence) argumentCaseCompletes(request EvidenceRequest, fact Fact) bool {
	if request.Completion == nil || len(fact.caseDischarges()) == 0 {
		return false
	}
	switch request.Instruction.(type) {
	case *ssa.Call, *ssa.Defer:
	default:
		return false
	}
	if request.Completion.InvokeTarget {
		query := ssaflow.CallCondition{Arguments: suppliedConstants(request.Instruction, nil)}
		return query.Arguments.Bound != 0 && factArgumentMatches(request.Instruction, request.Target, conditionalMask(fact, "", true, query),
			func(argument, target ssa.Value) bool {
				return heapmodel.NewStorage(request.Completion.Budget).Same(argument, target).Proven()
			})
	}
	method := requestedMethod(request)
	return method != "" && fact.caseDischargesArgument(request.Instruction, request.Target, method, nil, evidence.probe.Observer())
}

func (evidence *LifecycleEvidence) localProof(request EvidenceRequest) Proof {
	proof := Proof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	if request.Local != nil {
		proof.Proof = *request.Local
		if proof.Proven() {
			return proof
		}
	}
	if request.Completion != nil {
		completion := evidence.local.Completion(*request.Completion)
		proof.Proof = completion.Proof
		if proof.Proven() {
			return proof
		}
		if captured := evidence.capturedImportedCompletion(request); captured.Proven() {
			return captured
		}
	}
	if request.Transfer != nil {
		transfer := evidence.local.OwnershipTransfer(*request.Transfer)
		if transfer.Proven() {
			return Proof{Proof: transfer.Proof}
		}
		// A completion search abandoned before it could decide stays the
		// answer: finding no transfer does not turn it into a disproof. Prove
		// applies the same rule to imported summaries.
		if abandonedSearch(proof) {
			return proof
		}
		if !transfer.Known() {
			return Proof{Proof: transfer.Proof}
		}
		if !proof.Known() {
			return Proof{Proof: transfer.Proof}
		}
	}
	return proof
}

// abandonedSearch reports whether a local walk gave up before deciding: it ran
// out of budget, or found its only completion inside a loop.
func abandonedSearch(proof Proof) bool {
	return proof.Reason == ssaflow.EvidenceBudgetExhausted || proof.Reason == ssaflow.EvidenceCompletionInCycle
}

func importedProof(reason Reason, method string) Proof {
	return Proof{Proof: ssaflow.Proof{
		State: ssaflow.EvidenceProven, Method: method,
		Provenance: ssaflow.EvidenceFromImportedFact,
	}, SummaryReason: reason}
}

func requestedMethod(request EvidenceRequest) string {
	if request.Completion == nil || len(request.Completion.Methods) != 1 {
		return ""
	}
	return request.Completion.Methods[0]
}

func (evidence *LifecycleEvidence) emit(request EvidenceRequest, proof Proof) {
	if !evidence.probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch proof.State {
	case ssaflow.EvidenceProven:
		outcome = analysisTrace.OutcomeAccepted
	case ssaflow.EvidenceDisproven:
		outcome = analysisTrace.OutcomeRejected
	case ssaflow.EvidenceUnknown:
	}
	details := evidenceDetails(request.Instruction, request.Target)
	if proof.Method != "" {
		details["method"] = proof.Method
	}
	if proof.Provenance != "" {
		details["provenance"] = string(proof.Provenance)
	}
	position := token.NoPos
	if request.Instruction != nil {
		position = request.Instruction.Pos()
	}
	evidence.probe.Evidence(analysisTrace.Step{
		Reason:   proof.traceReason(),
		Outcome:  outcome,
		Pos:      position,
		Function: instructionFunction(request.Instruction),
		Details:  details,
	})
}

func evidenceDetails(instruction ssa.Instruction, target ssa.Value) map[string]string {
	details := map[string]string{}
	if instruction != nil {
		// The SSA text of the instruction lets a reader follow the trace as an
		// annotated SSA walk without rebuilding the lowering by hand.
		details["instruction"] = instruction.String()
	}
	if target != nil {
		details["target"] = target.Name()
		if target.Type() != nil {
			details["target_type"] = target.Type().String()
		}
	}
	if common := ssaflow.InstructionCall(instruction); common != nil && common.StaticCallee() != nil {
		details["callee"] = common.StaticCallee().String()
	}
	return details
}

func instructionFunction(instruction ssa.Instruction) string {
	if instruction == nil || instruction.Parent() == nil {
		return ""
	}
	return instruction.Parent().String()
}
