package resourcelifetime

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The use-after-release check is the dual of the leak check: after a direct
// release of an acquired resource, an operation the API documents as failing
// on a released value is reported. It is deliberately narrow. The
// release must be a plain call on the exact acquired value, not a deferred
// one, and it must dominate the use, so every path to the use has released
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
		{"database/sql", "Rows", []string{"Scan", "Columns", "ColumnTypes"}},
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
	query := releasedResource{resource: resource, contract: contract, methods: methods, storage: ssaflow.NewStorage(nil), knowledge: knowledge}
	reported := map[*ssa.Call]bool{}
	for _, release := range directReleases(function, &query) {
		analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), release.Pos()).Evidence(analysisTrace.Step{
			Reason: "known-resource-direct-release", Outcome: analysisTrace.OutcomeAccepted,
		})
		for _, instruction := range ssaflow.InstructionsReachableAfter(release) {
			call, ok := instruction.(*ssa.Call)
			if !ok || reported[call] || !slices.Contains(methods, ssaflow.CallName(call.Common())) || !query.operatesOn(call) {
				continue
			}
			probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceUseAfterRelease), call.Pos())
			probe.Candidate(analysisTrace.Step{Reason: "operation-on-released-resource"})
			proof := query.prove(acquisition, release, call)
			if !proof.Proven() {
				emitUnknownUseAfterRelease(function, probe, proof)
				continue
			}
			emitUseAfterRelease(pass, function, acquisition, release, call)
			reportUseAfterRelease(pass, acquisition, release, call, contract)
			reported[call] = true
		}
	}
}

type releasedResource struct {
	resource  ssa.Value
	contract  resourceContract
	methods   []string
	storage   *ssaflow.Storage
	knowledge *summaries.Provider
}

type useAfterReleaseProof struct {
	ssaflow.Proof
	interference ssa.Instruction
}

// Dominance supplies the ordering proof; a bounded effect scan supplies its
// validity interval. Reset, mutation, opaque helpers and asynchronous exposure
// can change the lifecycle even when pointer identity stays the same. Merely
// not recognizing another Close is not evidence that the value stays closed.
func (query *releasedResource) prove(acquisition, release, use *ssa.Call) useAfterReleaseProof {
	unknown := func(reason ssaflow.EvidenceReason, interference ssa.Instruction) useAfterReleaseProof {
		return useAfterReleaseProof{
			Proof:        ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: reason},
			interference: interference,
		}
	}
	if !ssaflow.InstructionDominates(release, use) {
		return unknown("release-does-not-dominate-use", nil)
	}
	if !query.releaseInvalidates(release, use) {
		return unknown("release-success-not-proven", nil)
	}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	effects := ssaflow.NewCallEffects(budget)
	for _, instruction := range ssaflow.InstructionsReachableAfter(acquisition) {
		if !budget.Spend() {
			return unknown("release-use-budget-exhausted", nil)
		}
		if instruction == use || instruction == release || !ssaflow.InstructionMayFollow(instruction, use) {
			continue
		}
		if ssaflow.InstructionTerminatesControlFlow(instruction) && ssaflow.InstructionDominates(instruction, use) {
			return unknown("release-use-unreachable", instruction)
		}
		if query.interferes(instruction, effects) {
			return unknown("release-use-opaque-effect", instruction)
		}
	}
	return useAfterReleaseProof{Proof: ssaflow.Proof{
		State: ssaflow.EvidenceProven, Reason: "release-dominates-use", Provenance: ssaflow.EvidenceFromLocalSSA,
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
	if update, ok := instruction.(*ssa.MapUpdate); ok && ssaflow.MayContainValue(update.Value, query.resource) {
		return true // Collection ownership and later mutation are not modeled.
	}
	return ssaflow.ClosureCapturesValue(instruction, query.resource) || ssaflow.SendsValue(instruction, query.resource) ||
		ssaflow.StoresValueInGlobal(instruction, query.resource) || ssaflow.StoresValueInEscapingField(instruction, query.resource)
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
		if ssaflow.MayContainValue(argument, query.resource) &&
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

// directReleases returns the plain calls of a cleanup method on the exact
// resource. Deferred releases run at return and cannot precede a use.
func directReleases(function *ssa.Function, query *releasedResource) []*ssa.Call {
	var releases []*ssa.Call
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && slices.Contains(query.contract.cleanup, ssaflow.CallName(call.Common())) && query.operatesOn(call) {
				releases = append(releases, call)
			}
		}
	}
	return releases
}

// Point-in-time storage identity preserves saved aliases while rejecting
// replacement fields and mixed joins. HTTP bodies additionally require an
// unchanged Body projection: merely sharing the response root is not enough.
func (query *releasedResource) operatesOn(call *ssa.Call) bool {
	// The same receiver resolution as the release proof: a call proven to
	// return its argument unchanged operates on that argument.
	receiver := cleanupReceiver(query.knowledge, call.Common())
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
		Reason:   "release-dominates-use",
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
	step := analysisTrace.Step{Reason: string(proof.Reason), Outcome: analysisTrace.OutcomeUnknown}
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
	release *ssa.Call,
	use *ssa.Call,
	contract resourceContract,
) {
	useSource := syntax.SourceRange(pass, use.Pos())
	acquisitionSource := syntax.SourceRange(pass, acquisition.Pos())
	releaseSource := syntax.SourceRange(pass, release.Pos())
	check.Report(pass, check.ResourceUseAfterRelease, analysis.Diagnostic{
		Pos: useSource.Pos(), End: useSource.End(),
		Message: fmt.Sprintf("resource from %s.%s is used after %s",
			syntax.ShortPackageName(contract.packagePath), contract.name, ssaflow.CallName(release.Common())),
		Related: []analysis.RelatedInformation{
			{Pos: acquisitionSource.Pos(), End: acquisitionSource.End(), Message: "resource acquired here"},
			{Pos: releaseSource.Pos(), End: releaseSource.End(), Message: "resource released here"},
		},
	})
}
