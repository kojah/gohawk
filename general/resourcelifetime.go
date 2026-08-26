package general

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

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
	return &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runResourceLifetime,
	}
}

func runResourceLifetime(pass *analysis.Pass) (any, error) {
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := resourceContractFor(call.Common())
				if !ok {
					continue
				}
				resource := resourceResult(call, contract.result)
				if resource == nil {
					continue
				}
				if resourceLeaks(call, resource, contract) {
					pass.Reportf(call.Pos(), "owned resource from %s.%s is not released on every return path", shortPackage(contract.packagePath), contract.name)
				}
			}
		}
	}
	return nil, nil
}

func resourceContractFor(common *ssa.CallCommon) (resourceContract, bool) {
	packagePath, name := analysisutil.CallPackage(common), analysisutil.CallName(common)
	if packagePath == "os" {
		switch name {
		case "Create", "CreateTemp", "Open", "OpenFile":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		}
	}
	if packagePath == "time" && (name == "NewTicker" || name == "NewTimer") {
		return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Stop"}, result: -1, consumable: name == "NewTimer"}, true
	}
	if packagePath == "database/sql" {
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
	if packagePath == "net/http" {
		switch name {
		case "Get", "Post", "PostForm", "Do":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		}
	}
	if packagePath == "compress/gzip" {
		switch name {
		case "NewReader", "NewWriterLevel":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		case "NewWriter":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: -1}, true
		}
	}
	if packagePath == "compress/zlib" {
		switch name {
		case "NewReader", "NewReaderDict", "NewWriterLevel", "NewWriterLevelDict":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: 0}, true
		case "NewWriter":
			return resourceContract{packagePath: packagePath, name: name, cleanup: []string{"Close"}, result: -1}, true
		}
	}
	return resourceContract{}, false
}

func receiverNamedType(common *ssa.CallCommon, packagePath, name string) bool {
	receiver := analysisutil.CallReceiver(common)
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

func releasesResource(instruction ssa.Instruction, resource ssa.Value, methods []string) bool {
	if analysisutil.StoresValueInField(instruction, resource) || analysisutil.StoresValueInOwnedMap(instruction, resource) || analysisutil.ClosureCapturesValue(instruction, resource) || analysisutil.CallTransfersValueToField(instruction, resource) {
		return true
	}
	common := analysisutil.InstructionCall(instruction)
	if common != nil && slices.Contains(methods, analysisutil.CallName(common)) && valueDerivesFrom(analysisutil.CallReceiver(common), resource, map[ssa.Value]bool{}) {
		return true
	}
	if common != nil {
		name := strings.ToLower(analysisutil.CallName(common))
		if strings.Contains(name, "close") || strings.Contains(name, "release") || strings.Contains(name, "cleanup") {
			for _, argument := range common.Args {
				if valueDerivesFrom(argument, resource, map[ssa.Value]bool{}) {
					return true
				}
			}
		}
	}
	for _, method := range methods {
		if analysisutil.DeferredClosureCalls(instruction, method, resource) {
			return true
		}
	}
	return false
}

func resourceLeaks(call *ssa.Call, resource ssa.Value, contract resourceContract) bool {
	index := analysisutil.InstructionIndex(call)
	if index < 0 {
		return false
	}
	errorValue := resourceResult(call, 1)
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
			state.released = state.released || releasesResource(instruction, resource, contract.cleanup) || contract.consumable && consumesResource(instruction, resource)
			if returned, ok := instruction.(*ssa.Return); ok && state.active && !state.released && !returnedValueAliases(returned, resource) {
				return true
			}
		}
		for _, successor := range analysisutil.FeasibleSuccessors(state.block, state.predecessor) {
			active := state.active
			if success, known := resourceSuccessBranch(state.block, successor, errorValue); known {
				active = active && success
			}
			queue = append(queue, resourceFlowState{block: successor, predecessor: state.block, active: active, released: state.released})
		}
	}
	return false
}

func resourceSuccessBranch(block, successor *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || len(block.Instrs) == 0 || len(block.Succs) != 2 {
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
	comparesErrorToNil := valueDerivesFrom(comparison.X, errorValue, map[ssa.Value]bool{}) && definitelyNil(comparison.Y, map[ssa.Value]bool{}) ||
		valueDerivesFrom(comparison.Y, errorValue, map[ssa.Value]bool{}) && definitelyNil(comparison.X, map[ssa.Value]bool{})
	if !comparesErrorToNil {
		return false, false
	}
	trueBranch := successor == block.Succs[0]
	if comparison.Op == token.EQL {
		return trueBranch, true
	}
	return !trueBranch, true
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
	if analysisutil.AliasesValue(value, source) {
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
		if analysisutil.AliasesValue(result, value) {
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
