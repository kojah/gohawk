package ssaflow

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// EnclosingCompletionRequest asks whether every locally visible invocation of
// a literal receives a value whose enclosing scope registered deferred cleanup.
// Unsupported escapes, dispatch, mutation, or exhausted budgets prove nothing.
// It does not enumerate callers of named functions or export relational facts.
type EnclosingCompletionRequest struct {
	Function *ssa.Function
	Value    ssa.Value
	Methods  []string
	Budget   *SearchBudget
}

type enclosingFrame struct {
	function   *ssa.Function
	bindings   *callbackBindings
	parent     *enclosingFrame
	invocation ssa.Instruction
}

type enclosingSearch struct {
	request EnclosingCompletionRequest
	search  *completionSearch
	memo    *CallGraphMemo[*enclosingFrame, bool]
	found   bool
}

// ProveEnclosingCompletion follows callback arguments from their lexical
// owner, requiring all discovered invocations to have the same cleanup
// guarantee. An incomplete traversal is Unknown, never a cleanup proof.
func ProveEnclosingCompletion(request EnclosingCompletionRequest) CompletionProof {
	unknown := CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	if request.Function == nil || request.Function.Parent() == nil || request.Value == nil || request.Budget == nil {
		return unknown
	}
	root := request.Function.Parent()
	for root.Parent() != nil {
		root = root.Parent()
	}
	search := &enclosingSearch{
		request: request, search: newCompletionSearch("", CoverageEveryReturn, request.Budget),
		memo: NewCallGraphMemo[*enclosingFrame, bool](),
	}
	complete := search.walk(&enclosingFrame{function: root})
	if request.Budget.Exhausted() {
		unknown.Reason = EvidenceBudgetExhausted
		return unknown
	}
	if !complete || !search.found {
		return unknown
	}
	return CompletionProof{Proof{State: EvidenceProven, Reason: EvidenceDeferredCompletion, Provenance: EvidenceFromLocalSSA}}
}

func (search *enclosingSearch) walk(frame *enclosingFrame) bool {
	return search.memo.Summarize(frame, frame.function, search.request.Budget, func() bool {
		if !search.request.Budget.Spend() {
			return false
		}
		if frame.function == search.request.Function {
			search.found = true
			return search.completed(frame)
		}
		for _, block := range frame.function.Blocks {
			for _, instruction := range block.Instrs {
				if !search.request.Budget.Spend() || !search.instruction(frame, instruction) {
					return false
				}
			}
		}
		return true
	}, func(SummaryUnavailable, bool) bool {
		return false
	})
}

func callbackStorageType(t types.Type) bool {
	for range 8 {
		switch underlying := t.Underlying().(type) {
		case *types.Signature:
			return true
		case *types.Pointer:
			t = underlying.Elem()
		case *types.Slice:
			t = underlying.Elem()
		case *types.Array:
			t = underlying.Elem()
		default:
			return false
		}
	}
	return true
}

func localCallbackAddress(value ssa.Value) bool {
	for range 8 {
		switch valueType := value.(type) {
		case *ssa.Alloc:
			return true
		case *ssa.IndexAddr:
			value = valueType.X
		case *ssa.Slice:
			value = valueType.X
		default:
			return false
		}
	}
	return false
}

// Reject escapes before collecting contexts: finding one safe invocation must
// not hide another invocation through a returned, stored, or opaque callback.
func callbackContextInstructionSafe(instruction ssa.Instruction) bool {
	switch instruction := instruction.(type) {
	case *ssa.Go, *ssa.Send:
		return false
	case *ssa.MakeInterface:
		if callbackStorageType(instruction.X.Type()) {
			return false
		}
	case *ssa.MapUpdate:
		if callbackStorageType(instruction.Value.Type()) {
			return false
		}
	case *ssa.Return:
		for _, result := range instruction.Results {
			if callbackStorageType(result.Type()) {
				return false
			}
		}
	case *ssa.Store:
		if callbackStorageType(instruction.Val.Type()) && !localCallbackAddress(instruction.Addr) {
			return false
		}
	}
	return true
}

func (search *enclosingSearch) instruction(frame *enclosingFrame, instruction ssa.Instruction) bool {
	if !callbackContextInstructionSafe(instruction) {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil {
		return true
	}
	if _, builtin := common.Value.(*ssa.Builtin); builtin {
		return CallMatchesSymbol(common, syntax.Builtin("len"))
	}
	if HasLibraryContract(common, ContractTestingCleanup) || CallMatchesSymbol(common,
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "testing", Receiver: "T", Name: "Run"})) {
		return search.testingCallback(frame, instruction, common)
	}
	callee := common.StaticCallee()
	relevant := callee == nil && !common.IsInvoke() || callee != nil && callee.Parent() != nil
	for _, arg := range common.Args {
		relevant = relevant || callbackStorageType(arg.Type())
	}
	if !relevant {
		return true
	}
	search.search.bindings = frame.bindings
	callees, ok := search.search.boundCallees(instruction)
	if !ok {
		return false
	}
	for _, callee := range callees {
		if callee.function == nil || len(callee.function.Blocks) == 0 {
			return false
		}
		callee.invocation = instruction
		if callee.environment == nil {
			callee.environment = frame.bindings
		}
		search.search.bindings = frame.bindings
		child := &enclosingFrame{function: callee.function, bindings: search.search.bindCallbackArguments(callee), parent: frame, invocation: instruction}
		if !search.walk(child) {
			return false
		}
	}
	return true
}

func (search *enclosingSearch) testingCallback(frame *enclosingFrame, instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if len(common.Args) == 0 {
		return false
	}
	value, ok := resolveCallbackValue(callbackValue{common.Args[len(common.Args)-1], frame.bindings, instruction}, search.request.Budget)
	if !ok {
		return false
	}
	callees, ok := calleesOf(&ssa.CallCommon{Value: value.value}, launchCallback, instruction, false)
	if !ok {
		return false
	}
	for _, callee := range callees {
		callee.environment, callee.invocation = value.bindings, instruction
		search.search.bindings = frame.bindings
		child := &enclosingFrame{
			function: callee.function, bindings: search.search.bindCallbackArguments(callee), parent: frame, invocation: instruction,
		}
		if !search.walk(child) {
			return false
		}
	}
	return true
}

func (search *enclosingSearch) completed(frame *enclosingFrame) bool {
	value, ok := search.resolve(callbackValue{search.request.Value, frame.bindings, nil})
	if !ok {
		return false
	}
	for child := frame; child.parent != nil; child = child.parent {
		parent := child.parent
		if value.Parent() != parent.function {
			continue
		}
		for _, block := range parent.function.Blocks {
			for _, instruction := range block.Instrs {
				if !search.request.Budget.Spend() {
					return false
				}
				if !InstructionDominates(instruction, child.invocation) {
					continue
				}
				if deferred, ok := instruction.(*ssa.Defer); ok && slices.Contains(search.request.Methods, CallName(deferred.Common())) {
					receiver, exact := search.resolve(callbackValue{CallReceiver(deferred.Common()), parent.bindings, instruction})
					if exact && receiver == value {
						return true
					}
				}
				proof := ProveCompletion(CompletionRequest{
					Instruction: instruction, Target: value, Methods: search.request.Methods, Budget: search.request.Budget, ExactTarget: true,
				})
				if proof.Proven() && proof.Reason == EvidenceDeferredCompletion {
					return true
				}
			}
		}
	}
	return false
}

func (search *enclosingSearch) resolve(ref callbackValue) (ssa.Value, bool) { //nolint:ireturn // Preserve SSA identity across invocation frames.
	if ref.value == nil || !search.request.Budget.Spend() {
		return nil, false
	}
	if ref.bindings != nil {
		if next, ok := ref.bindings.values[ref.value]; ok {
			return search.resolve(next)
		}
	}
	if load, ok := ref.value.(*ssa.UnOp); ok && load.Op == token.MUL {
		if field, ok := load.X.(*ssa.FieldAddr); ok {
			return search.resolveField(ref, field)
		}
		cell, ok := load.X.(*ssa.Alloc)
		if !ok {
			return nil, false
		}
		stored, ok := NewStorage(search.request.Budget).stableValue(cell, load)
		if !ok {
			return nil, false
		}
		ref.value = stored
		return search.resolve(ref)
	}
	switch ref.value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Phi:
		return nil, false
	}
	return ref.value, true
}

func (search *enclosingSearch) resolveField(ref callbackValue, field *ssa.FieldAddr) (ssa.Value, bool) { //nolint:ireturn // Preserve SSA identity.
	root := callbackValue{field.X, ref.bindings, ref.observation}
	for root.bindings != nil {
		if !search.request.Budget.Spend() {
			return nil, false
		}
		next, ok := root.bindings.values[root.value]
		if !ok {
			break
		}
		if !search.readOnly(root.value) {
			return nil, false
		}
		root = next
	}
	allocation, ok := root.value.(*ssa.Alloc)
	if !ok || allocation.Referrers() == nil {
		return nil, false
	}
	var stored ssa.Value
	for _, use := range *allocation.Referrers() {
		if !search.request.Budget.Spend() {
			return nil, false
		}
		address, ok := use.(*ssa.FieldAddr)
		if !ok {
			if use == root.observation {
				continue
			}
			return nil, false
		}
		if address.Field != field.Field {
			continue
		}
		value, ok := NewStorage(search.request.Budget).stableValue(address, root.observation)
		if !ok || stored != nil && stored != value {
			return nil, false
		}
		stored = value
	}
	root.value = stored
	return search.resolve(root)
}

func (search *enclosingSearch) readOnly(value ssa.Value) bool {
	if value.Referrers() == nil {
		return false
	}
	for _, use := range *value.Referrers() {
		if !search.request.Budget.Spend() {
			return false
		}
		switch use := use.(type) {
		case *ssa.FieldAddr:
			if use.Referrers() == nil {
				return false
			}
			for _, access := range *use.Referrers() {
				if !search.request.Budget.Spend() {
					return false
				}
				load, ok := access.(*ssa.UnOp)
				if !ok || load.Op != token.MUL {
					return false
				}
			}
		case *ssa.Call:
			callee := use.Common().StaticCallee()
			ok := true
			visited := search.memo.WithFunction(callee, func() {
				for _, binding := range CallBindings(use.Common(), callee, nil) {
					if binding.Supplied == value {
						ok = ok && search.readOnly(binding.Local)
					}
				}
			})
			if !visited || !ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}
