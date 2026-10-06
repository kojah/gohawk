package lifecyclefacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Field summaries let a user-defined type carry a resource obligation across
// packages. A constructor exports OwnedFields: the fields of its returned
// struct that, on every successful return, hold a resource the function
// acquired itself, as a call result rather than a parameter. A method exports
// ReleasedFields: the receiver fields whose resource cleanup it calls on every
// return. A consumer that sees a constructor with OwnedFields and a method
// whose ReleasedFields covers them synthesizes the contract "result of the
// constructor, released by that method". Either half alone proves nothing:
// a wrapper that stores a caller's file is not an owner, and an owner whose
// type never releases the field has no cleanup the caller could be asked for.
// Positions are struct field indices, so the 64-position mask cap applies.

// returnedStruct returns the struct type behind the function's first
// non-error pointer result, with that result's index.
func returnedStruct(function *ssa.Function) (*types.Struct, int, bool) {
	results := function.Signature.Results()
	for index := range results.Len() {
		if structure := syntax.PointerStruct(results.At(index).Type()); structure != nil {
			return structure, index, true
		}
	}
	return nil, 0, false
}

// ownedFields returns the mask of returned struct fields that hold, on every
// successful return, a resource value acquired in this function.
func ownedFields(pass *analysis.Pass, function *ssa.Function) FieldMask {
	structure, _, ok := returnedStruct(function)
	if !ok {
		return 0
	}
	var owned FieldMask
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			acquired, ok := instruction.(ssa.Value)
			if !ok || !acquiredResource(pass, acquired) || retainedOutsideResult(function, acquired) {
				continue
			}
			for _, index := range storedFieldIndices(acquired, structure) {
				if returnedOwnerOnEveryReturn(pass, function, acquired) {
					owned |= fieldMaskFor(index)
				}
			}
		}
	}
	return owned
}

// A constructor can return a handle while a manager also retains the same
// resource. That does not establish an independent cleanup duty for every
// caller receiving the handle. Keep fresh-result ownership unknown after a
// positive store into an already external owner; a local scratch map does not
// qualify. Possible containment suffices here because it only removes a claim.
// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/chunk/filemgr.go#L101-L130
func retainedOutsideResult(function *ssa.Function, resource ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if lifecycle.StoresValueInOwnedMap(instruction, resource) ||
				lifecycle.StoresOwnerOfValueInExternalField(instruction, resource) {
				return true
			}
		}
	}
	return false
}

// acquiredResource reports whether the value is the result of a call in this
// function whose concrete type belongs to the known resource vocabulary.
// Parameters, loads, and globals are excluded: a resource that arrived from
// elsewhere is borrowed.
func acquiredResource(pass *analysis.Pass, value ssa.Value) bool {
	var call *ssa.Call
	switch typed := value.(type) {
	case *ssa.Call:
		call = typed
	case *ssa.Extract:
		call, _ = typed.Tuple.(*ssa.Call)
	}
	if call == nil {
		return false
	}
	// A custom Close method proves only that cleanup can be requested, not
	// that construction acquired anything. Lazy handles may acquire later.
	// Restrict positive acquisition evidence to the concrete resource vocabulary;
	// nested custom owners remain unknown rather than inheriting a guessed duty.
	// https://github.com/prometheus-community/postgres_exporter/blob/e7e2095249dc369d943af1cef3d8c615228278ec/collector/instance.go#L31-L47
	_, cleanup := ResourceCleanup(value.Type())
	return cleanup && !returnsExistingResource(pass, call)
}

// A cleanup-bearing result can wrap an existing resource rather than acquire
// another one. Returned-owner evidence makes fresh ownership uncertain; it
// does not claim closing the input performs the wrapper's own finalization.
// The summary does not relate individual results to arguments: a helper that
// returns both a borrowed wrapper and a fresh resource therefore also declines
// this inference. Direct acquisition checks inside that helper still apply.
// Explicit acquisition contracts (such as gzip.Writer) remain independent.
// https://github.com/decke/smtprelay/blob/0df87cab3b0f25c33f82eb7da6a55d7159ccf482/smtp.go#L73-L89
func returnsExistingResource(pass *analysis.Pass, call *ssa.Call) bool {
	callee := call.Common().StaticCallee()
	if callee == nil {
		return false
	}
	fact, imported := importFact(pass, call)
	for index, argument := range call.Common().Args {
		if _, cleanup := typeCleanup(argument.Type()); !cleanup {
			continue
		}
		if imported && fact.ReturnedOwner().contains(index) {
			return true
		}
		if len(callee.Blocks) > 0 && index < len(callee.Params) &&
			returnedOwnerOnEveryReturn(pass, callee, callee.Params[index]) {
			return true
		}
	}
	return false
}

// typeCleanup returns the methods that release a value of this type. The
// listed resource types answer for a concrete value, but Go says an obligation
// through an interface just as often: a field holding what a wrapper was given
// is an io.Closer, and a constructor hands back an io.ReadCloser rather than
// naming the thing it opened. io.Closer is as much a statement that the value
// can be closed as *os.File is.
func typeCleanup(value types.Type) ([]string, bool) {
	if cleanup, ok := ResourceCleanup(value); ok {
		return cleanup, true
	}
	// Only the closing contract is taken from a method set. It is documented
	// as io.Closer and carries a signature to check, where a bare Stop or
	// Shutdown on a project type would be a guess about what the name means.
	if implementsCloser(value) {
		return []string{"Close"}, true
	}
	return nil, false
}

func implementsCloser(value types.Type) bool {
	for selection := range types.NewMethodSet(value).Methods() {
		method, ok := selection.Obj().(*types.Func)
		if !ok || method.Name() != "Close" {
			continue
		}
		signature, ok := method.Type().(*types.Signature)
		if !ok || signature.Params().Len() != 0 || signature.Results().Len() != 1 {
			continue
		}
		if types.Identical(signature.Results().At(0).Type(), types.Universe.Lookup("error").Type()) {
			return true
		}
	}
	return false
}

// storedFieldIndices returns the indices of the struct's fields the value is
// stored into, through field addresses of an allocation of that struct.
func storedFieldIndices(value ssa.Value, structure *types.Struct) []int {
	return storedFieldIndicesVia(value, structure, ssacall.NewCallGraphMemo[ssa.Value, []int]())
}

// storedFieldIndicesVia follows a delegated store into the callee performing
// it. A constructor commonly hands the value to a helper rather than writing
// the field itself, and reading only this body then finds no field at all,
// which leaves the proof unable to say whether anything releases it. The
// callee writes a field of the same struct, so the index it uses is the index
// here; the struct type is what ties the two together, and a value reaching a
// callee that writes a different type contributes nothing.
func storedFieldIndicesVia(
	value ssa.Value,
	structure *types.Struct,
	memo *ssacall.CallGraphMemo[ssa.Value, []int],
) []int {
	return memo.Compose(value, nil, func() []int {
		if value.Referrers() == nil {
			return nil
		}
		var indices []int
		for _, reference := range *value.Referrers() {
			if store, ok := reference.(*ssa.Store); ok {
				indices = append(indices, directFieldIndex(store, value, structure)...)
				continue
			}
			indices = append(indices, delegatedFieldIndices(reference, value, structure, memo)...)
		}
		return indices
	}, func(_ ssacall.SummaryUnavailable, partial []int) []int {
		return partial
	})
}

func directFieldIndex(store *ssa.Store, value ssa.Value, structure *types.Struct) []int {
	if store.Val != value {
		return nil
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	if !ok {
		return nil
	}
	pointer, ok := field.X.Type().Underlying().(*types.Pointer)
	if ok && types.Identical(pointer.Elem().Underlying(), structure) {
		return []int{field.Field}
	}
	return nil
}

func delegatedFieldIndices(
	reference ssa.Instruction,
	value ssa.Value,
	structure *types.Struct,
	memo *ssacall.CallGraphMemo[ssa.Value, []int],
) []int {
	common := ssaflow.InstructionCall(reference)
	callee := ssacall.ResolvedCallee(common)
	var indices []int
	memo.WithFunction(callee, func() {
		for _, binding := range ssacall.CallBindings(common, callee, nil) {
			if binding.Supplied == value {
				indices = append(indices, storedFieldIndicesVia(binding.Local, structure, memo)...)
			}
		}
	})
	return indices
}

// releasedFields returns the mask of receiver fields whose resource cleanup
// the method calls on every return, directly or through a completion the
// engine can prove for the loaded field.
func releasedFields(pass *analysis.Pass, function *ssa.Function) FieldMask {
	if function.Signature.Recv() == nil || len(function.Params) == 0 {
		return 0
	}
	receiver := function.Params[0]
	structure := syntax.PointerStruct(receiver.Type())
	if structure == nil {
		return 0
	}
	var released FieldMask
	for index := range structure.NumFields() {
		cleanup, ok := typeCleanup(structure.Field(index).Type())
		if !ok {
			continue
		}
		// A panic-only placeholder cannot define the owner's cleanup contract:
		// lack of a normal return does not witness release of any field.
		// https://github.com/talostrading/sonic/blob/fa70f8c39b9eea68e782c4c7f3604fe232d4301c/multicast/peer.go#L262-L264
		if lifecycle.MethodCallCoverage(function, func(instruction ssa.Instruction) bool {
			return releasesField(pass, instruction, receiver, index, cleanup)
		}, lifecycle.CoverageEveryReturn, nil) {
			released |= fieldMaskFor(index)
		}
	}
	return released
}

// releasesField reports whether the instruction discharges the receiver's
// field: a cleanup call whose receiver derives from a load of that field, or
// a completion proven for such a load handed to a helper, defer, or launch.
func releasesField(pass *analysis.Pass, instruction ssa.Instruction, receiver ssa.Value, index int, cleanup []string) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, load := range fieldLoads(receiver, index) {
		for _, method := range cleanup {
			if ssaflow.CallName(common) == method && heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), load) {
				return true
			}
		}
		proof := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
			Instruction: instruction, Target: load, Methods: cleanup, Budget: proofs.NewSearchBudget(proofs.SummaryBudget),
		})
		// An abandoned search counts as a release here, because both readers
		// of ReleasedFields take a clear bit as a positive claim: the
		// returned-view rule turns the constructor's result into a view the
		// caller must still release, and the cleanup contract withholds
		// credit from a caller of this method. Guessing "released" can only
		// hide a diagnostic; guessing "not released" can invent one.
		if proof.Proven() || proof.Reason == proofs.EvidenceBudgetExhausted {
			return true
		}
		if imported, ok := importFact(pass, instruction); ok {
			for _, method := range cleanup {
				if imported.dischargesArgument(instruction, load, method, nil) {
					return true
				}
			}
		}
	}
	return false
}

// fieldLoads returns every load of the receiver's field in the method.
func fieldLoads(receiver ssa.Value, index int) []ssa.Value {
	var loads []ssa.Value
	if receiver.Referrers() == nil {
		return nil
	}
	for _, reference := range *receiver.Referrers() {
		field, ok := reference.(*ssa.FieldAddr)
		if !ok || field.Field != index || field.Referrers() == nil {
			continue
		}
		for _, use := range *field.Referrers() {
			if load, ok := use.(*ssa.UnOp); ok && load.X == field {
				loads = append(loads, load)
			}
		}
	}
	return loads
}

// importResultMethods records the imported summaries of the methods of the
// callee's returned struct type, so a consumer can ask which of them release
// the owned fields.
func importResultMethods(pass *analysis.Pass, callee *ssa.Function, summaries Summaries) {
	for _, method := range resultMethods(callee) {
		if fact, ok := factForFunction(pass, method); ok {
			summaries[method] = fact
		}
	}
}

// resultMethods returns the declared methods of the callee's first pointer
// struct result.
func resultMethods(callee *ssa.Function) []*ssa.Function {
	_, index, ok := returnedStruct(callee)
	if !ok || callee.Pkg == nil {
		return nil
	}
	pointer := callee.Signature.Results().At(index).Type()
	var methods []*ssa.Function
	for selection := range types.NewMethodSet(pointer).Methods() {
		function, ok := selection.Obj().(*types.Func)
		if !ok || function.Pkg() == nil {
			continue
		}
		if method := callee.Prog.LookupMethod(pointer, function.Pkg(), function.Name()); method != nil && method.Object() == function {
			methods = append(methods, method)
		}
	}
	return methods
}

// OwnedResult reports whether the call's static callee is summarized as
// returning a struct that owns resource fields, and returns the methods of
// the result type whose ReleasedFields cover every owned field together with
// the index of that result. A type with no covering method yields false: the
// caller cannot be asked for a cleanup that does not exist.
func (evidence *LifecycleEvidence) OwnedResult(call *ssa.Call) ([]string, int, bool) {
	fact, ok := factFor(evidence.pass, call)
	if !ok || fact.Must.OwnedFields == 0 {
		return nil, 0, false
	}
	callee := call.Common().StaticCallee()
	_, index, ok := returnedStruct(callee)
	if !ok {
		return nil, 0, false
	}
	summaries, _ := evidence.pass.ResultOf[Analyzer].(Summaries)
	var cleanup []string
	for _, method := range resultMethods(callee) {
		if summary, ok := summaries[method]; ok && summary.Must.ReleasedFields&fact.Must.OwnedFields == fact.Must.OwnedFields {
			cleanup = append(cleanup, method.Name())
		}
	}
	reason := reasonOwnedResultContract
	if len(cleanup) == 0 {
		reason = reasonOwnedResultUnreleasable
	}
	evidence.emit(EvidenceRequest{Instruction: call, Target: call}, Proof{Proof: proofs.Proof{
		State: proofs.EvidenceProven, Provenance: proofs.EvidenceFromImportedFact,
	}, SummaryReason: reason})
	return cleanup, index, len(cleanup) > 0
}
