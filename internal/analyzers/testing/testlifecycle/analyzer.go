// Package testlifecycle implements the testlifecycle gohawk analyzer.
package testlifecycle

import (
	"errors"
	"go/token"
	"go/version"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass. The registry
// supplies the conservative ownership query at composition time.
func Analyzer(ownershipMayBeHandled func(*ssa.Go) bool) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "testlifecycle",
		Doc:      "checks that test-owned asynchronous work inherits the test lifecycle",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			if ownershipMayBeHandled == nil {
				return nil, errors.New("testlifecycle requires a goroutine ownership proof")
			}
			return runTestLifecycle(pass, ownershipMayBeHandled)
		},
	}
}

func runTestLifecycle(pass *analysis.Pass, ownershipMayBeHandled func(*ssa.Go) bool) (any, error) {
	if !supportsTestingContext(pass) {
		return nil, nil
	}
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportDetachedTestBackground(pass, function, ownershipMayBeHandled)
	}
	return nil, nil
}

func supportsTestingContext(pass *analysis.Pass) bool {
	if pass.Module == nil || pass.Module.GoVersion == "" {
		return true
	}
	moduleVersion := pass.Module.GoVersion
	if !strings.HasPrefix(moduleVersion, "go") {
		moduleVersion = "go" + moduleVersion
	}
	return version.Compare(moduleVersion, "go1.24") >= 0
}

type detachedTestContext struct {
	root     ssa.Value
	position token.Pos
}

func reportDetachedTestBackground(pass *analysis.Pass, function *ssa.Function, ownershipMayBeHandled func(*ssa.Go) bool) {
	file := ssaflow.FunctionFile(pass, function)
	if file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") || !functionHasTestingHandle(function) {
		return
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			spawn, ok := instruction.(*ssa.Go)
			if !ok || ownershipMayBeHandled(spawn) {
				continue
			}
			// A synchronous call with Background has no test-teardown lifetime to
			// inherit. Require an explicit detached goroutine and observable
			// cancellation before suggesting t.Context; Network Doctor's ordinary
			// probe calls are representative accepted forms:
			// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/diagnostic/checks_extra_test.go#L120-L130
			for _, candidate := range detachedTestContexts(spawn) {
				if functionObservesCancellation(spawnedFunction(spawn), candidate.root, map[contextObservation]bool{}) {
					check.Reportf(
						pass,
						check.TestLifecycleContext,
						candidate.position,
						"test-owned goroutine uses a never-cancelled context; use the testing handle's Context()",
					)
					break
				}
			}
		}
	}
}

func functionHasTestingHandle(function *ssa.Function) bool {
	for _, parameter := range function.Params {
		if syntax.NamedType(parameter.Type(), "testing", "T") || syntax.NamedType(parameter.Type(), "testing", "B") {
			return true
		}
	}
	for _, free := range function.FreeVars {
		if syntax.NamedType(free.Type(), "testing", "T") || syntax.NamedType(free.Type(), "testing", "B") {
			return true
		}
	}
	return false
}

func detachedTestContexts(spawn *ssa.Go) []detachedTestContext {
	function := spawnedFunction(spawn)
	if function == nil {
		return nil
	}
	var result []detachedTestContext
	if closure, ok := spawn.Common().Value.(*ssa.MakeClosure); ok {
		for index, binding := range closure.Bindings {
			if index >= len(function.FreeVars) {
				break
			}
			position, ok := neverCancelledTestContext(ssaflow.CapturedBindingValue(binding), map[ssa.Value]bool{})
			if ok {
				result = append(result, detachedTestContext{root: function.FreeVars[index], position: position})
			}
		}
	}
	for index, argument := range spawn.Common().Args {
		if index >= len(function.Params) {
			break
		}
		position, ok := neverCancelledTestContext(argument, map[ssa.Value]bool{})
		if ok {
			result = append(result, detachedTestContext{root: function.Params[index], position: position})
		}
	}
	return result
}

func spawnedFunction(spawn *ssa.Go) *ssa.Function {
	if closure, ok := spawn.Common().Value.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		return function
	}
	return spawn.Common().StaticCallee()
}

func neverCancelledTestContext(value ssa.Value, seen map[ssa.Value]bool) (token.Pos, bool) {
	// Background and TODO remain detached through transparent wrappers, stores,
	// and phi nodes only when every incoming value is likewise detached. A mixed
	// merge is not safe evidence for recommending the testing context.
	if value == nil || seen[value] {
		return token.NoPos, false
	}
	seen[value] = true
	if source, ok := contextSource(value); ok {
		return neverCancelledTestContext(source, seen)
	}
	switch typed := value.(type) {
	case *ssa.Call:
		return neverCancelledContextCall(typed, seen)
	case *ssa.Alloc:
		return neverCancelledStoredContext(typed, seen)
	case *ssa.Phi:
		return neverCancelledContextEdges(typed.Edges, seen)
	}
	return token.NoPos, false
}

func contextSource(value ssa.Value) (ssa.Value, bool) {
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return inner, true
	}
	switch typed := value.(type) {
	case *ssa.UnOp:
		return typed.X, true
	default:
		return nil, false
	}
}

func neverCancelledContextCall(call *ssa.Call, seen map[ssa.Value]bool) (token.Pos, bool) {
	common := call.Common()
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("context", "Background")) ||
		ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("context", "TODO")) {
		return call.Pos(), true
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("context", "WithValue")) {
		if len(common.Args) > 0 {
			return neverCancelledTestContext(common.Args[0], seen)
		}
	}
	return token.NoPos, false
}

func neverCancelledStoredContext(address ssa.Value, seen map[ssa.Value]bool) (token.Pos, bool) {
	if address.Referrers() == nil {
		return token.NoPos, false
	}
	values := make([]ssa.Value, 0)
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address {
			values = append(values, store.Val)
		}
	}
	return neverCancelledContextEdges(values, seen)
}

func neverCancelledContextEdges(values []ssa.Value, seen map[ssa.Value]bool) (token.Pos, bool) {
	if len(values) == 0 {
		return token.NoPos, false
	}
	var position token.Pos
	for _, value := range values {
		candidate, ok := neverCancelledTestContext(value, cloneContextSeen(seen))
		if !ok {
			return token.NoPos, false
		}
		position = candidate
	}
	return position, true
}

type contextObservation struct {
	function *ssa.Function
	value    ssa.Value
}

func functionObservesCancellation(function *ssa.Function, value ssa.Value, seen map[contextObservation]bool) bool {
	// A detached context matters only when the goroutine consults cancellation.
	// Follow exact arguments into static callees, but stop at dynamic dispatch or
	// value transformations whose parameter relationship cannot be proved.
	if function == nil || value == nil {
		return false
	}
	key := contextObservation{function: function, value: value}
	if seen[key] {
		return false
	}
	seen[key] = true
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if common == nil {
				continue
			}
			name := ssaflow.CallName(common)
			if (name == "Done" || name == "Err") && ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), value, map[ssa.Value]bool{}) {
				return true
			}
			callee := common.StaticCallee()
			if callee == nil {
				continue
			}
			for index, argument := range common.Args {
				if index < len(callee.Params) && ssaflow.SameValue(argument, value) && functionObservesCancellation(callee, callee.Params[index], seen) {
					return true
				}
			}
		}
	}
	return false
}

func cloneContextSeen(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	for value := range source {
		result[value] = true
	}
	return result
}
