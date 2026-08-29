package analysisutil

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type flowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	owned       bool
}

type flowKey struct {
	block       int
	predecessor int
	index       int
	owned       bool
}

// SourceSSAFunctions returns non-generated source functions from buildssa results.
func SourceSSAFunctions(pass *analysis.Pass) ([]*ssa.Function, error) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok {
		return nil, fmt.Errorf("buildssa prerequisite returned unexpected result")
	}
	functions := make([]*ssa.Function, 0, len(result.SrcFuncs))
	for _, function := range result.SrcFuncs {
		file := FunctionFile(pass, function)
		if function.Syntax() == nil || file == nil || !AnalyzeFile(pass, file) {
			continue
		}
		functions = append(functions, function)
	}
	return functions, nil
}

// InstructionCall returns call metadata carried by call-like SSA instructions.
func InstructionCall(instruction ssa.Instruction) *ssa.CallCommon {
	switch typed := instruction.(type) {
	case *ssa.Call:
		return typed.Common()
	case *ssa.Defer:
		return typed.Common()
	case *ssa.Go:
		return typed.Common()
	default:
		return nil
	}
}

// CallName returns statically known method, function, or builtin name.
func CallName(common *ssa.CallCommon) string {
	if common == nil {
		return ""
	}
	if common.Method != nil {
		return common.Method.Name()
	}
	if callee := common.StaticCallee(); callee != nil {
		return callee.Name()
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name()
	}
	return ""
}

// CallPackage returns statically known package path for a call.
func CallPackage(common *ssa.CallCommon) string {
	if common == nil {
		return ""
	}
	if common.Method != nil && common.Method.Pkg() != nil {
		return common.Method.Pkg().Path()
	}
	callee := common.StaticCallee()
	if callee == nil || callee.Pkg == nil || callee.Pkg.Pkg == nil {
		return ""
	}
	return callee.Pkg.Pkg.Path()
}

// CallReceiver returns receiver value for method calls and invocations.
func CallReceiver(common *ssa.CallCommon) ssa.Value { //nolint:ireturn // Call receivers have several concrete SSA value forms.
	if common == nil {
		return nil
	}
	if common.IsInvoke() {
		return common.Value
	}
	if len(common.Args) == 0 || common.Signature() == nil || common.Signature().Recv() == nil {
		return nil
	}
	return common.Args[0]
}

// CapturedBindingValue recovers value stored through an addressable closure binding.
func CapturedBindingValue(binding ssa.Value) ssa.Value { //nolint:ireturn // Stored captures may contain any SSA value implementation.
	if binding.Referrers() == nil {
		return binding
	}
	for _, reference := range *binding.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == binding {
			return store.Val
		}
	}
	return binding
}

// AliasesValue reports conservative SSA identity through common conversion and storage forms.
func AliasesValue(value, target ssa.Value) bool {
	// SSA removes ordinary assignments, but captured locals, embedded fields,
	// and interface conversions still need explicit identity recovery.
	return aliasesValueSeen(value, target, map[ssa.Value]bool{}) || aliasesValueSeen(target, value, map[ssa.Value]bool{})
}

// DeferredClosureCalls reports whether deferred closure calls method on target.
func DeferredClosureCalls(instruction ssa.Instruction, method string, target ssa.Value) bool {
	// defer commonly closes over an addressable local. Map closure free variable
	// back to stored outer value before comparing ownership target.
	deferred, ok := instruction.(*ssa.Defer)
	if !ok {
		return false
	}
	closure, ok := deferred.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			common := InstructionCall(candidate)
			if CallName(common) != method {
				continue
			}
			receiver := CallReceiver(common)
			for index, free := range function.FreeVars {
				if AliasesValue(receiver, free) && index < len(closure.Bindings) && AliasesValue(CapturedBindingValue(closure.Bindings[index]), target) {
					return true
				}
			}
		}
	}
	return false
}

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// ClosureCallsValue reports whether a call-like closure or created callback calls target.
func ClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	var closure *ssa.MakeClosure
	if created, ok := instruction.(*ssa.MakeClosure); ok {
		if created.Referrers() == nil || len(*created.Referrers()) == 0 {
			return false
		}
		closure = created
	} else if common := InstructionCall(instruction); common != nil {
		closure, _ = common.Value.(*ssa.MakeClosure)
	}
	if closure == nil {
		return false
	}
	return closureCallsValue(closure, target)
}

// ClosureOwnsValue reports whether a started closure captures value.
func ClosureOwnsValue(instruction ssa.Instruction, value ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if AliasesValue(CapturedBindingValue(binding), value) {
			return true
		}
	}
	return false
}

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			common := InstructionCall(candidate)
			if common == nil {
				continue
			}
			for index, free := range function.FreeVars {
				if AliasesValue(common.Value, free) && index < len(closure.Bindings) && AliasesValue(CapturedBindingValue(closure.Bindings[index]), target) {
					return true
				}
			}
		}
	}
	return false
}

// StoresValueInField reports whether instruction transfers value into a struct field.
func StoresValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !AliasesValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FieldAddr)
	return ok
}

// StoresValueInOwnedMap reports whether instruction transfers value into a
// map that belongs to a caller, receiver, closure, or package owner.
func StoresValueInOwnedMap(instruction ssa.Instruction, value ssa.Value) bool {
	update, ok := instruction.(*ssa.MapUpdate)
	return ok && AliasesValue(update.Value, value) && ExternallyOwnedValue(update.Map)
}

// ExternallyOwnedValue reports whether value comes from storage that outlives
// the current function invocation.
func ExternallyOwnedValue(value ssa.Value) bool {
	return externallyOwnedValue(value, map[ssa.Value]bool{})
}

func externallyOwnedValue(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		return true
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && externallyOwnedValue(store.Val, seen) {
					return true
				}
			}
		}
	case *ssa.FieldAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Field:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.IndexAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Index:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.UnOp:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Lookup:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeType:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Convert:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.MakeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if externallyOwnedValue(edge, seen) {
				return true
			}
		}
	}
	return false
}

// ClosureCapturesValue reports whether instruction creates a closure that owns value.
func ClosureCapturesValue(instruction ssa.Instruction, value ssa.Value) bool {
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok || !valueTransferred(closure, map[ssa.Value]bool{}) {
		return false
	}
	for _, binding := range closure.Bindings {
		if AliasesValue(CapturedBindingValue(binding), value) {
			return true
		}
	}
	return false
}

func valueTransferred(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] || value.Referrers() == nil {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Return:
			return true
		case *ssa.Store:
			if _, ok := typed.Addr.(*ssa.FieldAddr); ok {
				return true
			}
		case *ssa.ChangeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.ChangeType:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Convert:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Extract:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.MakeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Phi:
			if valueTransferred(typed, seen) {
				return true
			}
		}
	}
	return false
}

// CallTransfersValueToField reports whether a call consumes value and stores
// its result in a struct field, transferring cleanup to the receiving owner.
func CallTransfersValueToField(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	usesValue := false
	for _, argument := range call.Common().Args {
		usesValue = usesValue || AliasesValue(argument, value)
	}
	return usesValue && valueTransferred(call, map[ssa.Value]bool{})
}

// ClosureCallsMethod reports whether a call-like closure calls method on target.
func ClosureCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	closure, ok := common.Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			if CallName(called) != method {
				continue
			}
			receiver := CallReceiver(called)
			for index, free := range function.FreeVars {
				if AliasesValue(receiver, free) && index < len(closure.Bindings) && AliasesValue(CapturedBindingValue(closure.Bindings[index]), target) {
					return true
				}
			}
		}
	}
	return false
}

func aliasesValueSeen(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || target == nil || seen[value] {
		return false
	}
	if value == target {
		return true
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.ChangeType:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.Convert:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.MakeInterface:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.FieldAddr:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.IndexAddr:
		return aliasesValueSeen(typed.X, target, seen)
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return false
		}
		if other, ok := target.(*ssa.UnOp); ok && other.Op == token.MUL && aliasesValueSeen(typed.X, other.X, seen) {
			return true
		}
		if aliasesValueSeen(typed.X, target, seen) {
			return true
		}
		return storedValueAliases(typed.X, target, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if aliasesValueSeen(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

func storedValueAliases(address, target ssa.Value, seen map[ssa.Value]bool) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if typed.Addr == address && aliasesValueSeen(typed.Val, target, seen) {
				return true
			}
		case *ssa.FieldAddr:
			if storedValueAliases(typed, target, seen) {
				return true
			}
		case *ssa.IndexAddr:
			if storedValueAliases(typed, target, seen) {
				return true
			}
		}
	}
	return false
}

// InstructionIndex returns instruction position within its basic block.
func InstructionIndex(instruction ssa.Instruction) int {
	for index, candidate := range instruction.Block().Instrs {
		if candidate == instruction {
			return index
		}
	}
	return -1
}

// UnownedReturn reports whether any normal return reachable after start lacks
// an ownership action. Tracking owned state through CFG makes conditional
// cleanup visible without pretending infeasible branches are impossible.
func UnownedReturn(
	start ssa.Instruction,
	owns func(ssa.Instruction) bool,
	allowReturn func(*ssa.Return) bool,
) bool {
	index := InstructionIndex(start)
	if index < 0 {
		return false
	}
	queue := []flowState{{block: start.Block(), index: index + 1}}
	seen := map[flowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		predecessor := -1
		if state.predecessor != nil {
			predecessor = state.predecessor.Index
		}
		key := flowKey{block: state.block.Index, predecessor: predecessor, index: state.index, owned: state.owned}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs[state.index:] {
			state.owned = state.owned || owns(instruction)
			returned, ok := instruction.(*ssa.Return)
			if ok && !state.owned && (allowReturn == nil || !allowReturn(returned)) {
				return true
			}
		}
		for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// UnownedReturnFromEntry reports whether any normal return lacks an ownership action.
func UnownedReturnFromEntry(function *ssa.Function, owns func(ssa.Instruction) bool) bool {
	if len(function.Blocks) == 0 {
		return false
	}
	queue := []flowState{{block: function.Blocks[0]}}
	seen := map[flowKey]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		predecessor := -1
		if state.predecessor != nil {
			predecessor = state.predecessor.Index
		}
		key := flowKey{block: state.block.Index, predecessor: predecessor, owned: state.owned}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs {
			state.owned = state.owned || owns(instruction)
			if _, ok := instruction.(*ssa.Return); ok && !state.owned {
				return true
			}
		}
		for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
			queue = append(queue, flowState{block: successor, predecessor: state.block, owned: state.owned})
		}
	}
	return false
}

// FeasibleSuccessors preserves constants selected by predecessor-sensitive
// phis. This prevents impossible first-iteration loop exits from faking leaks.
func FeasibleSuccessors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	if len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return block.Succs
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return block.Succs
	}
	value, known := branchBool(branch.Cond, block, predecessor)
	if !known {
		return block.Succs
	}
	if value {
		return block.Succs[:1]
	}
	return block.Succs[1:]
}

func branchBool(value ssa.Value, block, predecessor *ssa.BasicBlock) (bool, bool) {
	if literal, ok := value.(*ssa.Const); ok && literal.Value != nil && literal.Value.Kind() == constant.Bool {
		return constant.BoolVal(literal.Value), true
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || phi.Block() != block || predecessor == nil {
		return false, false
	}
	for index, candidate := range block.Preds {
		if candidate == predecessor && index < len(phi.Edges) {
			return branchBool(phi.Edges[index], block, nil)
		}
	}
	return false, false
}

// ReachableReturns returns normal returns reachable after start.
func ReachableReturns(start ssa.Instruction) []*ssa.Return {
	index := InstructionIndex(start)
	if index < 0 {
		return nil
	}
	queue := []flowState{{block: start.Block(), index: index + 1}}
	seen := map[flowKey]bool{}
	var returns []*ssa.Return
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := flowKey{block: state.block.Index, index: state.index}
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, instruction := range state.block.Instrs[state.index:] {
			if returned, ok := instruction.(*ssa.Return); ok {
				returns = append(returns, returned)
			}
		}
		for _, successor := range state.block.Succs {
			queue = append(queue, flowState{block: successor})
		}
	}
	return returns
}

// ValueSources returns error-bearing SSA values contributing to value.
func ValueSources(value ssa.Value) map[ssa.Value]bool {
	sources := map[ssa.Value]bool{}
	collectValueSources(value, sources, map[ssa.Value]bool{})
	return sources
}

func collectValueSources(value ssa.Value, sources, seen map[ssa.Value]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	if IsErrorType(value.Type()) {
		sources[value] = true
	}
	switch typed := value.(type) {
	case *ssa.Call:
		for _, argument := range typed.Common().Args {
			collectValueSources(argument, sources, seen)
		}
		return
	case *ssa.Parameter, *ssa.FreeVar:
		return
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil {
			collectValueSources(*operand, sources, seen)
		}
	}
	collectStoredSources(value, sources, seen, map[ssa.Value]bool{})
}

func collectStoredSources(address ssa.Value, sources, seen, memorySeen map[ssa.Value]bool) {
	// Variadic logging and wrapping calls lower arguments into temporary arrays.
	// Following stores recovers original error value instead of losing identity.
	if address == nil || memorySeen[address] || address.Referrers() == nil {
		return
	}
	memorySeen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			collectValueSources(typed.Val, sources, seen)
		case *ssa.FieldAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		case *ssa.IndexAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		}
	}
}

// ValuesShareErrorSource reports whether values derive from one error-bearing SSA value.
func ValuesShareErrorSource(left, right ssa.Value) bool {
	leftSources := ValueSources(left)
	for source := range ValueSources(right) {
		if leftSources[source] {
			return true
		}
	}
	return false
}

// FunctionFile returns source file containing function.
func FunctionFile(pass *analysis.Pass, function *ssa.Function) *ast.File {
	for _, file := range pass.Files {
		if file.Pos() <= function.Pos() && function.Pos() <= file.End() {
			return file
		}
	}
	return nil
}

// ChannelType reports whether value has channel type.
func ChannelType(value ssa.Value) bool {
	if value == nil {
		return false
	}
	return channelTypeForType(value.Type())
}

func channelTypeForType(value types.Type) bool {
	_, ok := value.Underlying().(*types.Chan)
	return ok
}
