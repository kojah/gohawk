package resourcelifetime

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
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
	consumable  bool
	readerClose bool
}

func resourceContracts() []resourceContract {
	return []resourceContract{
		resourceFunction("os", "os", "Create", 0, "Close"),
		resourceFunction("os", "os", "CreateTemp", 0, "Close"),
		resourceFunction("os", "os", "Open", 0, "Close"),
		resourceFunction("os", "os", "OpenFile", 0, "Close"),
		resourceFunction("time", "time", "NewTicker", -1, "Stop"),
		consumableResource(resourceFunction("time", "time", "NewTimer", -1, "Stop")),

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

		readerResource(resourceFunction("compress", "compress/gzip", "NewReader", 0, "Close")),
		resourceFunction("compress", "compress/gzip", "NewWriterLevel", 0, "Close"),
		resourceFunction("compress", "compress/gzip", "NewWriter", -1, "Close"),
		readerResource(resourceFunction("compress", "compress/zlib", "NewReader", 0, "Close")),
		readerResource(resourceFunction("compress", "compress/zlib", "NewReaderDict", 0, "Close")),
		resourceFunction("compress", "compress/zlib", "NewWriterLevel", 0, "Close"),
		resourceFunction("compress", "compress/zlib", "NewWriterLevelDict", 0, "Close"),
		resourceFunction("compress", "compress/zlib", "NewWriter", -1, "Close"),
	}
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

func consumableResource(contract resourceContract) resourceContract {
	contract.consumable = true
	return contract
}

func readerResource(contract resourceContract) resourceContract {
	contract.readerClose = true
	return contract
}

func resourceContractFor(common *ssa.CallCommon, settings resourceLifetimeSettings) (resourceContract, bool) {
	for _, contract := range settings.catalog {
		if !settings.contracts[contract.family] || contract.readerClose && !settings.requireReaderClose {
			continue
		}
		if ssaflow.CallMatchesSymbol(common, contract.symbol) {
			return contract, true
		}
	}
	return resourceContract{}, false
}

func releasesResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	resource ssa.Value,
	owners []ssa.Value,
	methods []string,
	optionalAcquisition optionalAcquisitionProof,
) bool {
	if optionalAcquisition.Proven() {
		// The optional-acquisition proof deliberately authorizes only cleanup
		// through its exact resource phi. Letting the ordinary existential
		// derivation rules inspect a later phi could mistake cleanup of another
		// non-nil resource for cleanup of the acquired one.
		return optionalAcquisitionReleases(instruction, resource, methods)
	}
	return releasesOrdinaryResource(evidence, instruction, resource, owners, methods)
}

func releasesOrdinaryResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	resource ssa.Value,
	owners []ssa.Value,
	methods []string,
) bool {
	// Installing a resource in package storage transfers cleanup to that
	// package's lifecycle, as in Argus's Init/Close logging pair:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/uxlog/uxlog.go#L21-L39
	if instructionSettlesResourceOwnership(evidence, instruction, resource) ||
		callTakesResourceOwnership(evidence, instruction, resource) ||
		registersCleanupCallback(evidence, instruction, resource, methods) {
		return true
	}
	common := ssaflow.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssaflow.CallName(common)) &&
		ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), resource, map[ssa.Value]bool{}) {
		return true
	}
	if common != nil && slices.Contains(methods, ssaflow.CallName(common)) &&
		storedResourceAccessReleased(evidence, instruction, ssaflow.CallReceiver(common), resource) {
		return true
	}
	if common != nil && resourceLifecycleMethod(ssaflow.CallName(common)) && ssaflow.SameAsAny(ssaflow.CallReceiver(common), owners) {
		return true
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
		if releaseSettled(evidence.Prove(lifecyclefacts.EvidenceRequest{
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
		})) {
			return true
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

func callTakesResourceOwnership(evidence *lifecyclefacts.LifecycleEvidence, instruction ssa.Instruction, resource ssa.Value) bool {
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
	if evidence.ArgumentRetainedByCallee(instruction, resource) {
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

func storedResourceAccessReleased(
	evidence *lifecyclefacts.LifecycleEvidence,
	release ssa.Instruction,
	receiver, resource ssa.Value,
) bool {
	if receiver == nil || release.Parent() == nil {
		return false
	}
	for _, block := range release.Parent().Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || !ssaflow.InstructionDominates(store, release) || !ssaflow.SameValue(store.Val, resource) {
				continue
			}
			field, ok := store.Addr.(*ssa.FieldAddr)
			if ok && evidence.Identity(
				release,
				ssaflow.AccessPath{Value: receiver, Root: field.X},
				ssaflow.AccessPath{Value: field, Root: field.X},
			).Proven() {
				return true
			}
		}
	}
	return false
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
	cleanup, index, ok := evidence.OwnedResult(call)
	if !ok {
		return resourceContract{}, false
	}
	callee := call.Common().StaticCallee()
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
	if len(writerMethods) == 0 || contract.readerClose {
		return false
	}
	underlying := call.Common().Args[0]
	if inner, ok := ssaflow.UnwrapTransparentValue(
		underlying,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		underlying = inner
	}
	local, ok := underlying.(*ssa.Alloc)
	if !ok || local.Parent() != call.Parent() {
		return false
	}
	pointer, ok := local.Type().Underlying().(*types.Pointer)
	return ok && (syntax.NamedType(pointer.Elem(), "bytes", "Buffer") || syntax.NamedType(pointer.Elem(), "strings", "Builder"))
}
