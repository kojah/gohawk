package resourcelifetime

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Resource contracts are the authoritative acquisition, cleanup, and transfer
// vocabulary for this analyzer. Exact symbols are required so similarly named
// application methods do not imply ownership.

type resourceContract struct {
	symbol      syntax.Symbol
	family      string
	packagePath string
	name        string
	cleanup     []string
	result      int
	// retained marks a wrapper result that holds a resource no method of
	// the wrapper can release; the diagnostic says so.
	retained bool
	// role names which result this contract owns, when the call returns
	// more than one resource, as os.Pipe returns a read end and a write end.
	role string
}

func resourceContracts() []resourceContract {
	return []resourceContract{
		resourceFunction("os", "os", "Create", 0, "Close"),
		resourceFunction("os", "os", "CreateTemp", 0, "Close"),
		resourceFunction("os", "os", "Open", 0, "Close"),
		resourceFunction("os", "os", "OpenFile", 0, "Close"),
		// Each end of a pipe is its own descriptor: closing one releases
		// nothing of the other.
		resourceFunction("os", "os", "Pipe", 0, "Close").withRole("read end"),
		resourceFunction("os", "os", "Pipe", 1, "Close").withRole("write end"),
		// Channel timers are GC-managed since Go 1.23. Missing Stop alone
		// proves no leak. Main-module/runtime overrides are not established by
		// this package-local pass, so legacy timer behavior is not inferred.
		// This says nothing about AfterFunc callbacks or retained workers.
		// https://github.com/okteto/okteto/blob/ad42c0823762a2255d4b4ad2e53fb4ec190010e7/cmd/deploy/wait.go#L70-L71

		resourceMethod("sql", "database/sql", "DB", "Begin", "Commit", "Rollback"),
		resourceMethod("sql", "database/sql", "DB", "BeginTx", "Commit", "Rollback"),
		resourceMethod("sql", "database/sql", "Conn", "BeginTx", "Commit", "Rollback"),
		resourceMethod("sql", "database/sql", "DB", "Query", "Close"),
		resourceMethod("sql", "database/sql", "DB", "QueryContext", "Close"),
		resourceMethod("sql", "database/sql", "Conn", "QueryContext", "Close"),
		resourceMethod("sql", "database/sql", "Tx", "Query", "Close"),
		resourceMethod("sql", "database/sql", "Tx", "QueryContext", "Close"),
		resourceMethod("sql", "database/sql", "Stmt", "Query", "Close"),
		resourceMethod("sql", "database/sql", "Stmt", "QueryContext", "Close"),
		// Statements prepared on a transaction are closed automatically when that
		// transaction commits or rolls back, so Tx.Prepare* is deliberately absent.
		resourceMethod("sql", "database/sql", "DB", "Prepare", "Close"),
		resourceMethod("sql", "database/sql", "DB", "PrepareContext", "Close"),
		resourceMethod("sql", "database/sql", "Conn", "PrepareContext", "Close"),

		resourceFunction("http", "net/http", "Get", 0, "Close"),
		resourceFunction("http", "net/http", "Post", 0, "Close"),
		resourceFunction("http", "net/http", "PostForm", 0, "Close"),
		resourceMethod("http", "net/http", "Client", "Do", "Close"),
		// net/http documents the same obligation for these Client methods as
		// for the package functions above: "Caller should close resp.Body when
		// done reading from it." Head carries no such sentence, and a HEAD
		// response usually has http.NoBody, so neither Head form is listed.
		resourceMethod("http", "net/http", "Client", "Get", "Close"),
		resourceMethod("http", "net/http", "Client", "Post", "Close"),
		resourceMethod("http", "net/http", "Client", "PostForm", "Close"),

		// Compression readers do not own their inputs or require finalization.
		// Close neither closes the underlying reader nor validates a checksum;
		// missing it is not a resource-lifetime violation.
		// https://github.com/flux-iac/tofu-controller/blob/8fc67730e8b5d48092060f22b89bd2e88fc0ce86/api/plan/gzip.go#L28
		resourceFunction("compress", "compress/gzip", "NewWriterLevel", 0, "Close"),
		resourceFunction("compress", "compress/gzip", "NewWriter", -1, "Close"),
		resourceFunction("compress", "compress/zlib", "NewWriterLevel", 0, "Close"),
		resourceFunction("compress", "compress/zlib", "NewWriterLevelDict", 0, "Close"),
		resourceFunction("compress", "compress/zlib", "NewWriter", -1, "Close"),
	}
}

// Rows.Next closes on exhaustion of the final result set. A false result can
// also mark an intermediate set, so this is uncertainty, not proven release.
// Only the false edge of the exact SQL receiver qualifies; breaks and Scan
// errors still leave a live obligation.
// https://github.com/nkanaev/yarr/blob/bb427710efec1ba2c9b9b4cacde67874a347499b/src/storage/sqlite/item.go#L284
func proveSQLRowsExhaustionEdge(block, successor *ssa.BasicBlock, resource ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonNone, budget)
	}
	if len(block.Instrs) == 0 || len(block.Succs) != 2 || successor != block.Succs[1] {
		return carriedValueProof(false, resourceReasonNone, budget)
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return carriedValueProof(false, resourceReasonNone, budget)
	}
	next, ok := branch.Cond.(*ssa.Call)
	exhausted := ok && ssaflow.CallMatchesSymbol(next.Common(), syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "database/sql", Receiver: "Rows", Name: "Next",
	})) && heapmodel.NewStorage(budget).Same(ssaflow.CallReceiver(next.Common()), resource).Proven()
	return carriedValueProof(exhausted, resourceReasonRowsExhaustedEdgeUnknown, budget)
}

func resourceFunction(family, packagePath, name string, result int, cleanup ...string) resourceContract {
	return resourceContract{
		symbol: syntax.PackageFunction(packagePath, name), family: family, packagePath: packagePath, name: name, cleanup: cleanup, result: result,
	}
}

func (contract resourceContract) withRole(role string) resourceContract {
	contract.role = role
	return contract
}

func resourceMethod(family, packagePath, receiver, name string, cleanup ...string) resourceContract {
	return resourceContract{
		symbol:      syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: receiver, Name: name}),
		family:      family,
		packagePath: packagePath,
		name:        name,
		cleanup:     cleanup,
		result:      0,
	}
}

func sqlDatabaseCall(common *ssa.CallCommon, names ...string) bool {
	return sqlReceiverCall(common, "DB", names...)
}

func sqlReceiverCall(common *ssa.CallCommon, receiver string, names ...string) bool {
	for _, name := range names {
		if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
			PackagePath: "database/sql", Receiver: receiver, Name: name,
		})) {
			return true
		}
	}
	return false
}

// resourceContractsFor returns every contract the call matches: one for
// each owned result.
func resourceContractsFor(common *ssa.CallCommon, settings resourceLifetimeSettings) []resourceContract {
	var contracts []resourceContract
	for _, contract := range settings.catalog {
		if ssaflow.CallMatchesSymbol(common, contract.symbol) {
			contracts = append(contracts, contract)
		}
	}
	return contracts
}

// releasesResource classifies an instruction as settling the resource, as
// an uncertain release, or as neither. Uncertainty arises when the only
// release a helper performs lies inside a loop: the flow then neither
// credits nor reports it.
func (analysis *resourceAnalysis) releasesResource(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason) {
	if analysis.optional.Proven() {
		// The optional-acquisition proof deliberately authorizes only cleanup
		// through its exact resource phi. Letting the ordinary existential
		// derivation rules inspect a later phi could mistake cleanup of another
		// non-nil resource for cleanup of the acquired one.
		if optionalAcquisitionReleases(instruction, analysis.resource, analysis.contract.cleanup) {
			return actionSettled, resourceReasonSettled
		}
		return actionNone, resourceReasonNone
	}
	return analysis.releasesOrdinaryResource(instruction)
}

// cleanupReceiver is the value a cleanup call acts on, seen through a
// wrapper the result summary proves returns its argument unchanged:
// wrap(file).Close() closes file. The identity is exact and typed, so a
// conversion, a chosen value, or a real wrapper is not seen through.
//
//nolint:ireturn // SSA values keep their concrete forms.
func cleanupReceiver(knowledge *summaries.Provider, budget *ssaflow.SearchBudget, common *ssa.CallCommon) ssa.Value {
	receiver := ssaflow.CallReceiver(common)
	if knowledge == nil || receiver == nil {
		return receiver
	}
	if argument, ok := knowledge.ArgumentReturnedUnchanged(receiver, budget); ok {
		return argument
	}
	return receiver
}

func (analysis *resourceAnalysis) releasesOrdinaryResource(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason) {
	evidence, knowledge := analysis.evidence, analysis.summaries
	resource, owners, methods := analysis.resource, analysis.owners, analysis.contract.cleanup
	storage := heapmodel.NewStorage(analysis.budget(ssaflow.QueryBudget))
	settled := func() (resourceAction, resourceLifetimeReason) { return actionSettled, resourceReasonSettled }
	// Installing a resource in package storage transfers cleanup to that
	// package's lifecycle, as in Argus's Init/Close logging pair:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/uxlog/uxlog.go#L21-L39
	stored := analysis.resourceStorage(instruction)
	if stored.State == ssaflow.EvidenceUnknown && stored.Reason == resourceReasonBudgetExhausted {
		return actionUnknown, stored.Reason
	}
	if stored.Proven() || instructionSettlesResourceOwnership(evidence, instruction, resource) ||
		callTakesResourceOwnership(evidence, storage, instruction, resource, methods) ||
		registersCleanupCallback(evidence, instruction, resource, methods) {
		return settled()
	}
	common := ssaflow.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssaflow.CallName(common)) &&
		(storage.Same(cleanupReceiver(knowledge, storage.Budget(), common), resource).Proven() ||
			storage.Projection(ssaflow.CallReceiver(common), resource, instruction).Proven()) {
		return settled()
	}
	// A helper can invoke Close through an interface on every normal return
	// without a direct discharge mask. Its heap method requirement still
	// proves cleanup when the actual argument is exactly this acquisition.
	// https://github.com/cinar/indicator/blob/18a0b934a565dc5cbfcf5966f599882be80f6d40/helper/closer.go#L13-L25
	if common != nil && helperRequiresCleanup(evidence, storage, instruction, resource, methods) {
		return settled()
	}
	if common != nil && resourceLifecycleMethod(ssaflow.CallName(common)) && heapmodel.MayAliasAny(ssaflow.CallReceiver(common), owners) {
		return settled()
	}
	for _, method := range methods {
		// One completion proof covers every launch form. The callee may be a
		// deferred literal, a helper called now, a launched worker, a stored or
		// OnceFunc-wrapped callback, or a testing Cleanup registration, and it
		// may receive the owner, a cleanup-bearing projection such as a response
		// body, or a callback bound to the cleanup method. Representative shapes:
		// Notifiarr drains and closes a body through a deferred helper that
		// receives resp.Body:
		// https://github.com/Notifiarr/notifiarr/blob/63b3c072a1b6df73f676f37b367d75f0299458fc/pkg/services/checks.go#L243-L278
		// New Relic defers a helper that invokes the bound Body.Close callback:
		// https://github.com/newrelic/nri-elasticsearch/blob/9d4f88e2b4293b86dffaa82369dc580493f1b424/src/client.go#L99-L115
		// darkpawns stops a ticker from every exit of a launched select loop:
		// https://github.com/zax0rz/darkpawns/blob/5cdb4679815822a133a051af4c1249ddda800c38/pkg/events/queue.go#L255
		// Herdforge closes a body inside a decoder helper called synchronously:
		// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/provider/github.go#L356
		// ccLoad closes through an immediately invoked literal on an error path:
		// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/bridge_install.go#L264-L289
		completion := lifecycle.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Coverage:    deferredReleaseCoverage(instruction),
			// Completion keeps its larger query allowance while charging the
			// same candidate pool and inheriting its give-up observer. The
			// smaller storage query must not cap this independent question.
			Budget: analysis.budget(releaseSearchBudget),
		}
		proof := evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
			// Imported summaries may close the cleanup-bearing projection of an
			// acquired owner. Require an exact stable access path; IBM AI Services
			// drains response bodies through an exported helper with this shape:
			// https://github.com/IBM/project-ai-services/blob/7f5e30b300819abc2cc8a9307327ca78a145d5cb/ai-services/tests/e2e/catalog_configure_test.go#L66-L70
			// An imported helper summarized as invoking its callback parameter on
			// every return settles a bound cleanup method the same way. Fabric
			// defers utils.IgnoreErrorFunc(rows.Close) throughout its storage code:
			// https://github.com/hyperledger-labs/fabric-smart-client/blob/cb202fc2768b3e72b0197bbaf401b9c2287098e8/platform/view/services/storage/driver/sql/common/binding.go#L71-L75
			StrictImportedProjection: true,
			SelectMask:               releaseMask(instruction, resource, method),
		})
		if action, reason := releaseLabel(proof); action != actionNone {
			return action, reason
		}
	}
	return actionNone, resourceReasonNone
}

func helperRequiresCleanup(
	evidence *lifecyclefacts.LifecycleEvidence,
	storage *heapmodel.Storage,
	instruction ssa.Instruction,
	resource ssa.Value,
	methods []string,
) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.IsInvoke() {
		return false
	}
	for index, argument := range common.Args {
		if !storage.Same(argument, resource).Proven() {
			continue
		}
		for _, required := range evidence.ArgumentMethodsRequired(instruction, index) {
			if slices.Contains(methods, required) {
				return true
			}
		}
	}
	return false
}

// registersCleanupCallback reports whether the call hands a callback that
// releases the resource to a callee whose body is not available and that is
// not summarized as dropping it: the callee keeps the callback and decides
// when it runs, so the release is transferred rather than leaked. A callee
// summarized as not retaining the callback, such as one that invokes it only
// under a condition, leaves the obligation open. gvproxy registers its log
// file's close with logrus's exit handlers:
// https://github.com/containers/gvisor-tap-vsock/blob/d3d4f055ddc59879003e6d9f89912d575b111e66/cmd/gvproxy/config.go#L171-L180
// releaseSearchBudget bounds one "does this callee release the resource?"
// question by the instructions it may examine. Mutually recursive helpers make
// the number of routes through a call graph explode, and an answer the cycle
// guard cuts short cannot be memoized, so an unbounded search re-walks the
// graph once per route. Twenty-two mutually recursive functions with four calls
// each, and one resource held across a single call into them, took over thirty
// seconds before this bound.
const releaseSearchBudget = 250_000

func registersCleanupCallback(evidence *lifecyclefacts.LifecycleEvidence, instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() != nil && len(common.StaticCallee().Blocks) > 0 {
		return false
	}
	for index, argument := range common.Args {
		if _, ok := argument.(*ssa.MakeClosure); !ok {
			continue
		}
		if retained, summarized := evidence.ArgumentRetained(instruction, index); summarized && !retained {
			continue
		}
		for _, method := range methods {
			if lifecycle.ValueCallsMethod(argument, method, resource) {
				return true
			}
		}
	}
	return false
}

// deferredReleaseCoverage asks only whether a deferred callee may release the
// resource. The transaction idiom defers a literal that rolls back unless a
// committed flag was set, so the release is data-dependent and a leak cannot
// be proven; a called or launched callee must still release on every return.
// pad applies migrations this way:
// https://github.com/PerpetualSoftware/pad/blob/ebd1886ada1eca1f0c5ed39f9dc3ad629d0a0cd7/internal/store/store.go#L862-L871
func deferredReleaseCoverage(instruction ssa.Instruction) lifecycle.CompletionCoverage {
	if _, ok := instruction.(*ssa.Defer); ok {
		return lifecycle.CoverageAnywhere
	}
	return lifecycle.CoverageEveryReturn
}

// releaseMask selects the imported summaries that release the resource: the
// method's own mask, plus the Invoked mask only when the call passes a
// callback bound to method on the exact resource.
func releaseMask(instruction ssa.Instruction, resource ssa.Value, method string) func(lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
	return func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
		mask := fact.MethodMask(method)
		if invokesBoundCleanup(instruction, resource, method) {
			mask |= fact.InvokedParameters()
		}
		return mask
	}
}

// invokesBoundCleanup reports whether some argument of the call is a callback
// bound to method on the exact resource, so that invoking the argument is the
// cleanup itself rather than an unrelated callback that merely captured it.
func invokesBoundCleanup(instruction ssa.Instruction, resource ssa.Value, method string) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, argument := range common.Args {
		if lifecycle.ValueCallsMethod(argument, method, resource) {
			return true
		}
	}
	return false
}

func instructionSettlesResourceOwnership(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	resource ssa.Value,
) bool {
	transfer := lifecycle.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       resource,
		Modes: lifecycle.TransferStoredInGlobal | lifecycle.TransferStoredInEnclosingScope |
			lifecycle.TransferOwnerStoredInExternalField | lifecycle.TransferStoredInOwnedMap |
			lifecycle.TransferSentToReceiver | lifecycle.TransferCapturedByClosure,
	}
	return evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction,
		Target:      resource,
		Transfer:    &transfer,
	}).Proven()
}

// resourceReleaseMayFollow asks whether the caller can still claim cleanup
// after this retaining call. Cleanup in a mutually exclusive branch does not
// make this branch's retained writer borrowed. This does not prove coverage:
// an error return before the reachable cleanup is still checked by the flow.
// https://github.com/shijuvar/gokit/blob/4b5abbb8d4e6497a1eef211cb823c18b7977dde4/log/log.go#L59-L88
func resourceReleaseMayFollow(instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	if instruction == nil || instruction.Parent() == nil {
		return false
	}
	for _, block := range instruction.Parent().Blocks {
		for _, candidate := range block.Instrs {
			common := ssaflow.InstructionCall(candidate)
			if common == nil || !slices.Contains(methods, ssaflow.CallName(common)) || !ssaflow.InstructionMayFollow(instruction, candidate) {
				continue
			}
			if heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), resource) {
				return true
			}
		}
	}
	return false
}

func callTakesResourceOwnership(
	evidence *lifecyclefacts.LifecycleEvidence,
	storage *heapmodel.Storage,
	instruction ssa.Instruction,
	resource ssa.Value,
	methods []string,
) bool {
	// A summarized callee that returns a view over the resource keeps the
	// obligation with the caller even when the view's type has a Close of its
	// own: the summary proves that method releases nothing.
	if evidence.ArgumentReturnedAsView(instruction, resource) {
		return false
	}
	// A summarized callee that keeps the resource outside its returned value,
	// such as log.SetOutput installing a file as the process-wide logger
	// sink, owns the release from then on. charmbracelet's examples log to a
	// file this way:
	// https://github.com/charmbracelet/x/blob/6f6ad8b37b0af7e0765bcf38bac6aafaecb9a7d6/examples/cellbuf/main.go#L120-L126
	// Keeping the resource is only a handover when this function does not
	// release it later. png.Encoder.Encode keeps the writer it is given and
	// its summary says so, but a function that closes that writer on its
	// success path is the owner, and the store describes a use rather than a
	// handover; treating it as one settles the resource at the call and hides
	// an error return that never closes it. A function that never releases the
	// resource afterward, as when installing it as the process-wide logger
	// sink, has no such claim and the handover stands.
	// A receiver storing a reference to itself does not transfer its caller's
	// obligation. Rows.Scan, for example, retains receiver-local scan state;
	// that does not make an early Scan-error return close the rows.
	receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(instruction))
	if !storage.Same(receiver, resource).Proven() &&
		evidence.ArgumentRetainedByCallee(instruction, resource) &&
		!resourceReleaseMayFollow(instruction, resource, methods) {
		return true
	}
	transfer := lifecycle.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       resource,
		Modes: lifecycle.TransferCallResultStoredInField | lifecycle.TransferToReceiver |
			lifecycle.TransferToLifecycleOwner | lifecycle.TransferToReturnedOwner,
	}
	return evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction,
		Target:      resource,
		Transfer:    &transfer,
		// A callee that stores the resource in its returned struct transfers
		// it only when that struct's type can release it; a returned view such
		// as a buffered reader leaves the obligation with the caller.
		SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
			return fact.ReturnedOwner() &^ fact.Must.ReturnedView
		},
		ReceiverStore: true,
	}).Proven()
}

// ownedResultContract synthesizes a contract for a call whose callee is
// summarized as returning a struct that owns resource fields and whose result
// type has a method releasing them. The caller then owes that method exactly
// as it owes Close to os.Open; the summaries are proven from the constructor
// and method bodies, so no name enters the decision. A constructor that
// stores a caller's resource, or a type whose methods never release the
// field, produces no contract.
func ownedResultContract(evidence *lifecyclefacts.LifecycleEvidence, call *ssa.Call, settings resourceLifetimeSettings) (resourceContract, bool) {
	callee := call.Common().StaticCallee()
	cleanup, index, ok := evidence.OwnedResult(call)
	if !ok && !catalogCoversPackage(settings, callee) {
		// A constructor may hand the resource back directly rather than in
		// a struct; the summary then names the result position and its type
		// names the cleanup. The catalog stays authoritative for the packages
		// it models: database/sql's Prepare returns a fresh statement by its
		// body, but the catalog deliberately leaves statements to the
		// transaction or database that owns them, and an inferred owner must
		// not reopen that decision.
		cleanup, index, ok = evidence.OwnedDirectResult(call)
	}
	retained := false
	if !ok && !catalogCoversPackage(settings, callee) {
		// A constructor may instead return a wrapper that holds the resource
		// it acquired. No method releases it, so the caller settles the
		// obligation only by keeping, handing over, or returning the wrapper.
		index, ok = evidence.RetainingResult(call)
		retained = ok
	}
	if !ok {
		return resourceContract{}, false
	}
	// The package name only labels the diagnostic; identity was decided by
	// the imported summaries above.
	return resourceContract{
		family:      "owned",
		packagePath: callee.Pkg.Pkg.Name(),
		name:        callee.Name(),
		cleanup:     cleanup,
		result:      index,
		retained:    retained,
	}, true
}

// catalogCoversPackage reports whether any catalog contract names the
// callee's package, in which case the catalog's decisions about that API
// are complete and no inferred acquisition is added beside them.
func catalogCoversPackage(settings resourceLifetimeSettings, callee *ssa.Function) bool {
	if callee == nil || callee.Object() == nil {
		return false
	}
	for _, contract := range settings.catalog {
		if syntax.DeclaredInPackage(callee.Object(), contract.packagePath) {
			return true
		}
	}
	return false
}

// memoryWriterExempt reports whether a compression writer wraps a local
// in-memory buffer. Leaving such a writer unclosed on an error path holds no
// resource outside the function. A writer never closed before its buffer is
// read produces truncated output, which is a data defect rather than a leak
// and is not this check's claim.
func memoryWriterExempt(call *ssa.Call, contract resourceContract) bool {
	if contract.family != "compress" || len(call.Common().Args) == 0 {
		return false
	}
	writerMethods := contract.cleanup
	if len(writerMethods) == 0 {
		return false
	}
	underlying := call.Common().Args[0]
	if inner, ok := ssaflow.UnwrapTransparentValue(
		underlying,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		underlying = inner
	}
	// Standard constructors create the same memory-only buffer as a local
	// allocation. The input slice may be borrowed, but carries no descriptor
	// cleanup obligation. Dynamic factories and mixed external writers remain
	// outside this exemption.
	// https://github.com/apache/pulsar-client-go/blob/1a6d7ac818c9daae9df5c37cb24c0695fabc9eec/pulsar/internal/compression/zlib.go#L39-L52
	if constructor, ok := underlying.(*ssa.Call); ok && constructor.Parent() == call.Parent() {
		return ssaflow.CallMatchesAnySymbol(constructor.Common(),
			syntax.PackageFunction("bytes", "NewBuffer"), syntax.PackageFunction("bytes", "NewBufferString"))
	}
	local, ok := underlying.(*ssa.Alloc)
	if !ok || local.Parent() != call.Parent() {
		return false
	}
	pointer, ok := local.Type().Underlying().(*types.Pointer)
	return ok && (syntax.NamedType(pointer.Elem(), "bytes", "Buffer") || syntax.NamedType(pointer.Elem(), "strings", "Builder"))
}
