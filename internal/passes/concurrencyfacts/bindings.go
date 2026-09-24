package concurrencyfacts

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Binding a Summary preserves the time at which a resource was read. Captured
// cells additionally need stable contents because the worker may read them
// after the launch. A matching access path alone cannot justify this identity.
func (engine *Engine) instantiate(instruction ssa.CallInstruction) Summary {
	return engine.instantiatedCutoff(engine.instantiateEffects(instruction), instruction)
}

func (engine *Engine) instantiateEffects(instruction ssa.CallInstruction) Summary {
	common := engine.resolvedCommon(instruction)
	if function := common.StaticCallee(); function != nil && len(function.Blocks) == 0 {
		return engine.importedCall(instruction, function)
	}
	if instruction.Common().IsInvoke() && !common.IsInvoke() {
		function, closure := ssaflow.DirectCallee(common)
		callee := engine.summaries.Function(function, engine.budget)
		return engine.bindSummary(callee, ssaflow.CallBindings(common, function, closure), instruction)
	}
	return engine.summaries.AtCall(instruction, engine.budget, func(callee Summary, bindings []ssaflow.CallBinding) Summary {
		return engine.bindSummary(callee, bindings, instruction)
	})
}

func (engine *Engine) bindSummary(callee Summary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction) Summary {
	if len(callee.Paths) != 0 {
		return engine.bindPaths(callee.Paths, bindings, instruction)
	}
	if !composableLinear(callee) && callee.Reason != ReasonSelectAlternatives {
		return callee
	}
	result := Summary{
		Operations: make([]Operation, 0, len(callee.Operations)), Reason: callee.Reason,
		AlternativesComplete: callee.AlternativesComplete,
		Conditions:           boundConditions(callee.Conditions, bindings, instruction.Pos()),
		Returned:             callee.Returned,
	}
	inputs, reason := engine.bindCancellationInputs(callee.CancellationInputs, bindings, instruction)
	if reason != ReasonNone {
		return Summary{Reason: reason}
	}
	result.CancellationInputs = inputs
	offsets, reason := engine.bindSequence(&result, callee.Operations, bindings, instruction)
	if reason != ReasonNone {
		return Summary{Reason: reason}
	}
	workers, reason := engine.bindWorkers(callee.Workers, bindings, instruction)
	if reason != ReasonNone {
		return Summary{Reason: reason}
	}
	if reason := placeWorkers(&result, workers, offsets); reason != ReasonNone {
		return Summary{Reason: reason}
	}
	for _, choice := range callee.Choices {
		// A select's arm sequence is a separate complete path, not another
		// unconditional effect. Bind every resource on every arm before the
		// caller may use any variant as a graph proof.
		if choice.Prefix < 0 || choice.Prefix >= len(offsets) {
			return Summary{Reason: ReasonEffectUnknown}
		}
		bound := SelectChoice{Prefix: offsets[choice.Prefix], Site: choice.Site, Worker: choice.Worker}
		for _, arm := range choice.Arms {
			if !engine.budget.Spend() {
				return Summary{Reason: ReasonBudgetExhausted}
			}
			if !arm.Default {
				resource, ok := engine.bind(arm.Operation.Resource, bindings, instruction)
				if !ok {
					return Summary{Reason: ReasonChannelBindingUnknown}
				}
				arm.Operation.Resource, arm.Operation.Site = resource, instruction.Pos()
			}
			if arm.Complete {
				// Keep the callee's source location but attribute a bound action
				// to this call site, as with the linear effect sequence above.
				sequence := make([]Operation, 0, len(arm.Sequence))
				for _, op := range arm.Sequence {
					if !engine.budget.Spend() {
						return Summary{Reason: ReasonBudgetExhausted}
					}
					resource, ok := engine.bind(op.Resource, bindings, instruction)
					if !ok {
						return Summary{Reason: ReasonChannelBindingUnknown}
					}
					op.Resource, op.Site = resource, instruction.Pos()
					sequence = append(sequence, op)
				}
				arm.Sequence = sequence
			}
			bound.Arms = append(bound.Arms, arm)
		}
		result.Choices = append(result.Choices, bound)
	}
	return finishCancellation(result)
}

func (engine *Engine) bindWorkers(
	workers []WorkerSummary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction,
) ([]WorkerSummary, Reason) {
	if len(workers) > maxWorkers {
		return nil, ReasonParticipantsUnknown
	}
	bound := make([]WorkerSummary, 0, len(workers))
	for _, worker := range workers {
		child, reason := engine.bindWorker(worker, bindings, instruction)
		if reason != ReasonNone {
			return nil, reason
		}
		bound = append(bound, child)
	}
	return bound, ReasonNone
}

func (engine *Engine) bind(
	reference Reference, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (Reference, bool) {
	if reference.Projection.Depth > 0 {
		return engine.bindField(reference, bindings, instruction)
	}
	if global, ok := reference.Value.(*ssa.Global); ok && !reference.Indirect && MutexPointer(global.Type()) {
		// A package mutex is the same object in every caller.
		return reference, true
	}
	for _, binding := range bindings {
		if binding.Local != reference.Value {
			continue
		}
		if !reference.Indirect {
			return engine.reference(binding.Supplied)
		}
		if _, captured := binding.Supplied.(*ssa.FreeVar); captured {
			return Reference{Value: binding.Supplied, Indirect: true, Cancellation: reference.Cancellation}, true
		}
		content := engine.storage.StableContent(binding.Supplied, instruction)
		if content.Proven() {
			return engine.reference(content.Value)
		}
		return Reference{}, false
	}
	return engine.bindField(reference, bindings, instruction)
}

func (engine *Engine) reference(value ssa.Value) (Reference, bool) {
	if value == nil || !ssaflow.ChannelType(value) && !synchronizationPointer(value.Type()) && !cancellationType(value.Type()) {
		return Reference{}, false
	}
	return ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		engine.referenceLeaf, func(reference Reference) Reference { return reference })
}

func (engine *Engine) referenceLeaf(_ ssaflow.ReachingWalk, value ssa.Value) (Reference, bool) {
	resolved := engine.storage.Resolve(value)
	if resolved.Proven() {
		if reference, ok := engine.resolvedReference(resolved.Value); ok {
			return reference, true
		}
	}
	// A symbolic captured cell is resolved at its caller, where its local
	// allocation and all writes are visible to the shared storage query.
	load, ok := value.(*ssa.UnOp)
	if ok && load.Op == token.MUL {
		if ssaflow.ChannelType(load) {
			if path, exact := embeddedPath(load.X); exact && path.Depth > 0 {
				return Reference{Value: load, Projection: path, Indirect: true}, true
			}
		}
		if capture, ok := load.X.(*ssa.FreeVar); ok && readableAddress(capture) {
			return Reference{Value: capture, Indirect: true, Cancellation: cancellationType(value.Type())}, true
		}
	}
	return Reference{}, false
}

func (engine *Engine) resolvedReference(value ssa.Value) (Reference, bool) {
	if reference, ok := engine.cancellationReference(value); ok {
		return reference, true
	}
	switch value := value.(type) {
	case *ssa.Call:
		return Reference{Value: value}, ssaflow.CallMatchesSymbol(value.Common(), newCond)
	case *ssa.Parameter, *ssa.FreeVar, *ssa.MakeChan:
		return Reference{Value: value, Cancellation: cancellationType(value.Type())}, true
	case *ssa.Alloc:
		if synchronizationPointer(value.Type()) {
			return Reference{Value: value}, true
		}
	case *ssa.Global, *ssa.FieldAddr:
		// Mutex addresses identify cells, not mutable contents. Such values
		// can bind formal mutex parameters but cannot be exported as formals.
		if MutexPointer(value.Type()) {
			if path, ok := engine.identityPath(value); ok && path.Depth > 0 {
				value, found := engine.fieldAddress(value.Parent(), path)
				return Reference{Value: value}, found
			}
			return Reference{Value: value}, true
		}
	}
	return Reference{}, false
}

func readableAddress(value ssa.Value) bool {
	if localAddress(value) {
		return true
	}
	if _, ok := value.(*ssa.FreeVar); !ok {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	_, channel := pointer.Elem().Underlying().(*types.Chan)
	return channel || synchronizationPointer(pointer.Elem()) || cancellationType(pointer.Elem())
}

func localAddress(value ssa.Value) bool {
	return ssaflow.NewReachingWalk(ssaflow.TransparentNone).Every(value, localAddressLeaf)
}

func localAddressLeaf(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	switch value := value.(type) {
	case *ssa.Alloc:
		return true
	case *ssa.FieldAddr:
		return walk.Every(value.X, localAddressLeaf)
	default:
		return false
	}
}

// bindSequence binds the callee's linear operations into result. Binding
// creates new sequences: cached declaration identities must not become tied
// to the first caller, including its receiver projections. A filled callback
// hole can insert several operations, so the returned offsets map each callee
// position, and the end, to its position in the bound sequence.
func (engine *Engine) bindSequence(
	result *Summary, operations []Operation, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction,
) ([]int, Reason) {
	offsets := make([]int, 0, len(operations)+1)
	for _, op := range operations {
		offsets = append(offsets, len(result.Operations))
		if !engine.budget.Spend() {
			return nil, ReasonBudgetExhausted
		}
		if op.Kind == Invoke {
			supplied, ok := holeSupplied(op, bindings)
			if !ok {
				return nil, ReasonCallbackUnknown
			}
			if reason := engine.bindCallback(result, op, supplied, instruction); reason != ReasonNone {
				return nil, reason
			}
			continue
		}
		resource, ok := engine.bind(op.Resource, bindings, instruction)
		if !ok {
			return nil, ReasonChannelBindingUnknown
		}
		op.Resource, op.Site = resource, instruction.Pos()
		result.Operations = append(result.Operations, op)
	}
	return append(offsets, len(result.Operations)), ReasonNone
}

// placeWorkers maps each bound worker's launch point through offsets and
// merges it with workers a filled callback already launched, in launch order.
func placeWorkers(result *Summary, workers []WorkerSummary, offsets []int) Reason {
	for index := range workers {
		if workers[index].Prefix < 0 || workers[index].Prefix >= len(offsets) {
			return ReasonEffectUnknown
		}
		workers[index].Prefix = offsets[workers[index].Prefix]
	}
	result.Workers = append(workers, result.Workers...)
	if len(result.Workers) > maxWorkers {
		return ReasonParticipantsUnknown
	}
	slices.SortStableFunc(result.Workers, func(a, b WorkerSummary) int { return a.Prefix - b.Prefix })
	return ReasonNone
}
