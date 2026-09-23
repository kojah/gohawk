package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Resource contracts are the authoritative acquisition, cleanup, and transfer
// vocabulary for this analyzer. Exact symbols and configured contract families
// are required so similarly named application methods do not imply ownership.

type resourceContract struct {
	symbol      syntax.Symbol
	family      string
	packagePath string
	name        string
	cleanup     []string
	result      int
}

func resourceContracts() []resourceContract {
	return []resourceContract{
		resourceFunction("os", "os", "Create", 0, "Close"),
		resourceFunction("os", "os", "CreateTemp", 0, "Close"),
		resourceFunction("os", "os", "Open", 0, "Close"),
		resourceFunction("os", "os", "OpenFile", 0, "Close"),
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
func sqlRowsExhaustionEdge(block, successor *ssa.BasicBlock, resource ssa.Value) bool {
	if len(block.Instrs) == 0 || len(block.Succs) != 2 || successor != block.Succs[1] {
		return false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	next, ok := branch.Cond.(*ssa.Call)
	return ok && ssaflow.CallMatchesSymbol(next.Common(), syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "database/sql", Receiver: "Rows", Name: "Next",
	})) && ssaflow.NewStorage(nil).Same(ssaflow.CallReceiver(next.Common()), resource).Proven()
}

func resourceFunction(family, packagePath, name string, result int, cleanup ...string) resourceContract {
	return resourceContract{
		symbol: syntax.PackageFunction(packagePath, name), family: family, packagePath: packagePath, name: name, cleanup: cleanup, result: result,
	}
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

// DB.Close retires the connections that own DB-prepared driver statements.
// This is not a claim that the Stmt becomes unusable immediately: outstanding
// connection users can delay release. Rows, transactions, and Conn-prepared
// statements keep their own obligations. Require the captured receiver itself,
// not an existential alias through a mutable cell or a mixed-parent phi.
// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/benchmark_test.go#L377-L407
func closesStatementDatabase(acquisition *ssa.Call, instruction ssa.Instruction) bool {
	if acquisition == nil || !sqlDatabaseCall(acquisition.Common(), "Prepare", "PrepareContext") {
		return false
	}
	switch instruction.(type) {
	case *ssa.Call, *ssa.Defer:
	default:
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	return sqlDatabaseCall(common, "Close") &&
		statementParentIdentity(ssaflow.CallReceiver(common), ssaflow.CallReceiver(acquisition.Common()))
}

// Finishing a transaction cancels the transaction context watched by active
// Rows, including rows from statements prepared on that exact transaction.
// The cancellation may close rows asynchronously, so this establishes an
// opaque parent-owned lifetime, not a synchronous Rows.Close guarantee.
// DB.Close and Stmt.Close do not have this contract for their active rows.
// https://github.com/bluesky-social/indigo/blob/41278964ec8e3253e70d4e919dfb8e34211c543d/carstore/sqlite_store.go#L171-L190
func finishesRowsTransaction(acquisition *ssa.Call, instruction ssa.Instruction) bool {
	if acquisition == nil {
		return false
	}
	switch instruction.(type) {
	case *ssa.Call, *ssa.Defer:
	default:
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if !sqlReceiverCall(common, "Tx", "Commit", "Rollback") {
		return false
	}
	parent := rowsTransaction(acquisition)
	return parent != nil && statementParentIdentity(ssaflow.CallReceiver(common), parent)
}

func rowsTransaction(acquisition *ssa.Call) ssa.Value {
	common := acquisition.Common()
	if sqlReceiverCall(common, "Tx", "Query", "QueryContext") {
		return ssaflow.CallReceiver(common)
	}
	if !sqlReceiverCall(common, "Stmt", "Query", "QueryContext") {
		return nil
	}
	// Require the statement's exact constructor result. A different statement,
	// unresolved merge, or replaced receiver must retain its own obligation.
	statement := ssaflow.NewStorage(nil).Resolve(ssaflow.CallReceiver(common))
	extract, ok := statement.Value.(*ssa.Extract)
	if !statement.Proven() || !ok || extract.Index != 0 {
		return nil
	}
	prepare, ok := extract.Tuple.(*ssa.Call)
	if !ok || !sqlReceiverCall(prepare.Common(), "Tx", "Prepare", "PrepareContext") {
		return nil
	}
	return ssaflow.CallReceiver(prepare.Common())
}

// A later closure capture keeps even an unchanged local in a cell. Two loads
// agree only when the stored values at their respective execution points
// provably agree. Do not equate arbitrary
// loads from the same address: that would accept a reassigned DB.
func statementParentIdentity(left, right ssa.Value) bool {
	return ssaflow.NewStorage(nil).Same(left, right).Proven()
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

// These DB methods obtain a connection through DB.conn, which checks ctx.Done
// before giving the driver any work. Conn, Tx, and Stmt methods do not share
// that entry check. Only an ordinary invocation of the exact paired cancel
// dominating acquisition counts; deadlines, deferred calls, and sleeps do not.
// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L2794-L2803
func acquisitionContextCanceled(acquisition *ssa.Call) bool {
	common := acquisition.Common()
	if !sqlDatabaseCall(common, "PrepareContext", "QueryContext", "BeginTx") || len(common.Args) < 2 {
		return false
	}
	ctx, ok := common.Args[1].(*ssa.Extract)
	if !ok || ctx.Index != 0 {
		return false
	}
	constructor, ok := ctx.Tuple.(*ssa.Call)
	if !ok || !ssaflow.CallMatchesAnySymbol(constructor.Common(),
		syntax.PackageFunction("context", "WithCancel"),
		syntax.PackageFunction("context", "WithCancelCause")) {
		return false
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](acquisition.Parent()) {
		cancel, ok := call.Common().Value.(*ssa.Extract)
		if ok && cancel.Tuple == constructor && cancel.Index == 1 && ssaflow.InstructionDominates(call, acquisition) {
			return true
		}
	}
	return false
}

func resourceContractFor(common *ssa.CallCommon, settings resourceLifetimeSettings) (resourceContract, bool) {
	for _, contract := range settings.catalog {
		if !settings.contracts[contract.family] {
			continue
		}
		if ssaflow.CallMatchesSymbol(common, contract.symbol) {
			return contract, true
		}
	}
	return resourceContract{}, false
}

// releasesResource classifies an instruction as settling the resource, as
// an uncertain release, or as neither. Uncertainty arises when the only
// release a helper performs lies inside a loop: the flow then neither
// credits nor reports it.
func releasesResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	knowledge *summaries.Provider,
	instruction ssa.Instruction,
	resource ssa.Value,
	owners []ssa.Value,
	methods []string,
	optionalAcquisition optionalAcquisitionProof,
) (resourceAction, string) {
	if optionalAcquisition.Proven() {
		// The optional-acquisition proof deliberately authorizes only cleanup
		// through its exact resource phi. Letting the ordinary existential
		// derivation rules inspect a later phi could mistake cleanup of another
		// non-nil resource for cleanup of the acquired one.
		if optionalAcquisitionReleases(instruction, resource, methods) {
			return actionSettled, actionSettled.String()
		}
		return actionNone, ""
	}
	return releasesOrdinaryResource(evidence, knowledge, instruction, resource, owners, methods)
}

// cleanupReceiver is the value a cleanup call acts on, seen through a
// wrapper the result summary proves returns its argument unchanged:
// wrap(file).Close() closes file. The identity is exact and typed, so a
// conversion, a chosen value, or a real wrapper is not seen through.
func cleanupReceiver(knowledge *summaries.Provider, common *ssa.CallCommon) ssa.Value { //nolint:ireturn // SSA values keep their concrete forms.
	receiver := ssaflow.CallReceiver(common)
	if knowledge == nil || receiver == nil {
		return receiver
	}
	if argument, ok := knowledge.ArgumentReturnedUnchanged(receiver, ssaflow.NewSearchBudget(ssaflow.SummaryBudget)); ok {
		return argument
	}
	return receiver
}

func releasesOrdinaryResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	knowledge *summaries.Provider,
	instruction ssa.Instruction,
	resource ssa.Value,
	owners []ssa.Value,
	methods []string,
) (resourceAction, string) {
	settled := func() (resourceAction, string) { return actionSettled, actionSettled.String() }
	// Installing a resource in package storage transfers cleanup to that
	// package's lifecycle, as in Argus's Init/Close logging pair:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/uxlog/uxlog.go#L21-L39
	if instructionSettlesResourceOwnership(evidence, instruction, resource) ||
		callTakesResourceOwnership(evidence, instruction, resource, methods) ||
		registersCleanupCallback(evidence, instruction, resource, methods) {
		return settled()
	}
	common := ssaflow.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssaflow.CallName(common)) &&
		(ssaflow.NewStorage(nil).Same(cleanupReceiver(knowledge, common), resource).Proven() ||
			ssaflow.NewStorage(nil).Projection(ssaflow.CallReceiver(common), resource, instruction).Proven()) {
		return settled()
	}
	if common != nil && resourceLifecycleMethod(ssaflow.CallName(common)) && ssaflow.MayAliasAny(ssaflow.CallReceiver(common), owners) {
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
		completion := ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Coverage:    deferredReleaseCoverage(instruction),
			Budget:      ssaflow.NewSearchBudget(releaseSearchBudget),
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
		if releaseSettled(proof) {
			return settled()
		}
		// The helper's only release lies inside a loop over what it was
		// handed. The completion search declines to call that missing, and
		// this classifier declines to call it a release: unknown. A helper
		// whose release merely depends on a flag has complete path
		// information and stays diagnostic.
		if proof.Reason == ssaflow.EvidenceCompletionInCycle {
			return actionUnknown, "helper-cleanup-in-loop"
		}
	}
	return actionNone, ""
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

// releaseSettled reports whether the analyzer may treat the resource as
// released here: the evidence proved a release, or the search was abandoned
// before it could decide. A leak diagnostic claims the resource is provably
// never released, so an undecided release has to suppress. Leaving the
// obligation open would let a walk the analyzer gave up on produce a
// defect-tier report.
func releaseSettled(proof ssaflow.Proof) bool {
	return proof.Proven() || proof.Reason == ssaflow.EvidenceBudgetExhausted
}

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
			if ssaflow.ValueCallsMethod(argument, method, resource) {
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
func deferredReleaseCoverage(instruction ssa.Instruction) ssaflow.CompletionCoverage {
	if _, ok := instruction.(*ssa.Defer); ok {
		return ssaflow.CoverageAnywhere
	}
	return ssaflow.CoverageEveryReturn
}

// releaseMask selects the imported summaries that release the resource: the
// method's own mask, plus the Invoked mask only when the call passes a
// callback bound to method on the exact resource.
func releaseMask(instruction ssa.Instruction, resource ssa.Value, method string) func(lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
	return func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
		mask := fact.MethodMask(method)
		if invokesBoundCleanup(instruction, resource, method) {
			mask |= fact.Invoked
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
		if ssaflow.ValueCallsMethod(argument, method, resource) {
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
	transfer := ssaflow.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       resource,
		Modes: ssaflow.TransferStoredInGlobal | ssaflow.TransferStoredInEnclosingScope |
			ssaflow.TransferOwnerStoredInExternalField | ssaflow.TransferStoredInOwnedMap |
			ssaflow.TransferSentToReceiver | ssaflow.TransferCapturedByClosure,
	}
	return resourceTransferredToExternalField(instruction, resource) ||
		evidence.Prove(lifecyclefacts.EvidenceRequest{
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
			if ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), resource, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

func callTakesResourceOwnership(
	evidence *lifecyclefacts.LifecycleEvidence,
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
	if !ssaflow.NewStorage(nil).Same(receiver, resource).Proven() &&
		evidence.ArgumentRetainedByCallee(instruction, resource) &&
		!resourceReleaseMayFollow(instruction, resource, methods) {
		return true
	}
	transfer := ssaflow.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       resource,
		Modes: ssaflow.TransferCallResultStoredInField | ssaflow.TransferToReceiver |
			ssaflow.TransferToLifecycleOwner | ssaflow.TransferToReturnedOwner,
	}
	return evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction,
		Target:      resource,
		Transfer:    &transfer,
		// A callee that stores the resource in its returned struct transfers
		// it only when that struct's type can release it; a returned view such
		// as a buffered reader leaves the obligation with the caller.
		SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
			return fact.ReturnedOwner &^ fact.ReturnedView
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
	if !settings.contracts["owned"] {
		return resourceContract{}, false
	}
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
// in-memory buffer and the caller has not asked for those to be checked.
// Leaving such a writer unclosed on an error path loses nothing outside the
// function, so the finding is correct by contract but rarely actionable;
// it is opt-in through -require-memory-writer-close.
func memoryWriterExempt(call *ssa.Call, contract resourceContract, settings resourceLifetimeSettings) bool {
	if settings.requireMemoryWriterClose || contract.family != "compress" || len(call.Common().Args) == 0 {
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
	// outside this opt-out; strict mode above still requires finalization.
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

// httpResponseBodyField recognizes only the standard response's direct Body
// load. Identity and stability of that response are the caller's policy.
func httpResponseBodyField(value ssa.Value) *ssa.FieldAddr {
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !syntax.NamedType(field.X.Type(), "net/http", "Response") {
		return nil
	}
	pointer, ok := field.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok || structure.Field(field.Field).Name() != "Body" {
		return nil
	}
	return field
}
