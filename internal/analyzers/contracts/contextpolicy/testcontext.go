package contextpolicy

import (
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzers/ownership/goroutineownership"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type detachedTestContext struct {
	root     ssa.Value
	position token.Pos
}

func reportDetachedTestBackground(pass *analysis.Pass, function *ssa.Function) {
	file := ssautil.FunctionFile(pass, function)
	if file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") || !functionHasTestingHandle(function) {
		return
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			spawn, ok := instruction.(*ssa.Go)
			if !ok || goroutineownership.HasExplicitGoroutineOwnership(spawn) {
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
						check.ContextTestOwnership,
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
		if analysisutil.NamedType(parameter.Type(), "testing", "T") || analysisutil.NamedType(parameter.Type(), "testing", "B") {
			return true
		}
	}
	for _, free := range function.FreeVars {
		if analysisutil.NamedType(free.Type(), "testing", "T") || analysisutil.NamedType(free.Type(), "testing", "B") {
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
			position, ok := neverCancelledTestContext(ssautil.CapturedBindingValue(binding), map[ssa.Value]bool{})
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
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return typed.X, true
	case *ssa.ChangeType:
		return typed.X, true
	case *ssa.Convert:
		return typed.X, true
	case *ssa.MakeInterface:
		return typed.X, true
	case *ssa.UnOp:
		return typed.X, true
	default:
		return nil, false
	}
}

func neverCancelledContextCall(call *ssa.Call, seen map[ssa.Value]bool) (token.Pos, bool) {
	common := call.Common()
	if ssautil.CallPackage(common) != "context" {
		return token.NoPos, false
	}
	switch ssautil.CallName(common) {
	case "Background", "TODO":
		return call.Pos(), true
	case "WithValue":
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
			common := ssautil.InstructionCall(instruction)
			if common == nil {
				continue
			}
			name := ssautil.CallName(common)
			if (name == "Done" || name == "Err") && ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), value, map[ssa.Value]bool{}) {
				return true
			}
			callee := common.StaticCallee()
			if callee == nil {
				continue
			}
			for index, argument := range common.Args {
				if index < len(callee.Params) && ssautil.SameValue(argument, value) && functionObservesCancellation(callee, callee.Params[index], seen) {
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
