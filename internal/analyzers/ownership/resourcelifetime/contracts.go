package resourcelifetime

import (
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
	if instructionSettlesResourceOwnership(evidence, instruction, resource, methods) ||
		callTakesResourceOwnership(evidence, instruction, resource) {
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
	if common != nil && testingCleanupReleases(evidence, common, resource, methods) {
		return true
	}
	if common != nil && resourceLifecycleMethod(ssaflow.CallName(common)) && ssaflow.SameAsAny(ssaflow.CallReceiver(common), owners) {
		return true
	}
	for _, method := range methods {
		// A deferred static helper may receive the cleanup-bearing projection of
		// an acquired owner rather than the owner itself. Require an exact access
		// path into the helper and cleanup on every normal helper return. Notifiarr
		// drains and closes an HTTP response body with this shape:
		// https://github.com/Notifiarr/notifiarr/blob/63b3c072a1b6df73f676f37b367d75f0299458fc/pkg/services/checks.go#L243-L278
		// A deferred function literal may receive a field projected from the
		// acquired owner and close that exact parameter on every return. Webtor
		// uses this generated shape for response bodies:
		// https://github.com/webtor-io/web-ui/blob/4d541bb73a322597de7f72401fdb3d5e4b04fd9a/services/tmdb/api.go#L428-L449
		completion := ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes: ssaflow.CompletionByDeferredArgument |
				ssaflow.CompletionByDerivedDeferredHelperArgument,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
		}).Proven() {
			return true
		}
		// A deferred helper may own cleanup by receiving a bound lifecycle method
		// and invoking that exact callback on every normal helper return. New
		// Relic uses this to log Body.Close errors without losing defer semantics:
		// https://github.com/newrelic/nri-elasticsearch/blob/9d4f88e2b4293b86dffaa82369dc580493f1b424/src/client.go#L99-L115
		completion = ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes:       ssaflow.CompletionByDeferredHelperCallback,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
			// An imported helper summarized as invoking its callback parameter on
			// every return settles the bound cleanup method the same way. Fabric
			// defers utils.IgnoreErrorFunc(rows.Close) throughout its storage code:
			// https://github.com/hyperledger-labs/fabric-smart-client/blob/cb202fc2768b3e72b0197bbaf401b9c2287098e8/platform/view/services/storage/driver/sql/common/binding.go#L71-L75
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				if !invokesBoundCleanup(instruction, resource, method) {
					return 0
				}
				return fact.Invoked
			},
		}).Proven() {
			return true
		}
		// A launched lifecycle goroutine may take cleanup ownership when each
		// normal exit stops the resource. darkpawns uses this for tickers whose
		// select exits on either context or component shutdown:
		// https://github.com/zax0rz/darkpawns/blob/5cdb4679815822a133a051af4c1249ddda800c38/pkg/events/queue.go#L255
		completion = ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes:       ssaflow.CompletionInStartedClosure,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
		}).Proven() {
			return true
		}
		// Only accept a helper summary when every normal helper return has
		// performed cleanup. Herdforge's response decoder owns Body.Close this
		// way, without advertising ownership in the helper name:
		// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/provider/github.go#L356
		completion = ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes:       ssaflow.CompletionByHelper,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
			// Imported summaries may close the cleanup-bearing projection of an
			// acquired owner. Require an exact stable access path; IBM AI Services
			// drains response bodies through an exported helper with this shape:
			// https://github.com/IBM/project-ai-services/blob/7f5e30b300819abc2cc8a9307327ca78a145d5cb/ai-services/tests/e2e/catalog_configure_test.go#L66-L70
			StrictImportedProjection: true,
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				return fact.MethodMask(method)
			},
		}).Proven() {
			return true
		}
		// A directly invoked cleanup closure can own an individual error path just
		// as a defer owns the return path. Require direct invocation and cleanup
		// before any branch so a stored closure or conditional defer cannot hide a
		// leak. ccLoad uses a direct close while uzomuzo uses a nested defer:
		// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/bridge_install.go#L264-L289
		// https://github.com/future-architect/uzomuzo-oss/blob/0efb5096879fbb45b81a22d5c80e4c62cb722012/internal/infrastructure/golangresolve/resolve.go#L94-L107
		completion = ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes:       ssaflow.CompletionDeferred | ssaflow.CompletionInCalledClosureBeforeBranch,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
		}).Proven() {
			return true
		}
	}
	return false
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
	methods []string,
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
		}).Proven() ||
		startedClosureReleasesResource(evidence, instruction, resource, methods)
}

func callTakesResourceOwnership(evidence *lifecyclefacts.LifecycleEvidence, instruction ssa.Instruction, resource ssa.Value) bool {
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
		SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
			return fact.ReturnedOwner
		},
		ReceiverStore: true,
	}).Proven()
}

func startedClosureReleasesResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	resource ssa.Value,
	methods []string,
) bool {
	completion := ssaflow.CompletionRequest{
		Instruction: instruction,
		Target:      resource,
		Methods:     methods,
		Modes:       ssaflow.CompletionInStartedClosure,
	}
	return evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction,
		Target:      resource,
		Completion:  &completion,
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

func testingCleanupReleases(
	evidence *lifecyclefacts.LifecycleEvidence,
	common *ssa.CallCommon,
	resource ssa.Value,
	methods []string,
) bool {
	if !ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return false
	}
	// testing.TB guarantees that Cleanup callbacks run when the test and its
	// subtests complete. Require an unconditional cleanup call inside the
	// callback rather than treating arbitrary capture as ownership transfer.
	// https://github.com/heymaikol/network-doctor/blob/6d0df6eaba1de237077e0a1f8224fd8d5c3d083a/internal/app/app_test.go#L1298-L1303
	for _, argument := range common.Args {
		instruction, ok := argument.(ssa.Instruction)
		if !ok {
			continue
		}
		completion := ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     methods,
			Modes:       ssaflow.CompletionBeforeBranch,
		}
		if evidence.Prove(lifecyclefacts.EvidenceRequest{
			Instruction: instruction,
			Target:      resource,
			Completion:  &completion,
		}).Proven() {
			return true
		}
	}
	return false
}
