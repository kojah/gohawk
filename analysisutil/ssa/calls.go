package ssautil

import (
	"errors"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// SourceSSAFunctions returns non-generated source functions from buildssa results.
func SourceSSAFunctions(pass *analysis.Pass) ([]*ssa.Function, error) {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok {
		return nil, errors.New("buildssa prerequisite returned unexpected result")
	}
	functions := make([]*ssa.Function, 0, len(result.SrcFuncs))
	for _, function := range result.SrcFuncs {
		file := FunctionFile(pass, function)
		if function.Syntax() == nil || file == nil || !analysisutil.AnalyzeFile(pass, file) {
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
	if callee == nil {
		return ""
	}
	if object := callee.Object(); object != nil && object.Pkg() != nil {
		return object.Pkg().Path()
	}
	if callee.Pkg == nil || callee.Pkg.Pkg == nil {
		return ""
	}
	return callee.Pkg.Pkg.Path()
}

// InstructionTerminatesControlFlow reports calls whose documented behavior
// prevents execution from continuing in the current goroutine.
func InstructionTerminatesControlFlow(instruction ssa.Instruction) bool {
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	name := CallName(common)
	if CallPackage(common) == "runtime" && name == "Goexit" {
		return true
	}
	if CallPackage(common) != "testing" {
		return false
	}
	switch name {
	case "FailNow", "Fatal", "Fatalf", "Skip", "Skipf", "SkipNow":
		return true
	default:
		return false
	}
}

// CallInvokesArgumentOnEveryReturn reports whether a statically known helper
// invokes target on every normal path through the helper.
func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	return callInvokesArgumentOnEveryReturn(instruction, target, map[*ssa.Function]bool{})
}

func callInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, seen map[*ssa.Function]bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || seen[common.StaticCallee()] {
		return false
	}
	seen[common.StaticCallee()] = true
	defer delete(seen, common.StaticCallee())
	return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
		common := InstructionCall(candidate)
		return common != nil && AliasesValue(common.Value, parameter) || callInvokesArgumentOnEveryReturn(candidate, parameter, seen)
	})
}

// CallCallsMethodOnArgumentOnEveryReturn reports whether a statically known
// helper calls method on target on every normal path through the helper.
func CallCallsMethodOnArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return callCallsMethodOnArgumentOnEveryReturn(instruction, method, target, map[*ssa.Function]bool{})
}

func callCallsMethodOnArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value, seen map[*ssa.Function]bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || seen[common.StaticCallee()] {
		return false
	}
	seen[common.StaticCallee()] = true
	defer delete(seen, common.StaticCallee())
	return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
		common := InstructionCall(candidate)
		return common != nil && CallName(common) == method && ValueDerivesFrom(CallReceiver(common), parameter, map[ssa.Value]bool{}) ||
			callCallsMethodOnArgumentOnEveryReturn(candidate, method, parameter, seen)
	})
}

func callOwnsArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, owns func(ssa.Instruction, ssa.Value) bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if len(callee.Blocks) == 0 {
		return false
	}
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !AliasesValue(argument, target) && !ValueOwnsValue(argument, target) {
			continue
		}
		parameter := callee.Params[index]
		if !UnownedReturnFromEntryAssumingNonNil(callee, parameter, func(candidate ssa.Instruction) bool {
			return owns(candidate, parameter)
		}) {
			return true
		}
	}
	return false
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
	if pointer, ok := binding.Type().Underlying().(*types.Pointer); ok {
		if _, structured := pointer.Elem().Underlying().(*types.Struct); structured {
			// A captured struct local is represented by its address. Its stores
			// initialize or mutate the value; they do not replace its identity.
			return binding
		}
	}
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

// CapturedBindingAliases reports whether a closure binding directly contains
// target or refers to an addressable local that has contained target. Unlike
// CapturedBindingValue, it handles variables reassigned before a callback is
// installed without depending on referrer iteration order.
func CapturedBindingAliases(binding, target ssa.Value) bool {
	if AliasesValue(binding, target) {
		return true
	}
	if binding == nil || binding.Referrers() == nil {
		return false
	}
	for _, reference := range *binding.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == binding && AliasesValue(store.Val, target) {
			return true
		}
	}
	return false
}

// AliasesValue reports conservative SSA identity through common conversion and storage forms.
func AliasesValue(value, target ssa.Value) bool {
	// SSA removes ordinary assignments, but captured locals, embedded fields,
	// and interface conversions still need explicit identity recovery.
	return aliasesValueSeen(value, target, map[ssa.Value]bool{}) || aliasesValueSeen(target, value, map[ssa.Value]bool{})
}

// DefinitelyNil reports whether every represented SSA value is nil.
func DefinitelyNil(value ssa.Value) bool {
	return definitelyNil(value, map[ssa.Value]bool{})
}

func definitelyNil(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if literal, ok := value.(*ssa.Const); ok {
		return literal.IsNil()
	}
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return definitelyNil(typed.X, seen)
	case *ssa.ChangeType:
		return definitelyNil(typed.X, seen)
	case *ssa.Convert:
		return definitelyNil(typed.X, seen)
	case *ssa.MakeInterface:
		return definitelyNil(typed.X, seen)
	case *ssa.Phi:
		if len(typed.Edges) == 0 {
			return false
		}
		for _, edge := range typed.Edges {
			if !definitelyNil(edge, seen) {
				return false
			}
		}
		return true
	}
	return false
}

// DeferredClosureCalls reports whether deferred closure calls method on target.
func DeferredClosureCalls(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsMethod(instruction, method, target)
}

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred
// closure delegates target to a helper that invokes it on every normal path.
func DeferredClosureInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for index, free := range function.FreeVars {
				if index < len(closure.Bindings) && CapturedBindingAliases(closure.Bindings[index], target) && CallInvokesArgumentOnEveryReturn(candidate, free) {
					return true
				}
			}
			for index, parameter := range function.Params {
				if common != nil && index < len(common.Args) && AliasesValue(common.Args[index], target) && CallInvokesArgumentOnEveryReturn(candidate, parameter) {
					return true
				}
			}
		}
	}
	return false
}

// DeferredClosurePassesValueToNamedCall reports whether a deferred closure
// passes target to a call whose name contains one of fragments.
func DeferredClosurePassesValueToNamedCall(instruction ssa.Instruction, target ssa.Value, fragments ...string) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			name := strings.ToLower(CallName(called))
			if called == nil || !slices.ContainsFunc(fragments, func(fragment string) bool {
				return strings.Contains(name, fragment)
			}) {
				continue
			}
			for _, argument := range called.Args {
				for index, free := range function.FreeVars {
					if index < len(closure.Bindings) && ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) && CapturedBindingAliases(closure.Bindings[index], target) {
						return true
					}
				}
				for index, parameter := range function.Params {
					if common != nil && index < len(common.Args) && ValueDerivesFrom(argument, parameter, map[ssa.Value]bool{}) && AliasesValue(common.Args[index], target) {
						return true
					}
				}
			}
		}
	}
	return false
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

// ValueCallsValue reports whether value is, or wraps, a callback that invokes
// target. It follows common callback wrappers and addressable locals so callers
// can recognize cleanup registered through higher-order APIs.
func ValueCallsValue(value, target ssa.Value) bool {
	return valueCallsValue(value, target, map[ssa.Value]bool{})
}

// ValueCallsMethod reports whether value is, or wraps, a callback that invokes
// method on target.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return valueCallsMethod(value, method, target, map[ssa.Value]bool{})
}

func valueCallsMethod(value ssa.Value, method string, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok {
		if ClosureCallsMethod(closure, method, target) {
			return true
		}
		if function, ok := closure.Fn.(*ssa.Function); ok {
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					if nested, ok := instruction.(*ssa.MakeClosure); ok && valueCallsMethod(nested, method, target, seen) {
						return true
					}
					common := InstructionCall(instruction)
					if common == nil {
						continue
					}
					for index, free := range function.FreeVars {
						if index < len(closure.Bindings) && AliasesValue(common.Value, free) && valueCallsMethod(closure.Bindings[index], method, target, seen) {
							return true
						}
					}
				}
			}
		}
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed && valueCallsMethod(store.Val, method, target, seen) {
					return true
				}
			}
		}
	case *ssa.Call:
		for _, argument := range typed.Common().Args {
			if valueCallsMethod(argument, method, target, seen) {
				return true
			}
		}
	case *ssa.ChangeInterface:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.ChangeType:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.Convert:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.MakeInterface:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() != nil {
			for _, reference := range *typed.X.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed.X && valueCallsMethod(store.Val, method, target, seen) {
					return true
				}
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsMethod(edge, method, target, seen) {
				return true
			}
		}
	}
	return false
}

func valueCallsValue(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok {
		if closureCallsValue(closure, target) {
			return true
		}
		if function, ok := closure.Fn.(*ssa.Function); ok {
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					inner, ok := instruction.(*ssa.MakeClosure)
					if ok && valueCallsValue(inner, target, seen) {
						return true
					}
				}
			}
		}
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed && valueCallsValue(store.Val, target, seen) {
					return true
				}
			}
		}
	case *ssa.ChangeInterface:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.ChangeType:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.Convert:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.MakeInterface:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() != nil {
			for _, reference := range *typed.X.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed.X && valueCallsValue(store.Val, target, seen) {
					return true
				}
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsValue(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

// ClosureCallsMethod reports whether a call-like closure calls method on target.
// It maps both captured free variables and explicit closure parameters back to
// the values supplied by the enclosing function.
func ClosureCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			if CallName(called) != method {
				continue
			}
			receiver := CallReceiver(called)
			if calledReceiverMatches(common, closure, function, receiver, target) {
				return true
			}
		}
	}
	return false
}

// StartedClosureCallsMethodOnEveryReturn reports whether a launched closure
// calls method on target before each of its normal returns.
func StartedClosureCallsMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	hasReturn, hasCleanup := false, false
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if _, ok := candidate.(*ssa.Return); ok {
				hasReturn = true
			}
			called := InstructionCall(candidate)
			if CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target) {
				hasCleanup = true
			}
		}
	}
	return hasReturn && hasCleanup && !UnownedReturnFromEntry(function, func(candidate ssa.Instruction) bool {
		called := InstructionCall(candidate)
		return CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target)
	})
}

// CallStartsClosureCallingMethodOnArgument reports whether a source-visible
// helper delegates an argument's lifecycle method to a goroutine. Starting the
// waiter transfers the obligation even when the caller joins it separately.
// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/middleware/plugin/process_unix_test.go#L35-L45
func CallStartsClosureCallingMethodOnArgument(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !AliasesValue(argument, target) && !ValueOwnsValue(argument, target) {
			continue
		}
		for _, block := range callee.Blocks {
			for _, candidate := range block.Instrs {
				if _, ok := candidate.(*ssa.Go); ok && ClosureCallsMethod(candidate, method, callee.Params[index]) {
					return true
				}
			}
		}
	}
	return false
}

// ClosureCallsMethodBeforeBranch reports whether a called function invokes
// method on target along an unconditional path from its entry block.
func ClosureCallsMethodBeforeBranch(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common, closure, function := calledFunction(instruction)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	visited := map[*ssa.BasicBlock]bool{}
	for block := function.Blocks[0]; block != nil && !visited[block]; {
		visited[block] = true
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			if CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target) {
				return true
			}
		}
		if len(block.Succs) != 1 {
			return false
		}
		block = block.Succs[0]
	}
	return false
}

func calledFunction(instruction ssa.Instruction) (*ssa.CallCommon, *ssa.MakeClosure, *ssa.Function) {
	if closure, ok := instruction.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		return nil, closure, function
	}
	common := InstructionCall(instruction)
	if common == nil {
		return nil, nil, nil
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	return common, closure, function
}

func calledReceiverMatches(common *ssa.CallCommon, closure *ssa.MakeClosure, function *ssa.Function, receiver, target ssa.Value) bool {
	if closure != nil {
		for index, free := range function.FreeVars {
			if AliasesValue(receiver, free) && index < len(closure.Bindings) && CapturedBindingAliases(closure.Bindings[index], target) {
				return true
			}
		}
	}
	if common == nil {
		return false
	}
	for index, parameter := range function.Params {
		if AliasesValue(receiver, parameter) && index < len(common.Args) && AliasesValue(common.Args[index], target) {
			return true
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
