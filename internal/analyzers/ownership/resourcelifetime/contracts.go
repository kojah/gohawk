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
	evidence *lifecyclefacts.EvidenceQuery,
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
		// A launched lifecycle goroutine may take cleanup ownership when each
		// normal exit stops the resource. darkpawns uses this for tickers whose
		// select exits on either context or component shutdown:
		// https://github.com/zax0rz/darkpawns/blob/5cdb4679815822a133a051af4c1249ddda800c38/pkg/events/queue.go#L255
		completion := ssaflow.CompletionRequest{
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
			SelectMask: func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask {
				return fact.MethodMask(method)
			},
		}).Proven() {
			return true
		}
		// A directly invoked cleanup closure can own an individual error path just
		// as a defer owns the return path. Require the close before any branch in
		// the closure so conditional cleanup cannot hide a leak. ccLoad uses this
		// pattern while constructing verified temporary files:
		// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/bridge_install.go#L264-L289
		completion = ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      resource,
			Methods:     []string{method},
			Modes:       ssaflow.CompletionDeferred | ssaflow.CompletionBeforeBranch,
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

func instructionSettlesResourceOwnership(
	evidence *lifecyclefacts.EvidenceQuery,
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

func callTakesResourceOwnership(evidence *lifecyclefacts.EvidenceQuery, instruction ssa.Instruction, resource ssa.Value) bool {
	transfer := ssaflow.OwnershipTransferRequest{
		Instruction: instruction,
		Value:       resource,
		Modes: ssaflow.TransferCallResultStoredInField | ssaflow.TransferToReceiver |
			ssaflow.TransferToLifecycleOwner,
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
	evidence *lifecyclefacts.EvidenceQuery,
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
	evidence *lifecyclefacts.EvidenceQuery,
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
	evidence *lifecyclefacts.EvidenceQuery,
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
