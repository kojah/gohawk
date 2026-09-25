package ssaflow

import (
	"errors"
	"go/types"

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
	sources := sourceFunctions(result)
	functions := make([]*ssa.Function, 0, len(sources))
	for _, function := range sources {
		file := FunctionFile(pass, function)
		if function.Syntax() == nil || file == nil || !syntax.AnalyzeFile(pass, file) {
			continue
		}
		functions = append(functions, function)
	}
	return functions, nil
}

// sourceFunctions is buildssa's SrcFuncs plus the closures written in
// package-level variable initializers. SrcFuncs walks only function
// declarations, and such a closure belongs to the synthetic package
// initializer instead, so without this every command body declared as
// var cmd = &cli.Command{Action: func(...) {...}} would go unanalyzed.
// The synthetic initializer itself has no source to analyze.
func sourceFunctions(result *buildssa.SSA) []*ssa.Function {
	functions := append([]*ssa.Function(nil), result.SrcFuncs...)
	if result.Pkg == nil {
		return functions
	}
	return appendClosures(functions, result.Pkg.Func("init"))
}

// appendClosures appends function's closures, nested ones included.
func appendClosures(functions []*ssa.Function, function *ssa.Function) []*ssa.Function {
	if function == nil {
		return functions
	}
	for _, closure := range function.AnonFuncs {
		functions = appendClosures(append(functions, closure), closure)
	}
	return functions
}

// PackageFunctions returns every source function of the package outside
// excluded test files, for inventories that must see all of the package's
// code rather than only the canonical copy SourceSSAFunctions selects.
func PackageFunctions(pass *analysis.Pass) []*ssa.Function {
	result, ok := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	if !ok {
		return nil
	}
	sources := sourceFunctions(result)
	functions := make([]*ssa.Function, 0, len(sources))
	for _, function := range sources {
		if file := FunctionFile(pass, function); file != nil && syntax.ExcludedTestFile(pass, file) {
			continue
		}
		functions = append(functions, function)
	}
	return functions
}

// DeclaredFunctions lists a package's declared functions and methods and
// their closures, as buildssa's SrcFuncs does, for callers that have a
// package but no analysis pass. It cannot tell test files apart; an analyzer
// with a pass uses PackageFunctions.
func DeclaredFunctions(pkg *ssa.Package) []*ssa.Function {
	var functions []*ssa.Function
	var add func(*ssa.Function)
	add = func(function *ssa.Function) {
		if function == nil {
			return
		}
		functions = append(functions, function)
		for _, closure := range function.AnonFuncs {
			add(closure)
		}
	}
	// Closures in package-level variable initializers belong to the
	// synthetic initializer; see sourceFunctions.
	functions = appendClosures(functions, pkg.Func("init"))
	scope := pkg.Pkg.Scope()
	for _, name := range scope.Names() {
		switch object := scope.Lookup(name).(type) {
		case *types.Func:
			add(pkg.Prog.FuncValue(object))
		case *types.TypeName:
			if named, ok := object.Type().(*types.Named); ok && !object.IsAlias() {
				for method := range named.Methods() {
					add(pkg.Prog.FuncValue(method))
				}
			}
		}
	}
	return functions
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

// CallInvokesArgumentOnEveryReturn reports whether a statically known helper
// invokes target on every normal path through the helper.

// strictNonEmptyAccessPath reports whether value is a field or constant-index
// path strictly beneath root whose selected storage was not replaced before
// the load that observes it.

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
