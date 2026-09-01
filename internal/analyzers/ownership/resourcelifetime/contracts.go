package resourcelifetime

import (
	"slices"

	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type resourceContract struct {
	symbol      analysisutil.Symbol
	family      string
	packagePath string
	name        string
	cleanup     []string
	result      int
	consumable  bool
	readerClose bool
}

var resourceContracts = []resourceContract{
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

func resourceFunction(family, packagePath, name string, result int, cleanup ...string) resourceContract {
	return resourceContract{
		symbol: analysisutil.PackageFunction(packagePath, name), family: family, packagePath: packagePath, name: name, cleanup: cleanup, result: result,
	}
}

func resourceMethod(family, packagePath, receiver, name string, cleanup ...string) resourceContract {
	return resourceContract{
		symbol: analysisutil.PackageMethod(packagePath, receiver, name), family: family, packagePath: packagePath, name: name, cleanup: cleanup, result: 0,
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
	for _, contract := range resourceContracts {
		if !settings.contracts[contract.family] || contract.readerClose && !settings.requireReaderClose {
			continue
		}
		if ssautil.CallMatchesSymbol(common, contract.symbol) {
			return contract, true
		}
	}
	return resourceContract{}, false
}

func releasesResource(pass *analysis.Pass, instruction ssa.Instruction, resource ssa.Value, owners []ssa.Value, methods []string) bool {
	// Installing a resource in package storage transfers cleanup to that
	// package's lifecycle, as in Argus's Init/Close logging pair:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/uxlog/uxlog.go#L21-L39
	if instructionSettlesResourceOwnership(instruction, resource, methods) ||
		callTakesResourceOwnership(pass, instruction, resource) {
		return true
	}
	common := ssautil.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssautil.CallName(common)) &&
		ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), resource, map[ssa.Value]bool{}) {
		return true
	}
	if common != nil && slices.Contains(methods, ssautil.CallName(common)) &&
		storedResourceAccessReleased(instruction, ssautil.CallReceiver(common), resource) {
		return true
	}
	if common != nil && testingCleanupReleases(common, resource, methods) {
		return true
	}
	if common != nil && resourceLifecycleMethod(ssautil.CallName(common)) && ssautil.SameAsAny(ssautil.CallReceiver(common), owners) {
		return true
	}
	for _, method := range methods {
		// A launched lifecycle goroutine may take cleanup ownership when each
		// normal exit stops the resource. darkpawns uses this for tickers whose
		// select exits on either context or component shutdown:
		// https://github.com/zax0rz/darkpawns/blob/5cdb4679815822a133a051af4c1249ddda800c38/pkg/events/queue.go#L255
		if ssautil.StartedClosureCallsMethodOnEveryReturn(instruction, method, resource) {
			return true
		}
		// Only accept a helper summary when every normal helper return has
		// performed cleanup. Herdforge's response decoder owns Body.Close this
		// way, without advertising ownership in the helper name:
		// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/provider/github.go#L356
		if lifecyclefacts.OwnsArgument(
			pass,
			"resourcelifetime",
			string(check.ResourceRelease),
			instruction,
			resource,
			func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.MethodMask(method) },
		) ||
			ssautil.CallCallsMethodOnArgumentOnEveryReturn(instruction, method, resource) {
			return true
		}
		// A directly invoked cleanup closure can own an individual error path just
		// as a defer owns the return path. Require the close before any branch in
		// the closure so conditional cleanup cannot hide a leak. ccLoad uses this
		// pattern while constructing verified temporary files:
		// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/bridge_install.go#L264-L289
		if ssautil.DeferredClosureCalls(instruction, method, resource) || ssautil.ClosureCallsMethodBeforeBranch(instruction, method, resource) {
			return true
		}
	}
	return false
}

func instructionSettlesResourceOwnership(instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	return resourceTransferredToExternalField(instruction, resource) ||
		ssautil.StoresValueInGlobal(instruction, resource) ||
		ssautil.StoresValueInEnclosingScope(instruction, resource) ||
		ssautil.StoresOwnerOfValueInExternalField(instruction, resource) ||
		ssautil.StoresValueInOwnedMap(instruction, resource) ||
		ssautil.SendsValue(instruction, resource) ||
		ssautil.ClosureCapturesValue(instruction, resource) ||
		startedClosureReleasesResource(instruction, resource, methods)
}

func callTakesResourceOwnership(pass *analysis.Pass, instruction ssa.Instruction, resource ssa.Value) bool {
	return ssautil.CallTransfersValueToField(instruction, resource) ||
		lifecyclefacts.OwnsArgument(
			pass,
			"resourcelifetime",
			string(check.ResourceRelease),
			instruction,
			resource,
			func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.ReturnedOwner },
		) ||
		lifecyclefacts.StoresInEscapingReceiver(
			pass,
			"resourcelifetime",
			string(check.ResourceRelease),
			instruction,
			resource,
		) ||
		ssautil.CallTransfersArgumentToReceiver(instruction, resource) ||
		ssautil.CallTransfersArgumentToLifecycleOwner(instruction, resource)
}

func startedClosureReleasesResource(instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	for _, method := range methods {
		if ssautil.StartedClosureCallsMethodOnEveryReturn(instruction, method, resource) {
			return true
		}
	}
	return false
}

func storedResourceAccessReleased(release ssa.Instruction, receiver, resource ssa.Value) bool {
	if receiver == nil || release.Parent() == nil {
		return false
	}
	for _, block := range release.Parent().Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || !ssautil.InstructionDominates(store, release) || !ssautil.SameValue(store.Val, resource) {
				continue
			}
			field, ok := store.Addr.(*ssa.FieldAddr)
			if ok && ssautil.SameAccessPath(receiver, field.X, field, field.X) {
				return true
			}
		}
	}
	return false
}

func testingCleanupReleases(common *ssa.CallCommon, resource ssa.Value, methods []string) bool {
	if !ssautil.HasLibraryContract(common, ssautil.ContractTestingCleanup) {
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
		for _, method := range methods {
			if ssautil.ClosureCallsMethodBeforeBranch(instruction, method, resource) {
				return true
			}
		}
	}
	return false
}
