// Package resourcelifetime implements the resourcelifetime gohawk analyzer.
package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/analysisutil"
	ssautil "github.com/kojah/gohawk/analysisutil/ssa"
	analysisTrace "github.com/kojah/gohawk/analysisutil/trace"
	"github.com/kojah/gohawk/internal/analyzerbase"
	"github.com/kojah/gohawk/internal/analyzers/ownership/lifecyclefacts"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type resourceContract struct {
	packagePath string
	name        string
	cleanup     []string
	result      int
	consumable  bool
}

type resourceFlowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	active      bool
	released    bool
}

type resourceFlowKey struct {
	block       int
	predecessor int
	index       int
	active      bool
	released    bool
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := resourceLifetimeConfig{contracts: "os,http,sql,time,compress", requireReaderClose: true}
	analyzer := &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
	}
	analyzer.Flags.Var(analyzerbase.NewCommaSeparatedChoice(&config.contracts, "os", "http", "sql", "time", "compress"), "contracts", "comma-separated resource contract families: os,http,sql,time,compress")
	analyzer.Flags.BoolVar(&config.requireReaderClose, "require-reader-close", true, "require gzip and zlib readers to be closed")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runResourceLifetime(pass, config)
	}
	return analyzer
}

type resourceLifetimeConfig struct {
	contracts          string
	requireReaderClose bool
}

type resourceLifetimeSettings struct {
	contracts          map[string]bool
	requireReaderClose bool
}

func runResourceLifetime(pass *analysis.Pass, config resourceLifetimeConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	settings := resourceLifetimeSettings{contracts: analyzerbase.CommaSeparatedSet(config.contracts), requireReaderClose: config.requireReaderClose}
	completeTimers := completeTimerLifecyclePositions(pass)
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := resourceContractFor(call.Common(), settings)
				if !ok {
					continue
				}
				resource := ssautil.CallResult(call, contract.result)
				if resource == nil {
					continue
				}
				leaks := resourceLeaks(pass, call, resource, contract)
				completeTimer := completeTimers[call.Pos()]
				emitResourceDecision(pass, function, call, resource, contract, leaks, completeTimer)
				if leaks && !completeTimer {
					analyzerbase.Reportf(pass, analyzerbase.CheckResourceRelease, call.Pos(), "owned resource from %s.%s is not released on every return path", analysisutil.ShortPackageName(contract.packagePath), contract.name)
				}
			}
		}
	}
	return nil, nil
}

func emitResourceDecision(pass *analysis.Pass, function *ssa.Function, call *ssa.Call, resource ssa.Value, contract resourceContract, leaks, completeTimer bool) {
	check := string(analyzerbase.CheckResourceRelease)
	if !analysisTrace.Enabled("resourcelifetime", check) {
		return
	}
	outcome, reason := analysisTrace.OutcomeAccepted, "release-proven"
	if completeTimer {
		reason = "complete-timer-lifecycle"
	} else if leaks {
		outcome, reason = analysisTrace.OutcomeRejected, "unowned-return"
	}
	details := map[string]string{"acquisition": contract.packagePath + "." + contract.name}
	if resource != nil && resource.Type() != nil {
		details["resource_type"] = resource.Type().String()
	}
	analysisTrace.Emit(pass, analysisTrace.Event{Analyzer: "resourcelifetime", Check: check, Phase: "decision", Reason: reason, Outcome: outcome, Pos: call.Pos(), Function: function.String(), Details: details})
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
	if resourceTransferredToExternalField(instruction, resource) || ssautil.StoresValueInGlobal(instruction, resource) || ssautil.StoresValueInEnclosingScope(instruction, resource) || ssautil.StoresOwnerOfValueInExternalField(instruction, resource) || ssautil.StoresValueInOwnedMap(instruction, resource) || ssautil.SendsValue(instruction, resource) || ssautil.ClosureCapturesValue(instruction, resource) || startedClosureReleasesResource(instruction, resource, methods) || ssautil.CallTransfersValueToField(instruction, resource) || lifecyclefacts.OwnsArgument(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource, func(fact ssautil.LifecycleFact) ssautil.ParameterMask { return fact.ReturnedOwner }) || lifecyclefacts.StoresInEscapingReceiver(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource) || ssautil.CallTransfersArgumentToReceiver(instruction, resource) || ssautil.CallTransfersArgumentToLifecycleOwner(instruction, resource) {
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
		if lifecyclefacts.OwnsArgument(pass, "resourcelifetime", string(analyzerbase.CheckResourceRelease), instruction, resource, func(fact ssautil.LifecycleFact) ssautil.ParameterMask { return fact.MethodMask(method) }) || ssautil.CallCallsMethodOnArgumentOnEveryReturn(instruction, method, resource) {
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

func resourceLeaks(pass *analysis.Pass, call *ssa.Call, resource ssa.Value, contract resourceContract) bool {
	index := ssautil.InstructionIndex(call)
	if index < 0 {
		return false
	}
	errorValue := ssautil.CallResult(call, 1)
	if contract.packagePath == "net/http" && testProvesHTTPError(call, resource, errorValue) {
		return false
	}
	owners := localResourceOwners(call.Parent(), resource)
	queue := []resourceFlowState{{block: call.Block(), index: index + 1, active: true}}
	seen := map[resourceFlowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		predecessor := -1
		if state.predecessor != nil {
			predecessor = state.predecessor.Index
		}
		key := resourceFlowKey{block: state.block.Index, predecessor: predecessor, index: state.index, active: state.active, released: state.released}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs[state.index:] {
			state.released = state.released || releasesResource(pass, instruction, resource, owners, contract.cleanup) || contract.consumable && consumesResource(instruction, resource)
			if ssautil.InstructionTerminatesControlFlow(instruction) {
				state.active = false
				break
			}
			if returned, ok := instruction.(*ssa.Return); ok && state.active && !state.released && !returnedResourceOwner(pass, returned, resource, contract.cleanup) && !ssautil.ReturnedSameAsAny(returned, owners) {
				return true
			}
		}
		for _, successor := range ssautil.FeasibleSuccessors(state.block, state.predecessor) {
			active := state.active
			if success, known := resourceSuccessBranch(state.block, successor, errorValue); known {
				active = active && success
			}
			if present, known := resourcePresenceBranch(state.block, successor, resource); known {
				active = active && present
			}
			queue = append(queue, resourceFlowState{block: successor, predecessor: state.block, active: active, released: state.released})
		}
	}
	return false
}

func returnedResourceOwner(pass *analysis.Pass, returned *ssa.Return, resource ssa.Value, cleanup []string) bool {
	if ssautil.ReturnedValueOwnsValue(returned, resource) {
		return true
	}
	for _, result := range returned.Results {
		if !ssautil.ValueDerivesFrom(result, resource, map[ssa.Value]bool{}) {
			continue
		}
		methods := types.NewMethodSet(result.Type())
		for index := range methods.Len() {
			if slices.Contains(cleanup, methods.At(index).Obj().Name()) {
				if analysisTrace.Enabled("resourcelifetime", string(analyzerbase.CheckResourceRelease)) {
					analysisTrace.Emit(pass, analysisTrace.Event{Analyzer: "resourcelifetime", Check: string(analyzerbase.CheckResourceRelease), Phase: "evidence", Reason: "returned-cleanup-projection", Outcome: analysisTrace.OutcomeAccepted, Pos: returned.Pos(), Function: returned.Parent().String()})
				}
				return true
			}
		}
	}
	return false
}

func testProvesHTTPError(acquisition *ssa.Call, resource, errorValue ssa.Value) bool {
	// Test assertions can prove the owned-response path infeasible even though
	// the assertion package expresses that fact outside the CFG.
	// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/cmd/wfx/cmd/root/root_test.go#L154-L157
	var errorAssertions, nilAssertions []ssa.Instruction
	for _, block := range acquisition.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !ssautil.InstructionMayFollow(acquisition, instruction) {
				continue
			}
			common := ssautil.InstructionCall(instruction)
			if !ssautil.HasLibraryContract(common, ssautil.ContractTestifyAssertion) {
				continue
			}
			if ssautil.CallName(common) == "Error" {
				for _, argument := range common.Args {
					if ssautil.ValueDerivesFrom(argument, errorValue, map[ssa.Value]bool{}) {
						errorAssertions = append(errorAssertions, instruction)
					}
				}
			}
			if ssautil.CallName(common) == "Nil" {
				for _, argument := range common.Args {
					if ssautil.SameValue(argument, resource) {
						nilAssertions = append(nilAssertions, instruction)
					}
				}
			}
		}
	}
	// net/http only returns a non-nil response together with an error for a
	// failed redirect policy, and its body is already closed. A fatal Error
	// assertion therefore eliminates the success path; a paired Nil assertion
	// supplies the same evidence for non-fatal assertion packages.
	for _, assertedError := range errorAssertions {
		if fatalErrorAssertion(assertedError) {
			return true
		}
		for _, assertedNil := range nilAssertions {
			if ssautil.InstructionDominates(assertedError, assertedNil) {
				return true
			}
		}
	}
	return false
}

func fatalErrorAssertion(instruction ssa.Instruction) bool {
	common := ssautil.InstructionCall(instruction)
	return ssautil.HasLibraryContract(common, ssautil.ContractTestifyFatalError)
}

func resourcePresenceBranch(block, successor *ssa.BasicBlock, resource ssa.Value) (bool, bool) {
	if resource == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesResourceToNil := ssautil.ValueDerivesFrom(comparison.X, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.Y) ||
		ssautil.ValueDerivesFrom(comparison.Y, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.X)
	if !comparesResourceToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	// On the nil branch there is no owned value to release. This matters when
	// callers defensively close a response whenever net/http returns one, even
	// on an error path:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/codex_utls_transport_test.go#L305-L319
	if comparison.Op == token.NEQ {
		return trueBranch, true
	}
	return !trueBranch, true
}

func localResourceOwners(function *ssa.Function, resource ssa.Value) []ssa.Value {
	var owners []ssa.Value
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			owner := resourceFieldOwner(instruction, resource)
			if owner != nil && !ssautil.ExternallyOwnedValue(owner) && !ssautil.SameAsAny(owner, owners) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

func resourceTransferredToExternalField(instruction ssa.Instruction, resource ssa.Value) bool {
	owner := resourceFieldOwner(instruction, resource)
	return owner != nil && ssautil.ExternallyOwnedValue(owner)
}

func resourceFieldOwner(instruction ssa.Instruction, resource ssa.Value) ssa.Value { //nolint:ireturn // Owners retain their concrete SSA value forms.
	store, ok := instruction.(*ssa.Store)
	if !ok || !ssautil.ValueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) {
		return nil
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	return field.X
}

func resourceSuccessBranch(block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return false, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return false, false
	}
	// A true errors.Is check against the documented non-nil missing-file
	// sentinel proves that os.Open did not produce an owned file. This commonly
	// appears before the generic err != nil check when callers skip missing
	// inputs. Do not carry a live resource around that continue edge.
	// https://github.com/heymaikol/network-doctor/blob/6d0df6eaba1de237077e0a1f8224fd8d5c3d083a/internal/simulation/evidence.go#L407-L415
	if missingFileCheck(branch.Cond, errorValue) && successor == block.Succs[0] {
		return false, true
	}
	return ssautil.SuccessBranch(block, successor, errorValue)
}

func missingFileCheck(condition, errorValue ssa.Value) bool {
	if errorsIsMissingFile(condition, errorValue) {
		return true
	}
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	// os.IsNotExist is the legacy equivalent of errors.Is(err,
	// fs.ErrNotExist); on its true branch os.Open did not return an owned file.
	// https://github.com/Kampe/Herdforge/blob/198b704aed6a18b68e7eeb50ba8e97d37855f6b2/pkg/feedback/send.go#L124
	return ssautil.CallPackage(common) == "os" && ssautil.CallName(common) == "IsNotExist" && len(common.Args) == 1 &&
		ssautil.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{})
}

func errorsIsMissingFile(condition, errorValue ssa.Value) bool {
	call, ok := condition.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if ssautil.CallPackage(common) != "errors" || ssautil.CallName(common) != "Is" || len(common.Args) != 2 {
		return false
	}
	if !ssautil.ValueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
		return false
	}
	return isMissingFileSentinel(common.Args[1])
}

func isMissingFileSentinel(value ssa.Value) bool {
	for {
		switch typed := value.(type) {
		case *ssa.ChangeInterface:
			value = typed.X
		case *ssa.ChangeType:
			value = typed.X
		case *ssa.Convert:
			value = typed.X
		case *ssa.MakeInterface:
			value = typed.X
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return false
			}
			value = typed.X
		case *ssa.Global:
			if typed.Pkg == nil || typed.Pkg.Pkg == nil || typed.Name() != "ErrNotExist" {
				return false
			}
			packagePath := typed.Pkg.Pkg.Path()
			return packagePath == "os" || packagePath == "io/fs"
		default:
			return false
		}
	}
}

func consumesResource(instruction ssa.Instruction, resource ssa.Value) bool {
	if receive, ok := instruction.(*ssa.UnOp); ok {
		return receive.Op == token.ARROW && ssautil.ValueDerivesFrom(receive.X, resource, map[ssa.Value]bool{})
	}
	selection, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	for _, state := range selection.States {
		if state.Dir == types.RecvOnly && ssautil.ValueDerivesFrom(state.Chan, resource, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func resourceLifecycleMethod(name string) bool {
	switch name {
	case "Close", "Kill", "Shutdown", "Stop", "Wait":
		return true
	default:
		return false
	}
}
