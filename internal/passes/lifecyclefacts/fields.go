package lifecyclefacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

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

// resourceType names a type whose values carry a lifecycle obligation and the
// method that discharges it. This is a type vocabulary, distinct from the
// acquisition contracts an analyzer matches at call sites.
type resourceType struct {
	packagePath string
	name        string
	cleanup     []string
}

func resourceTypes() []resourceType {
	return []resourceType{
		{"os", "File", []string{"Close"}},
		{"database/sql", "Tx", []string{"Commit", "Rollback"}},
		{"database/sql", "Rows", []string{"Close"}},
		{"database/sql", "Stmt", []string{"Close"}},
		{"net/http", "Response", []string{"Close"}},
		{"compress/gzip", "Reader", []string{"Close"}},
		{"compress/gzip", "Writer", []string{"Close"}},
		{"compress/zlib", "Writer", []string{"Close"}},
		{"time", "Ticker", []string{"Stop"}},
		{"time", "Timer", []string{"Stop"}},
	}
}

// ResourceCleanup returns the cleanup methods of a resource type, or false
// when the type carries no obligation this vocabulary knows.
func ResourceCleanup(value types.Type) ([]string, bool) {
	for _, entry := range resourceTypes() {
		if syntax.NamedType(value, entry.packagePath, entry.name) {
			return entry.cleanup, true
		}
	}
	return nil, false
}

// returnedStruct returns the struct type behind the function's first
// non-error pointer result, with that result's index.
func returnedStruct(function *ssa.Function) (*types.Struct, int, bool) {
	results := function.Signature.Results()
	for index := range results.Len() {
		pointer, ok := results.At(index).Type().Underlying().(*types.Pointer)
		if !ok {
			continue
		}
		if structure, ok := pointer.Elem().Underlying().(*types.Struct); ok {
			return structure, index, true
		}
	}
	return nil, 0, false
}

// ownedFields returns the mask of returned struct fields that hold, on every
// successful return, a resource value acquired in this function.
func ownedFields(pass *analysis.Pass, function *ssa.Function) ParameterMask {
	structure, _, ok := returnedStruct(function)
	if !ok {
		return 0
	}
	var owned ParameterMask
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			acquired, ok := instruction.(ssa.Value)
			if !ok || !acquiredResource(acquired) {
				continue
			}
			for _, index := range storedFieldIndices(acquired, structure) {
				if returnedOwnerOnEveryReturn(pass, function, acquired) {
					owned |= parameterMaskFor(index)
				}
			}
		}
	}
	return owned
}

// acquiredResource reports whether the value is the result of a call in this
// function whose type carries an obligation. Parameters, loads, and globals
// are excluded: a resource that arrived from elsewhere is borrowed.
func acquiredResource(value ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.Call:
		_, ok := ResourceCleanup(typed.Type())
		return ok
	case *ssa.Extract:
		_, call := typed.Tuple.(*ssa.Call)
		_, ok := ResourceCleanup(typed.Type())
		return call && ok
	}
	return false
}

// storedFieldIndices returns the indices of the struct's fields the value is
// stored into, through field addresses of an allocation of that struct.
// parameterMayBeReleased reports whether the function releases the parameter
// despite its type offering no way to, by asserting it to a type that carries
// a cleanup method. A callee may know more about its argument than the
// parameter type admits, and without this the rule above would claim the
// caller keeps an obligation the constructor had already discharged.
//
// Handing the parameter to a callee that settles it needs no separate
// question: a callee proven to settle it on every return makes this function
// proven to settle it too, and that is what suppresses the diagnostic.
func parameterMayBeReleased(function *ssa.Function, parameter ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if assertion, ok := instruction.(*ssa.TypeAssert); ok &&
				ssaflow.ValueDerivesFrom(assertion.X, parameter, map[ssa.Value]bool{}) &&
				typeCanRelease(assertion.AssertedType) {
				return true
			}
		}
	}
	return false
}

// typeCanRelease reports whether a value of this type carries any method that
// could release what it holds. The names are the cleanup vocabulary used
// throughout the lifecycle proofs; a type with none of them offers the caller
// no way to release a field, whatever that field holds.
func typeCanRelease(value types.Type) bool {
	for selection := range types.NewMethodSet(value).Methods() {
		switch selection.Obj().Name() {
		case "Cancel", "Close", "Finalize", "Release", "Shutdown", "Stop":
			return true
		}
	}
	return false
}

func storedFieldIndices(value ssa.Value, structure *types.Struct) []int {
	if value.Referrers() == nil {
		return nil
	}
	var indices []int
	for _, reference := range *value.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Val != value {
			continue
		}
		field, ok := store.Addr.(*ssa.FieldAddr)
		if !ok {
			continue
		}
		pointer, ok := field.X.Type().Underlying().(*types.Pointer)
		if ok && types.Identical(pointer.Elem().Underlying(), structure) {
			indices = append(indices, field.Field)
		}
	}
	return indices
}

// releasedFields returns the mask of receiver fields whose resource cleanup
// the method calls on every return, directly or through a completion the
// engine can prove for the loaded field.
func releasedFields(pass *analysis.Pass, function *ssa.Function) ParameterMask {
	if function.Signature.Recv() == nil || len(function.Params) == 0 {
		return 0
	}
	receiver := function.Params[0]
	pointer, ok := receiver.Type().Underlying().(*types.Pointer)
	if !ok {
		return 0
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok {
		return 0
	}
	var released ParameterMask
	for index := range structure.NumFields() {
		cleanup, ok := ResourceCleanup(structure.Field(index).Type())
		if !ok {
			continue
		}
		if !ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
			return releasesField(pass, instruction, receiver, index, cleanup)
		}) {
			released |= parameterMaskFor(index)
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
			if ssaflow.CallName(common) == method && ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), load, map[ssa.Value]bool{}) {
				return true
			}
		}
		if ssaflow.ProveCompletion(ssaflow.CompletionRequest{Instruction: instruction, Target: load, Methods: cleanup}).Proven() {
			return true
		}
		if imported, ok := importFact(pass, instruction); ok {
			for _, method := range cleanup {
				if factOwnsArgument(instruction, load, imported.MethodMask(method)) {
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
	if !ok || fact.OwnedFields == 0 {
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
		if summary, ok := summaries[method]; ok && summary.ReleasedFields&fact.OwnedFields == fact.OwnedFields {
			cleanup = append(cleanup, method.Name())
		}
	}
	reason := reasonOwnedResultContract
	if len(cleanup) == 0 {
		reason = reasonOwnedResultUnreleasable
	}
	evidence.emit(EvidenceRequest{Instruction: call, Target: call}, ssaflow.Proof{
		State: ssaflow.EvidenceProven, Reason: reason, Provenance: ssaflow.EvidenceFromImportedFact,
	})
	return cleanup, index, len(cleanup) > 0
}

// returnedViews narrows the function's ReturnedOwner mask to the parameters
// that are stored in the returned struct but that no method of the result
// type releases: the result is a view over the caller's resource, and the
// caller keeps the obligation. A parameter of a type this vocabulary does not
// know is never a view, because there is no obligation to keep. Method
// summaries come from this package's own summaries or from imported facts.
func returnedViews(pass *analysis.Pass, function *ssa.Function, fact Fact, summaries Summaries) ParameterMask {
	if fact.ReturnedOwner == 0 {
		return 0
	}
	structure, resultIndex, ok := returnedStruct(function)
	if !ok {
		return 0
	}
	result := function.Signature.Results().At(resultIndex).Type()
	var released ParameterMask
	for _, method := range resultMethods(function) {
		summary, ok := summaries[method]
		if !ok {
			imported, found := factForFunction(pass, method)
			if !found {
				continue
			}
			summary = imported
		}
		released |= summary.ReleasedFields
	}
	var views ParameterMask
	for index, parameter := range function.Params {
		if !fact.ReturnedOwner.contains(index) {
			continue
		}
		if parameterIsView(function, parameter, result, structure, released) {
			views |= parameterMaskFor(index)
		}
	}
	return views
}

// parameterIsView reports whether the returned struct keeps the parameter in a
// field that nothing on that type releases.
//
// The field holding the parameter is only needed to ask whether some method
// releases it, so a type carrying no cleanup method at all answers the
// question without it: nothing on it can release anything. That case is the
// one this rule exists for, because a constructor commonly delegates the
// store to a helper and leaves no store to read here, and because neither
// *bufio.Reader nor *json.Decoder closes the reader it was given.
//
// The test is on the type rather than on the computed mask. An empty mask
// also means the methods could not be summarized, and reading that as "this
// releases nothing" would turn a wrapper that does close its argument into a
// view. When the type can release, the answer depends on which field this is,
// so an unreadable store stays conservative and claims no view.
func parameterIsView(
	function *ssa.Function,
	parameter ssa.Value,
	result types.Type,
	structure *types.Struct,
	released ParameterMask,
) bool {
	// A callee cannot release what it was handed if the parameter's own type
	// offers no way to. compress/gzip takes an io.Reader and documents that
	// its Close does not close it, which is the standard convention: a wrapper
	// closes what it constructed, never what it was given, and taking a plain
	// reader rather than a ReadCloser is the API saying so. Asking the
	// returned type instead gets that case wrong, because *gzip.Reader has a
	// Close that closes its own decompressor.
	if !typeCanRelease(parameter.Type()) && !parameterMayBeReleased(function, parameter) {
		return true
	}
	if !typeCanRelease(result) {
		return true
	}
	for _, field := range storedFieldIndices(parameter, structure) {
		if !released.contains(field) {
			return true
		}
	}
	return false
}

// ArgumentReturnedAsView reports whether the call's static callee is
// summarized as returning a view over the argument that contains target: the
// argument is stored in the returned struct and nothing on that type releases
// it. The proof outranks a lifecycle-looking method name on the result type.
func (evidence *LifecycleEvidence) ArgumentReturnedAsView(instruction ssa.Instruction, target ssa.Value) bool {
	return CallReturnsView(evidence.pass, instruction, target)
}

// CallReturnsView is ArgumentReturnedAsView for callers that hold the pass
// rather than an evidence context.
func CallReturnsView(pass *analysis.Pass, instruction ssa.Instruction, target ssa.Value) bool {
	fact, ok := factFor(pass, instruction)
	return ok && factOwnsArgument(instruction, target, fact.ReturnedView)
}

// ArgumentRetainedByCallee reports whether the call's static callee is
// summarized as keeping the argument that contains target somewhere other
// than its returned value: a logger sink, a registry, a receiver field. The
// callee then owns the value's release. A parameter kept only inside the
// returned aggregate is decided by the returned-owner and view rules instead.
func (evidence *LifecycleEvidence) ArgumentRetainedByCallee(instruction ssa.Instruction, target ssa.Value) bool {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok || !factOwnsExactArgument(instruction, target, fact.Stored&^fact.ReturnedOwner) {
		return false
	}
	evidence.emit(EvidenceRequest{Instruction: instruction, Target: target}, ssaflow.Proof{
		State: ssaflow.EvidenceProven, Reason: reasonStoredByCallee, Provenance: ssaflow.EvidenceFromImportedFact,
	})
	return true
}

// CalleeClaims reports what the call's static callee is summarized as doing
// with the argument at index. The second result separates a callee proven not
// to do it from one that carries no summary at all, which a proof must decide
// about for itself: a rule looking for evidence that an obligation was
// discharged must not read silence as proof that it was not.
func (evidence *LifecycleEvidence) CalleeClaims(
	instruction ssa.Instruction,
	index int,
	claim Claim,
) (holds bool, known bool) {
	fact, ok := factFor(evidence.pass, instruction)
	if !ok {
		return false, false
	}
	return fact.Claim(claim).contains(index), true
}

// CalleeSummarized reports whether the call's static callee carries a
// lifecycle summary, so a consumer can distinguish a callee proven to do
// nothing with an argument from one it knows nothing about.
func (evidence *LifecycleEvidence) CalleeSummarized(instruction ssa.Instruction) bool {
	_, ok := factFor(evidence.pass, instruction)
	return ok
}
