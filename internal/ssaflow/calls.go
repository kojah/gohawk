package ssaflow

import (
	"errors"
	"go/token"

	"github.com/kojah/gohawk/internal/syntax"

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
		if function.Syntax() == nil || file == nil || !syntax.AnalyzeFile(pass, file) {
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

// CallName returns a statically known method, function, or builtin name.
// Use it only for structural contracts whose receiver or package is established
// separately. Match well-known declarations with CallMatchesSymbol instead.
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

// CallPackage returns a statically known package path for reporting,
// configuration, or package-wide API families. Match an exact well-known
// declaration with CallMatchesSymbol instead.
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
	return HasLibraryContract(common, ContractRuntimeGoexit) || HasLibraryContract(common, ContractTestingTermination)
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
		return common != nil && SameValue(common.Value, parameter) || callInvokesArgumentOnEveryReturn(candidate, parameter, seen)
	})
}

// CallCallsMethodOnArgumentOnEveryReturn reports whether a statically known
// helper calls method on target on every normal path through the helper.
func CallCallsMethodOnArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return callCallsMethodOnArgumentOnEveryReturn(instruction, method, target, map[*ssa.Function]bool{})
}

// DeferredHelperCallsMethodOnDerivedArgumentOnEveryReturn reports whether a
// deferred, statically known helper receives a non-trivial access path
// projected from target and calls method on that exact parameter on every
// normal return path.
func DeferredHelperCallsMethodOnDerivedArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	for _, argument := range common.Args {
		if !strictNonEmptyAccessPath(argument, target) {
			continue
		}
		if callCallsMethodOnExactArgumentOnEveryReturn(instruction, method, argument) {
			return true
		}
	}
	return false
}

// helperCallsMethodOnProjectedArgumentOnEveryReturn is the derived-argument
// helper proof for a named helper that is called directly or, when launched
// is set, started on a goroutine. Function literals are excluded: an
// immediately invoked literal that takes the projection as a parameter keeps
// its existing conservative boundary.
func helperCallsMethodOnProjectedArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value, launched bool) bool {
	if _, isGo := instruction.(*ssa.Go); isGo != launched {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	if common.StaticCallee().Parent() != nil {
		// Function literals, captured or not, keep the conservative boundary.
		return false
	}
	for _, argument := range common.Args {
		if strictNonEmptyAccessPath(argument, target) && callCallsMethodOnExactArgumentOnEveryReturn(instruction, method, argument) {
			return true
		}
	}
	return false
}

func strictNonEmptyAccessPath(value, root ssa.Value) bool {
	depth, ok := strictAccessPathDepth(value, root, map[ssa.Value]bool{})
	return ok && depth > 0
}

func strictAccessPathDepth(value, root ssa.Value, seen map[ssa.Value]bool) (int, bool) {
	if value == nil || root == nil || seen[value] {
		return 0, false
	}
	if value == root {
		return 0, true
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return strictAccessPathDepth(inner, root, seen)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		depth, ok := strictAccessPathDepth(typed.X, root, seen)
		return depth + 1, ok
	case *ssa.IndexAddr:
		if _, ok := constantIndex(typed.Index); !ok {
			return 0, false
		}
		depth, ok := strictAccessPathDepth(typed.X, root, seen)
		return depth + 1, ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			if depth, ok := strictAccessPathDepth(typed.X, root, seen); ok {
				return depth, true
			}
			if !storageAddressUnaliasedBeforeLoad(typed.X, typed) {
				return 0, false
			}
			stored, ok := uniquelyStoredValueBefore(typed.X, typed)
			if !ok {
				return 0, false
			}
			return strictAccessPathDepth(stored, root, map[ssa.Value]bool{})
		}
	}
	return 0, false
}

func storageAddressUnaliasedBeforeLoad(address ssa.Value, observation *ssa.UnOp) bool {
	if address == nil || address.Referrers() == nil || observation == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.Store:
			if typed.Addr == address {
				continue
			}
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == address {
				continue
			}
		}
		if InstructionMayFollow(reference, observation) {
			return false
		}
	}
	return true
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
		directCall := common != nil && CallName(common) == method &&
			ValueDerivesFrom(CallReceiver(common), parameter, map[ssa.Value]bool{})
		// A static helper can own cleanup by registering a deferred callback on
		// the exact parameter before every normal return. Blnk uses this shape to
		// close SQL rows after a shared scanner has consumed them:
		// https://github.com/blnkfinance/blnk/blob/3356cb4c482c065e96624957a3f4be3ae9739c1a/database/transaction_coalescing.go#L149-L154
		deferredCall := DeferredCallbackCallsMethod(candidate, method, parameter)
		// A helper may also hand cleanup to testing's Cleanup, which runs after
		// the test regardless of how the helper returns. The callback must call
		// method on the exact parameter unconditionally. filesql closes fixture
		// files this way:
		// https://github.com/nao1215/filesql/blob/60ff1b62eace61b6a0d9da6b55aaf8e053bf89f2/internal/parser/parser_test.go#L18-L26
		return directCall || deferredCall || testingCleanupCallsMethod(candidate, method, parameter) ||
			callCallsMethodOnArgumentOnEveryReturn(candidate, method, parameter, seen)
	})
}

// testingCleanupCallsMethod reports whether instruction registers a testing
// Cleanup callback that unconditionally calls method on target.
func testingCleanupCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common := InstructionCall(instruction)
	if !HasLibraryContract(common, ContractTestingCleanup) {
		return false
	}
	for _, argument := range common.Args {
		if closure, ok := argument.(*ssa.MakeClosure); ok && ClosureCallsMethodBeforeBranch(closure, method, target) {
			return true
		}
	}
	return false
}

func callCallsMethodOnExactArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0 {
		return false
	}
	callee := common.StaticCallee()
	for index, argument := range common.Args {
		if index >= len(callee.Params) || argument != target {
			continue
		}
		parameter := callee.Params[index]
		calls := func(candidate ssa.Instruction) bool {
			called := InstructionCall(candidate)
			return called != nil && CallName(called) == method && exactCleanupReceiver(CallReceiver(called), parameter)
		}
		if MethodCallCoverage(callee, calls, CoverageEveryReturn, parameter) {
			return true
		}
	}
	return false
}

func exactCleanupReceiver(receiver, parameter ssa.Value) bool {
	if receiver == nil || parameter == nil {
		return false
	}
	if inner, ok := UnwrapTransparentValue(
		receiver,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return exactCleanupReceiver(inner, parameter)
	}
	// The derived caller projection is already mapped to this parameter. Only
	// the parameter itself may discharge the obligation; phis, loads, aliases,
	// and further projections remain opaque.
	return receiver == parameter
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
		if index >= len(callee.Params) || !SameValue(argument, target) && !ValueContainsValue(argument, target) {
			continue
		}
		parameter := callee.Params[index]
		calls := func(candidate ssa.Instruction) bool {
			return owns(candidate, parameter)
		}
		if MethodCallCoverage(callee, calls, CoverageEveryReturn, parameter) {
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
