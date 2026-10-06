package lifecyclefacts

import (
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	proofs "github.com/kojah/gohawk/internal/proof"
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
	Local       *proofs.Proof
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
		return Proof{Proof: proofs.Proof{
			State: proofs.EvidenceDisproven, Provenance: proofs.EvidenceFromImportedFact,
		}, SummaryReason: reasonReceiverDoesNotEscape}, true
	}

	importedRequested := request.SelectMask != nil || request.ReceiverStore
	if importedRequested && !summarized {
		return Proof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}, true
	}
	if importedRequested && summarized {
		return Proof{Proof: proofs.Proof{
			State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound,
			Provenance: proofs.EvidenceFromImportedFact,
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
		return Proof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}, true
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
		query := suppliedCondition(request.Instruction, nil)
		return !query.Unconditional() && factArgumentMatches(request.Instruction, request.Target, conditionalMask(fact, "", true, query),
			func(argument, target ssa.Value) bool {
				return heapmodel.NewStorage(request.Completion.Budget).Same(argument, target).Proven()
			})
	}
	method := requestedMethod(request)
	return method != "" && fact.caseDischargesArgument(request.Instruction, request.Target, method, nil, evidence.probe.Observer())
}

func (evidence *LifecycleEvidence) localProof(request EvidenceRequest) Proof {
	proof := Proof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
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
	return proof.Reason == proofs.EvidenceBudgetExhausted || proof.Reason == proofs.EvidenceCompletionInCycle
}

func importedProof(reason Reason, method string) Proof {
	return Proof{Proof: proofs.Proof{
		State: proofs.EvidenceProven, Method: method,
		Provenance: proofs.EvidenceFromImportedFact,
	}, SummaryReason: reason}
}

// question names what a request asked, so a trace tells apart the several
// answers one instruction gets: a release, a transfer, a proof the caller
// supplied, or a summary mask.
func (request EvidenceRequest) question() string {
	var parts []string
	if request.Local != nil {
		parts = append(parts, "local")
	}
	if request.Completion != nil {
		parts = append(parts, "release")
	}
	if request.Transfer != nil {
		parts = append(parts, "transfer")
	}
	if request.SelectMask != nil {
		parts = append(parts, "summary")
	}
	if request.ReceiverStore {
		parts = append(parts, "receiver-store")
	}
	return strings.Join(parts, "+")
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
	case proofs.EvidenceProven:
		outcome = analysisTrace.OutcomeAccepted
	case proofs.EvidenceDisproven:
		outcome = analysisTrace.OutcomeRejected
	case proofs.EvidenceUnknown:
	}
	details := evidenceDetails(request.Instruction, request.Target)
	details["question"] = request.question()
	if proof.Method != "" {
		details["method"] = proof.Method
	}
	if proof.Provenance != 0 {
		details["provenance"] = proof.Provenance.String()
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
