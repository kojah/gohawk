package ownership

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/analysisutil/ssa"

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

func resourceLifetimeAnalyzer() *analysis.Analyzer {
	config := resourceLifetimeConfig{contracts: "os,http,sql,time,compress", requireReaderClose: true}
	analyzer := &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.Var(newCommaSeparatedChoice(&config.contracts, "os", "http", "sql", "time", "compress"), "contracts", "comma-separated resource contract families: os,http,sql,time,compress")
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
	settings := resourceLifetimeSettings{contracts: commaSeparatedSet(config.contracts), requireReaderClose: config.requireReaderClose}
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
				resource := resourceResult(call, contract.result)
				if resource == nil {
					continue
				}
				if resourceLeaks(call, resource, contract) && !completeTimers[call.Pos()] {
					reportf(pass, checkResourceRelease, call.Pos(), "owned resource from %s.%s is not released on every return path", shortPackage(contract.packagePath), contract.name)
				}
			}
		}
	}
	return nil, nil
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

func resourceResult(call *ssa.Call, index int) ssa.Value { //nolint:ireturn // SSA call results have several concrete forms.
	if index < 0 {
		return call
	}
	if call.Referrers() == nil {
		return nil
	}
	for _, reference := range *call.Referrers() {
		if extract, ok := reference.(*ssa.Extract); ok && extract.Index == index {
			return extract
		}
	}
	return nil
}

func releasesResource(instruction ssa.Instruction, resource ssa.Value, owners []ssa.Value, methods []string) bool {
	if resourceTransferredToExternalField(instruction, resource) || ssautil.StoresValueInOwnedMap(instruction, resource) || ssautil.ClosureCapturesValue(instruction, resource) || ssautil.CallTransfersValueToField(instruction, resource) {
		return true
	}
	common := ssautil.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, ssautil.CallName(common)) && valueDerivesFrom(ssautil.CallReceiver(common), resource, map[ssa.Value]bool{}) {
		return true
	}
	if common != nil && testingCleanupReleases(common, resource, methods) {
		return true
	}
	if common != nil && lifecycleMethod(ssautil.CallName(common)) && aliasesAny(ssautil.CallReceiver(common), owners) {
		return true
	}
	if common != nil {
		name := strings.ToLower(ssautil.CallName(common))
		if strings.Contains(name, "close") || strings.Contains(name, "release") || strings.Contains(name, "cleanup") {
			for _, argument := range common.Args {
				if valueDerivesFrom(argument, resource, map[ssa.Value]bool{}) {
					return true
				}
			}
		}
	}
	for _, method := range methods {
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

func testingCleanupReleases(common *ssa.CallCommon, resource ssa.Value, methods []string) bool {
	if ssautil.CallPackage(common) != "testing" || ssautil.CallName(common) != "Cleanup" {
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

func resourceLeaks(call *ssa.Call, resource ssa.Value, contract resourceContract) bool {
	index := ssautil.InstructionIndex(call)
	if index < 0 {
		return false
	}
	errorValue := resourceResult(call, 1)
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
			state.released = state.released || releasesResource(instruction, resource, owners, contract.cleanup) || contract.consumable && consumesResource(instruction, resource)
			if returned, ok := instruction.(*ssa.Return); ok && state.active && !state.released && !returnedValueAliases(returned, resource) && !returnedAliasesAny(returned, owners) {
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
	comparesResourceToNil := valueDerivesFrom(comparison.X, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.Y) ||
		valueDerivesFrom(comparison.Y, resource, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.X)
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
			if owner != nil && !ssautil.ExternallyOwnedValue(owner) && !aliasesAny(owner, owners) {
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
	if !ok || !valueDerivesFrom(store.Val, resource, map[ssa.Value]bool{}) {
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
	if errorsIsMissingFile(branch.Cond, errorValue) && successor == block.Succs[0] {
		return false, true
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	comparesErrorToNil := valueDerivesFrom(comparison.X, errorValue, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.Y) ||
		valueDerivesFrom(comparison.Y, errorValue, map[ssa.Value]bool{}) && ssautil.DefinitelyNil(comparison.X)
	if !comparesErrorToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	if comparison.Op == token.EQL {
		return trueBranch, true
	}
	return !trueBranch, true
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
	if !valueDerivesFrom(common.Args[0], errorValue, map[ssa.Value]bool{}) {
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
		return receive.Op == token.ARROW && valueDerivesFrom(receive.X, resource, map[ssa.Value]bool{})
	}
	selection, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	for _, state := range selection.States {
		if state.Dir == types.RecvOnly && valueDerivesFrom(state.Chan, resource, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}

func valueDerivesFrom(value, source ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || source == nil || seen[value] {
		return false
	}
	if ssautil.AliasesValue(value, source) {
		return true
	}
	seen[value] = true
	if load, ok := value.(*ssa.UnOp); ok && load.X.Referrers() != nil {
		for _, reference := range *load.X.Referrers() {
			if store, storeOK := reference.(*ssa.Store); storeOK && valueDerivesFrom(store.Val, source, seen) {
				return true
			}
		}
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return false
	}
	var operands []*ssa.Value
	for _, operand := range instruction.Operands(operands) {
		if operand != nil && valueDerivesFrom(*operand, source, seen) {
			return true
		}
	}
	return false
}

func returnedValueAliases(returned *ssa.Return, value ssa.Value) bool {
	for _, result := range returned.Results {
		if ssautil.AliasesValue(result, value) {
			return true
		}
	}
	return false
}

func shortPackage(packagePath string) string {
	for index := len(packagePath) - 1; index >= 0; index-- {
		if packagePath[index] == '/' {
			return packagePath[index+1:]
		}
	}
	return packagePath
}
