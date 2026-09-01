package resourcelifetime

import (
	"slices"

	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type resourceContract struct {
	packagePath string
	name        string
	cleanup     []string
	result      int
	consumable  bool
}

func resourceContractFor(common *ssa.CallCommon, settings resourceLifetimeSettings) (resourceContract, bool) {
	packagePath, name := ssautil.CallPackage(common), ssautil.CallName(common)
	if settings.contracts["os"] && packagePath == "os" {
		switch name {
		case "Create", "CreateTemp", "Open", "OpenFile":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		}
	}
	if settings.contracts["time"] && packagePath == "time" && (name == "NewTicker" || name == "NewTimer") {
		return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Stop"}, result: -1, consumable: name == "NewTimer"}, true
	}
	if settings.contracts["sql"] && packagePath == "database/sql" {
		switch name {
		case "Begin", "BeginTx":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Commit", "Rollback"}, result: 0}, true
		case "Query", "QueryContext":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		case "Prepare", "PrepareContext":
			// Statements prepared on a transaction are closed automatically when
			// that transaction commits or rolls back.
			if !receiverNamedType(common, packagePath, "Tx") {
				return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
			}
		}
	}
	if settings.contracts["http"] && packagePath == "net/http" {
		switch name {
		case "Get", "Post", "PostForm", "Do":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		}
	}
	if settings.contracts["compress"] && packagePath == "compress/gzip" {
		switch name {
		case "NewReader":
			if settings.requireReaderClose {
				return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
			}
		case "NewWriterLevel":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		case "NewWriter":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: -1}, true
		}
	}
	if settings.contracts["compress"] && packagePath == "compress/zlib" {
		switch name {
		case "NewReader", "NewReaderDict":
			if settings.requireReaderClose {
				return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
			}
		case "NewWriterLevel", "NewWriterLevelDict":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		case "NewWriter":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: -1}, true
		}
	}
	return resourceContract{}, false
}

func receiverNamedType(common *ssa.CallCommon, packagePath, name string) bool {
	receiver := ssautil.CallReceiver(common)
	return receiver != nil && analysisutil.NamedType(receiver.Type(), packagePath, name)
}

func releasesResource(pass *analysis.Pass, instruction ssa.Instruction, resource ssa.Value, owners []ssa.Value, methods []string) bool {
	// Installing a resource in package storage transfers cleanup to that
	// package's lifecycle, as in Argus's Init/Close logging pair:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/uxlog/uxlog.go#L21-L39
	if resourceTransferredToExternalField(instruction, resource) || ssautil.StoresValueInGlobal(instruction, resource) || ssautil.StoresValueInEnclosingScope(instruction, resource) || ssautil.StoresOwnerOfValueInExternalField(instruction, resource) || ssautil.StoresValueInOwnedMap(instruction, resource) || ssautil.SendsValue(instruction, resource) || ssautil.ClosureCapturesValue(instruction, resource) || startedClosureReleasesResource(instruction, resource, methods) || ssautil.CallTransfersValueToField(instruction, resource) || lifecyclefacts.OwnsArgument(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource, func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.ReturnedOwner }) || lifecyclefacts.StoresInEscapingReceiver(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource) || ssautil.CallTransfersArgumentToReceiver(instruction, resource) || ssautil.CallTransfersArgumentToLifecycleOwner(instruction, resource) {
		return true
	}
	common := ssautil.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssautil.CallName(common)) && ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), resource, map[ssa.Value]bool{}) {
		return true
	}
	if common != nil && slices.Contains(methods, ssautil.CallName(common)) && storedResourceAccessReleased(instruction, ssautil.CallReceiver(common), resource) {
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
		if lifecyclefacts.OwnsArgument(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource, func(fact lifecyclefacts.Fact) lifecyclefacts.ParameterMask { return fact.MethodMask(method) }) || ssautil.CallCallsMethodOnArgumentOnEveryReturn(instruction, method, resource) {
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
