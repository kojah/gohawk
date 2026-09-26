package resourcelifetime

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/internal/check"
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

// The use-after-release check is the dual of the leak check: after a release
// of an acquired resource, an operation the API documents as failing on a
// released value is reported. It is deliberately narrow. The release must be
// a plain call, not a deferred one: a cleanup call on the exact acquired
// value, or a helper proven to release that exact value on every normal
// return. It must dominate the use, so every path to the use has released
// first; a release on one branch and a use after the merge is not claimed.
// Only operations documented to fail on a released value count. Idioms that
// touch a released value harmlessly, such as rows.Err after rows.Close, a
// deferred Close after an explicit one, or Rollback after a failed Commit,
// are not in the table.

type invalidatingOperation struct {
	packagePath string
	name        string
	methods     []string
}

// invalidatingOperations lists, per resource type, the methods whose
// documented behavior on a released value is an error.
func invalidatingOperations() []invalidatingOperation {
	return []invalidatingOperation{
		{"os", "File", []string{
			"Read", "ReadAt", "ReadFrom", "Write", "WriteAt", "WriteString", "Seek", "Sync", "Truncate", "Readdir", "ReadDir", "Readdirnames",
		}},
		// Next after Close is documented to return false, so a loop over
		// closed rows silently sees no rows; a helper that iterates the rows
		// it is handed therefore requires them unreleased, which its
		// summary states as requiring Next.
		{"database/sql", "Rows", []string{"Scan", "Columns", "ColumnTypes", "Next"}},
		{"database/sql", "Tx", []string{
			"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Prepare", "PrepareContext", "Stmt", "StmtContext",
		}},
		{"database/sql", "Stmt", []string{"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext"}},
		{"net/http", "Response", []string{"Read"}},
		// Reader.Close does not guarantee that a later Read fails. In
		// particular, gzip delegates to flate, whose Close need not invalidate
		// the reader. A cleanup obligation alone is not an invalidation contract.
		{"compress/gzip", "Writer", []string{"Write", "Flush"}},
		{"compress/zlib", "Writer", []string{"Write", "Flush"}},
	}
}

func invalidatingMethods(resource ssa.Value) []string {
	for _, entry := range invalidatingOperations() {
		if syntax.NamedType(resource.Type(), entry.packagePath, entry.name) {
			return entry.methods
		}
	}
	return nil
}

func reportUsesAfterRelease(
	pass *analysis.Pass, knowledge *summaries.Provider, function *ssa.Function, acquisition *ssa.Call, resource ssa.Value, contract resourceContract,
) {
	methods := invalidatingMethods(resource)
	if len(methods) == 0 {
		return
	}
	evidence, _ := knowledge.LifecycleEvidence("resourcelifetime", string(check.ResourceUseAfterRelease))
	query := releasedResource{
		resource: resource, contract: contract, methods: methods, storage: heapmodel.NewStorage(nil), knowledge: knowledge, evidence: evidence,
	}
	reported := map[*ssa.Call]bool{}
	for _, point := range releasePoints(function, &query) {
		release := point.call
		analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), release.Pos()).Evidence(analysisTrace.Step{
			Reason: point.reason().String(), Outcome: analysisTrace.OutcomeAccepted,
		})
		for _, instruction := range ssaflow.InstructionsReachableAfter(release) {
			call, ok := instruction.(*ssa.Call)
			if !ok || reported[call] {
				continue
			}
			operation, ok := query.operation(call)
			if !ok {
				continue
			}
			probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), call.Pos())
			probe.Candidate(analysisTrace.Step{Reason: resourceReasonOperationOnReleasedResource.String()})
			if operation.helper != nil {
				probe.Evidence(analysisTrace.Step{
					Reason: resourceReasonHelperRequiresOperation.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: call.Pos(),
					Details: map[string]string{"helper": operation.helper.String(), "method": operation.method},
				})
			}
			proof := query.prove(acquisition, release, call)
			if !proof.Proven() {
				emitUnknownUseAfterRelease(function, probe, proof)
				continue
			}
			emitUseAfterRelease(pass, function, acquisition, release, call)
			reportUseAfterRelease(pass, acquisition, point, call, contract, operation)
			reported[call] = true
		}
	}
}

// releasedOperation is one call that performs an invalidating operation on
// the released resource: the operation itself, or a summarized helper that
// performs it on every path with the resource it was handed.
type releasedOperation struct {
	method string
	helper *ssa.Function
}

// operation decides whether the call operates on the released resource with
// a method its contract says fails after release. A direct call is one
// whose receiver is the exact resource. A helper counts when its summary
// requires the method of the argument at the position the exact resource
// is passed: the helper's summary names what it calls, and the resource's
// own type decides that the name is an invalidating operation, so a helper
// that calls Err on the rows it is handed is as silent as a direct rows.Err.
// https://github.com/facebook/infer: this is the precondition footprint of a
// Pulse summary, checked against the caller's released state.
func (query *releasedResource) operation(call *ssa.Call) (releasedOperation, bool) {
	name := ssaflow.CallName(call.Common())
	if slices.Contains(query.methods, name) && query.operatesOn(call) {
		return releasedOperation{method: name}, true
	}
	callee := call.Common().StaticCallee()
	if query.evidence == nil || callee == nil || call.Common().IsInvoke() {
		return releasedOperation{}, false
	}
	for index, argument := range call.Common().Args {
		if !query.storage.Same(argument, query.resource).Proven() {
			continue
		}
		for _, required := range query.evidence.ArgumentMethodsRequired(call, index) {
			if slices.Contains(query.methods, required) {
				return releasedOperation{method: required, helper: callee}, true
			}
		}
	}
	return releasedOperation{}, false
}

type releasedResource struct {
	resource  ssa.Value
	contract  resourceContract
	methods   []string
	storage   *heapmodel.Storage
	knowledge *summaries.Provider
	evidence  *lifecyclefacts.LifecycleEvidence
}

type useAfterReleaseProof struct {
	resourceProof
	interference ssa.Instruction
}

// Dominance supplies the ordering proof; a bounded effect scan supplies its
// validity interval. Reset, mutation, opaque helpers and asynchronous exposure
// can change the lifecycle even when pointer identity stays the same. Merely
// not recognizing another Close is not evidence that the value stays closed.
func (query *releasedResource) prove(acquisition, release, use *ssa.Call) useAfterReleaseProof {
	unknown := func(reason resourceLifetimeReason, interference ssa.Instruction) useAfterReleaseProof {
		return useAfterReleaseProof{
			resourceProof: resourceProof{State: ssaflow.EvidenceUnknown, Reason: reason},
			interference:  interference,
		}
	}
	if !ssaflow.InstructionDominates(release, use) {
		return unknown(resourceReasonReleaseDoesNotDominateUse, nil)
	}
	if !query.releaseInvalidates(release, use) {
		return unknown(resourceReasonReleaseSuccessNotProven, nil)
	}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	effects := ssaflow.NewCallEffects(budget)
	for _, instruction := range ssaflow.InstructionsReachableAfter(acquisition) {
		if !budget.Spend() {
			return unknown(resourceReasonReleaseUseBudgetExhausted, nil)
		}
		if instruction == use || instruction == release || !ssaflow.InstructionMayFollow(instruction, use) {
			continue
		}
		// A call the summaries prove never returns ends the path as os.Exit
		// does, so a use only reachable through it is not reached.
		if ssaflow.InstructionTerminatesWith(instruction, query.knowledge.Terminates()) && ssaflow.InstructionDominates(instruction, use) {
			return unknown(resourceReasonReleaseUseUnreachable, instruction)
		}
		if query.interferes(instruction, effects) {
			return unknown(resourceReasonReleaseUseOpaqueEffect, instruction)
		}
	}
	return useAfterReleaseProof{resourceProof: resourceProof{
		State: ssaflow.EvidenceProven, Reason: resourceReasonReleaseDominatesUse, Provenance: ssaflow.EvidenceFromLocalSSA,
	}}
}

func (query *releasedResource) releaseInvalidates(release, use *ssa.Call) bool {
	if !syntax.NamedType(query.resource.Type(), "database/sql", "Tx") || ssaflow.CallName(release.Common()) != "Commit" {
		return true
	}
	// Commit can return a canceled context before marking the transaction done.
	// Only its proven success branch establishes invalidation here; a failed
	// attempt is not interchangeable with a completed transaction.
	for _, successor := range release.Block().Succs {
		if success, known := ssaflow.SuccessBranch(release.Block(), successor, release); known && success && successor.Dominates(use.Block()) {
			return true
		}
	}
	return false
}

func (query *releasedResource) interferes(instruction ssa.Instruction, effects *ssaflow.CallEffects) bool {
	if call, ok := instruction.(*ssa.Call); ok && query.operatesOn(call) &&
		(slices.Contains(query.methods, ssaflow.CallName(call.Common())) || slices.Contains(query.contract.cleanup, ssaflow.CallName(call.Common()))) {
		return false
	}
	if common := ssaflow.InstructionCall(instruction); common != nil && query.callInterferes(instruction, common, effects) {
		return true
	}
	if store, ok := instruction.(*ssa.Store); ok &&
		(query.storage.Same(store.Addr, query.resource).Proven() || ssaflow.ValueIsAccessPathFrom(store.Addr, query.resource)) {
		return true // Overwriting the resource object can reopen the same pointer.
	}
	if update, ok := instruction.(*ssa.MapUpdate); ok && lifecycle.MayContainValue(update.Value, query.resource) {
		return true // Collection ownership and later mutation are not modeled.
	}
	return lifecycle.ClosureCapturesValue(instruction, query.resource) || lifecycle.SendsValue(instruction, query.resource) ||
		lifecycle.StoresValueInGlobal(instruction, query.resource) || lifecycle.StoresValueInEscapingField(instruction, query.resource)
}

// callInterferes reports whether a call may change the resource's lifecycle
// between the release and the use: it receives the resource, or something
// holding it, and is not proven to leave its storage alone.
func (query *releasedResource) callInterferes(instruction ssa.Instruction, common *ssa.CallCommon, effects *ssaflow.CallEffects) bool {
	if _, deferred := instruction.(*ssa.Defer); deferred {
		return false // Registration does not execute cleanup before the use.
	}
	if call, ok := instruction.(*ssa.Call); ok && query.passesResourceThrough(call, effects) {
		return false // Handing the resource back unchanged is not a lifecycle change.
	}
	for _, argument := range append([]ssa.Value{common.Value}, common.Args...) {
		if lifecycle.MayContainValue(argument, query.resource) &&
			(!query.storage.Same(argument, query.resource).Proven() || !effects.Call(instruction, argument).PreservesStorage()) {
			return true
		}
	}
	return false
}

// passesResourceThrough reports whether a call is proven to return the
// resource unchanged and to do nothing else with it but read. The call's
// retention effect is explained by the return, so it is not the opaque
// handoff the interference scan otherwise assumes; a write through the
// resource, an asynchronous exposure, or a callback invocation still counts.
func (query *releasedResource) passesResourceThrough(call *ssa.Call, effects *ssaflow.CallEffects) bool {
	if query.knowledge == nil {
		return false
	}
	argument, ok := query.knowledge.ArgumentReturnedUnchanged(call, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !ok || !query.storage.Same(argument, query.resource).Proven() {
		return false
	}
	proof := effects.Call(call, argument)
	return proof.Proven() && proof.Effects&^(ssaflow.EffectRead|ssaflow.EffectRetain) == 0
}

// releasePoint is a plain call after which the exact resource is released:
// a cleanup method called on it, or a helper proven to call one on it.
type releasePoint struct {
	call   *ssa.Call
	method string
	helper *ssa.Function
}

func (point releasePoint) reason() resourceLifetimeReason {
	if point.helper != nil {
		return resourceReasonKnownResourceHelperRelease
	}
	return resourceReasonKnownResourceDirectRelease
}

// releasePoints returns the plain calls that release the exact resource.
// Deferred releases run at return and cannot precede a use.
func releasePoints(function *ssa.Function, query *releasedResource) []releasePoint {
	var points []releasePoint
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			if name := ssaflow.CallName(call.Common()); slices.Contains(query.contract.cleanup, name) && query.operatesOn(call) {
				points = append(points, releasePoint{call: call, method: name})
				continue
			}
			if method, ok := query.helperRelease(call); ok {
				points = append(points, releasePoint{call: call, method: method, helper: call.Common().StaticCallee()})
			}
		}
	}
	return points
}

// helperRelease reports the cleanup method a helper call is proven to call on
// the exact resource on every normal return: unconditionally, or in the case
// the call's constant arguments select. The leak check settles on weaker
// evidence, an exhausted search or an aggregate the resource sits in, because
// settling only suppresses a report; a release point starts one, so it needs
// the positive proof for the value itself. A helper Commit is not a release
// point: only the success branch of a direct Commit invalidates a transaction.
func (query *releasedResource) helperRelease(call *ssa.Call) (string, bool) {
	common := call.Common()
	if query.evidence == nil || common.IsInvoke() || common.StaticCallee() == nil {
		return "", false
	}
	if !slices.ContainsFunc(common.Args, func(argument ssa.Value) bool { return query.storage.Same(argument, query.resource).Proven() }) {
		return "", false
	}
	for _, method := range query.contract.cleanup {
		if method == "Commit" {
			continue
		}
		completion := lifecycle.CompletionRequest{
			Instruction: call, Target: query.resource, Methods: []string{method}, ExactTarget: true,
			Budget: ssaflow.NewSearchBudget(releaseSearchBudget),
		}
		proof := query.evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: call, Target: query.resource, Completion: &completion, SelectMask: releaseMask(call, query.resource, method),
		})
		if proof.State == ssaflow.EvidenceProven {
			return method, true
		}
	}
	return "", false
}

// Point-in-time storage identity preserves saved aliases while rejecting
// replacement fields and mixed joins. HTTP bodies additionally require an
// unchanged Body projection: merely sharing the response root is not enough.
func (query *releasedResource) operatesOn(call *ssa.Call) bool {
	// The same receiver resolution as the release proof: a call proven to
	// return its argument unchanged operates on that argument.
	receiver := cleanupReceiver(query.knowledge, ssaflow.NewSearchBudget(ssaflow.SummaryBudget), call.Common())
	if receiver == nil {
		return false
	}
	if query.storage.Same(receiver, query.resource).Proven() {
		return true
	}
	if !syntax.NamedType(query.resource.Type(), "net/http", "Response") {
		return false
	}
	field := httpResponseBodyField(receiver)
	if field == nil || !query.storage.Same(field.X, query.resource).Proven() {
		return false
	}
	load, ok := receiver.(*ssa.UnOp)
	return ok && query.storage.Projection(receiver, query.resource, load).Proven()
}

func emitUseAfterRelease(pass *analysis.Pass, function *ssa.Function, acquisition *ssa.Call, release, use *ssa.Call) {
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), use.Pos())
	if !probe.Enabled() {
		return
	}
	probe.Evidence(analysisTrace.Step{
		Reason:   resourceReasonReleaseDominatesUse.String(),
		Outcome:  analysisTrace.OutcomeRejected,
		Pos:      use.Pos(),
		Function: function.String(),
		Details: map[string]string{
			"acquisition": pass.Fset.Position(acquisition.Pos()).String(),
			"release":     release.String(),
			"instruction": use.String(),
		},
	})
}

func emitUnknownUseAfterRelease(
	function *ssa.Function,
	probe analysisTrace.Probe,
	proof useAfterReleaseProof,
) {
	step := analysisTrace.Step{Reason: proof.Reason.String(), Outcome: analysisTrace.OutcomeUnknown}
	if proof.interference != nil && probe.Enabled() {
		step.Pos = proof.interference.Pos()
		step.Function = function.String()
		step.Details = map[string]string{"instruction": proof.interference.String()}
	}
	probe.Decision(step)
}

func reportUseAfterRelease(
	pass *analysis.Pass,
	acquisition *ssa.Call,
	release releasePoint,
	use *ssa.Call,
	contract resourceContract,
	operation releasedOperation,
) {
	useSource := syntax.SourceRange(pass, use.Pos())
	acquisitionSource := syntax.SourceRange(pass, acquisition.Pos())
	releaseSource := syntax.SourceRange(pass, release.call.Pos())
	related := []analysis.RelatedInformation{
		{Pos: acquisitionSource.Pos(), End: acquisitionSource.End(), Message: "resource acquired here"},
		{Pos: releaseSource.Pos(), End: releaseSource.End(), Message: "resource released here"},
	}
	if release.helper != nil {
		related = append(related, analysis.RelatedInformation{
			Pos: releaseSource.Pos(), End: releaseSource.End(),
			Message: fmt.Sprintf("%s calls %s on the resource on every path", release.helper.RelString(nil), release.method),
		})
	}
	if operation.helper != nil {
		related = append(related, analysis.RelatedInformation{
			Pos: useSource.Pos(), End: useSource.End(),
			Message: fmt.Sprintf("%s calls %s on the resource on every path", operation.helper.RelString(nil), operation.method),
		})
	}
	check.Report(pass, check.ResourceUseAfterRelease, analysis.Diagnostic{
		Pos: useSource.Pos(), End: useSource.End(),
		Message: fmt.Sprintf("resource from %s.%s is used after %s",
			syntax.ShortPackageName(contract.packagePath), contract.name, release.method),
		Related: related,
	})
}
