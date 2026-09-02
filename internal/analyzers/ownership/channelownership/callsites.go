package channelownership

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// This file reconstructs only finite bound-method callsites needed by channel
// ownership proofs. It maps synthetic wrapper arguments back to source methods
// and stops at the first opaque function-value use or escape.

// channelCallsite preserves the outer invocation and maps its arguments back
// to the source callee's parameters. Bound method wrappers insert the receiver,
// so their raw call arguments do not share the source method's parameter index.
type channelCallsite struct {
	instruction ssa.CallInstruction
	arguments   []ssa.Value
}

type channelCalleeCalls struct {
	calls       []channelCallsite
	complete    bool
	finiteBound bool
}

type boundCallResolution struct {
	calls     []ssa.CallInstruction
	complete  bool
	done      bool
	resolving bool
}

type boundCallResolver struct {
	cache map[ssa.Value]boundCallResolution
}

// buildChannelCallsites adds one deliberately narrow form of indirect call
// evidence to ordinary static callsites. It accepts only compiler-generated
// bound method wrappers whose closure values have a finite, completely resolved
// local use graph. Any escape keeps complete false and therefore cannot suppress
// a diagnostic.
func buildChannelCallsites(allFunctions, analyzedFunctions []*ssa.Function) map[*ssa.Function]*channelCalleeCalls {
	result := make(map[*ssa.Function]*channelCalleeCalls, len(analyzedFunctions))
	resolver := boundCallResolver{cache: make(map[ssa.Value]boundCallResolution)}
	analyzed := make(map[*ssa.Function]bool, len(analyzedFunctions))
	analyzedByObject := make(map[types.Object]*ssa.Function, len(analyzedFunctions))
	for _, function := range analyzedFunctions {
		analyzed[function] = true
		if function.Object() != nil {
			analyzedByObject[function.Object()] = function
		}
		result[function] = &channelCalleeCalls{complete: true}
	}
	addStaticChannelCallsites(allFunctions, result)
	addBoundMethodCallsites(allFunctions, analyzed, analyzedByObject, result, &resolver)
	for _, function := range allFunctions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				markOpaqueFunctionReferences(instruction, analyzedByObject, result)
			}
		}
	}
	return result
}

func addStaticChannelCallsites(allFunctions []*ssa.Function, result map[*ssa.Function]*channelCalleeCalls) {
	for callee, calls := range ssaflow.StaticCallsites(allFunctions) {
		entry := result[callee]
		if entry == nil {
			continue
		}
		for _, call := range calls {
			entry.calls = append(entry.calls, channelCallsite{
				instruction: call,
				arguments:   call.Common().Args,
			})
		}
	}
}

func addBoundMethodCallsites(
	allFunctions []*ssa.Function,
	analyzed map[*ssa.Function]bool,
	analyzedByObject map[types.Object]*ssa.Function,
	result map[*ssa.Function]*channelCalleeCalls,
	resolver *boundCallResolver,
) {
	for _, function := range allFunctions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				closure, ok := instruction.(*ssa.MakeClosure)
				if !ok {
					continue
				}
				possibleTarget := possibleBoundMethodTarget(closure, analyzedByObject)
				if possibleTarget == nil {
					continue
				}
				entry := result[possibleTarget]
				entry.finiteBound = true
				target, wrapper, exact := exactBoundMethodClosure(closure)
				if !exact || !analyzed[target] || target != possibleTarget {
					// Recognizing a possible source target is enough to invalidate
					// completeness. Only the exact forwarding shape below may add
					// suppressing caller evidence.
					entry.complete = false
					continue
				}
				calls, complete := resolveBoundClosureCalls(closure, wrapper, resolver)
				entry.calls = append(entry.calls, calls...)
				entry.complete = entry.complete && complete
			}
		}
	}
}

func channelCallerFunctions(pkg *ssa.Package, sourceFunctions []*ssa.Function) []*ssa.Function {
	result := make([]*ssa.Function, 0, len(sourceFunctions)+1)
	seen := make(map[*ssa.Function]bool, len(sourceFunctions)+1)
	var add func(*ssa.Function)
	add = func(function *ssa.Function) {
		if function == nil || seen[function] {
			return
		}
		seen[function] = true
		result = append(result, function)
		for _, anonymous := range function.AnonFuncs {
			add(anonymous)
		}
	}
	for _, function := range sourceFunctions {
		add(function)
	}
	if pkg != nil {
		// buildssa.SrcFuncs contains function declarations and their literals,
		// while package-scope initializer expressions live in synthetic init.
		add(pkg.Func("init"))
	}
	return result
}

func possibleBoundMethodTarget(closure *ssa.MakeClosure, analyzedByObject map[types.Object]*ssa.Function) *ssa.Function {
	wrapper, ok := closure.Fn.(*ssa.Function)
	if !ok || wrapper.Syntax() != nil || len(wrapper.FreeVars) != 1 || len(closure.Bindings) != 1 || wrapper.Signature == nil ||
		wrapper.Signature.Recv() != nil {
		return nil
	}
	object := wrapper.Object()
	method, ok := object.(*types.Func)
	if !ok {
		return nil
	}
	signature, ok := method.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return nil
	}
	return analyzedByObject[object]
}

// exactBoundMethodClosure recognizes only the x/tools SSA wrapper shape whose
// sole operation forwards its captured receiver and parameters to one method.
func exactBoundMethodClosure(closure *ssa.MakeClosure) (*ssa.Function, *ssa.Function, bool) {
	wrapper, ok := closure.Fn.(*ssa.Function)
	if !ok || !strings.HasPrefix(wrapper.Synthetic, "bound method wrapper") || len(wrapper.Blocks) != 1 ||
		len(wrapper.FreeVars) != 1 || len(closure.Bindings) != 1 {
		return nil, nil, false
	}

	var forwarded *ssa.Call
	for _, instruction := range wrapper.Blocks[0].Instrs {
		switch typed := instruction.(type) {
		case *ssa.Call:
			if forwarded != nil {
				return nil, nil, false
			}
			forwarded = typed
		case *ssa.Return, *ssa.DebugRef:
		default:
			return nil, nil, false
		}
	}
	if forwarded == nil || forwarded.Common().StaticCallee() == nil {
		return nil, nil, false
	}
	target := forwarded.Common().StaticCallee()
	arguments := forwarded.Common().Args
	if target.Signature == nil || target.Signature.Recv() == nil || len(arguments) != len(wrapper.Params)+1 ||
		len(target.Params) != len(arguments) || arguments[0] != wrapper.FreeVars[0] {
		return nil, nil, false
	}
	for index, parameter := range wrapper.Params {
		if arguments[index+1] != parameter {
			return nil, nil, false
		}
	}
	return target, wrapper, true
}

func resolveBoundClosureCalls(
	origin *ssa.MakeClosure,
	wrapper *ssa.Function,
	resolver *boundCallResolver,
) ([]channelCallsite, bool) {
	invocations, complete := resolver.resolve(origin)
	if !complete || len(origin.Bindings) != 1 {
		return nil, false
	}
	result := make([]channelCallsite, 0, len(invocations))
	for _, invocation := range invocations {
		if len(invocation.Common().Args) != len(wrapper.Params) {
			return nil, false
		}
		arguments := make([]ssa.Value, len(wrapper.Params)+1)
		arguments[0] = origin.Bindings[0]
		copy(arguments[1:], invocation.Common().Args)
		result = append(result, channelCallsite{instruction: invocation, arguments: arguments})
	}
	return result, true
}

func (resolver *boundCallResolver) resolve(value ssa.Value) ([]ssa.CallInstruction, bool) {
	if value == nil {
		return nil, false
	}
	// A cycle in a phi graph contributes no new terminal use. It is complete
	// only if the surrounding traversal still reaches concrete invocations;
	// an empty result cannot settle ownership at the caller decision point.
	if cached, ok := resolver.cache[value]; ok {
		if cached.done {
			return cached.calls, cached.complete
		}
		if cached.resolving {
			return nil, true
		}
	}
	resolver.cache[value] = boundCallResolution{resolving: true}
	if value.Referrers() == nil {
		resolver.cache[value] = boundCallResolution{complete: true, done: true}
		return nil, true
	}

	var calls []ssa.CallInstruction
	for _, reference := range *value.Referrers() {
		// Completeness is all-or-nothing: only transparent value plumbing and an
		// invocation may consume the method value. A store, return, or opaque call
		// invalidates every otherwise safe callsite collected for this callee.
		var nested ssa.Value
		switch typed := reference.(type) {
		case ssa.CallInstruction:
			if typed.Common().Value != value {
				resolver.cache[value] = boundCallResolution{done: true}
				return nil, false
			}
			calls = append(calls, typed)
			continue
		case *ssa.Phi:
			nested = typed
		case *ssa.ChangeType:
			nested = typed
		case *ssa.Convert:
			nested = typed
		case *ssa.DebugRef:
			continue
		default:
			resolver.cache[value] = boundCallResolution{done: true}
			return nil, false
		}
		resolved, complete := resolver.resolve(nested)
		if !complete {
			resolver.cache[value] = boundCallResolution{done: true}
			return nil, false
		}
		calls = append(calls, resolved...)
	}
	resolver.cache[value] = boundCallResolution{calls: calls, complete: true, done: true}
	return calls, true
}

func markOpaqueFunctionReferences(
	instruction ssa.Instruction,
	analyzedByObject map[types.Object]*ssa.Function,
	result map[*ssa.Function]*channelCalleeCalls,
) {
	if _, ok := instruction.(*ssa.DebugRef); ok {
		return
	}
	common := ssaflow.InstructionCall(instruction)
	for _, operand := range instruction.Operands(nil) {
		if operand == nil || *operand == nil {
			continue
		}
		function, ok := (*operand).(*ssa.Function)
		if !ok || common != nil && common.StaticCallee() == function {
			continue
		}
		if closure, ok := instruction.(*ssa.MakeClosure); ok && closure.Fn == function {
			continue
		}
		target := function
		if result[target] == nil {
			target = possibleMethodExpressionTarget(function, analyzedByObject)
		}
		entry := result[target]
		if entry == nil {
			continue
		}
		// A direct reference to the source function is a function or method
		// expression escape. Bound method values refer to their synthetic wrapper
		// instead and are handled above.
		entry.complete = false
	}
}

func possibleMethodExpressionTarget(function *ssa.Function, analyzedByObject map[types.Object]*ssa.Function) *ssa.Function {
	if function == nil || function.Syntax() != nil || function.Signature == nil || function.Signature.Recv() != nil {
		return nil
	}
	method, ok := function.Object().(*types.Func)
	if !ok {
		return nil
	}
	signature, ok := method.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return nil
	}
	return analyzedByObject[method]
}
