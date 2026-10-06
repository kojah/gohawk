package lifecyclefacts

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Returned-view inference distinguishes borrowed wrappers from returned owners.
// It combines exact returned-parameter evidence with cleanup capabilities and
// released-field summaries. Type-only view claims stay separate from claims
// that need a readable stored-field relationship.

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
				heapmodel.ValueDerivesFrom(assertion.X, parameter) &&
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
		name := selection.Obj().Name()
		if name == "Cancel" {
			return true
		}
		if slices.Contains(cleanupMethods, name) {
			return true
		}
	}
	return false
}

// viewsFromResultsAlone narrows every returned owner to a view when no result
// of the function can release anything. It is the answer for a result that is
// not a struct, such as an interface, where there is no field to attribute the
// obligation to.
func viewsFromResultsAlone(function *ssa.Function, fact Fact) ParameterMask {
	results := function.Signature.Results()
	if results.Len() == 0 {
		return 0
	}
	for result := range results.Variables() {
		// One result that can release is enough for the obligation to have
		// somewhere to go, and deciding which one needs the field analysis
		// this path does not have.
		if typeCanRelease(result.Type()) {
			return 0
		}
	}
	var views ParameterMask
	for index, parameter := range function.Params {
		if !fact.ReturnedOwner().contains(index) {
			continue
		}
		// A constructor that released the argument itself leaves the caller
		// nothing to keep, exactly as in the struct case.
		if parameterMayBeReleased(function, parameter) {
			continue
		}
		views |= parameterMaskFor(index)
	}
	return views
}

// returnedViews narrows the function's ReturnedOwner mask to the parameters
// that are stored in the returned struct but that no method of the result
// type releases: the result is a view over the caller's resource, and the
// caller keeps the obligation. A parameter of a type this vocabulary does not
// know is never a view, because there is no obligation to keep. Method
// summaries come from this package's own summaries or from imported facts.
func returnedViews(pass *analysis.Pass, function *ssa.Function, fact Fact, summaries Summaries) ParameterMask {
	if fact.ReturnedOwner() == 0 {
		return 0
	}
	structure, resultIndex, ok := returnedStruct(function)
	if !ok {
		// The result does not have to be a struct to answer this. A struct is
		// only needed to ask which field holds the parameter and whether some
		// method releases that field; when no result can release anything at
		// all, there is nothing to ask and the caller keeps the obligation.
		// zap's AddSync hands its argument back as a WriteSyncer, which has
		// Write and Sync but no Close, so a file passed to it stays the
		// caller's to close.
		return viewsFromResultsAlone(function, fact)
	}
	result := function.Signature.Results().At(resultIndex).Type()
	var released FieldMask
	methodsKnown := true
	for _, method := range resultMethods(function) {
		summary, ok := summaries[method]
		if !ok {
			imported, found := factForFunction(pass, method)
			if !found {
				// Private methods have no exported fact, but a visible body is
				// not opaque. Reuse the same receiver-field release inference;
				// its conservative completion cutoffs also apply to this path.
				if object := method.Object(); object != nil && !object.Exported() && len(method.Blocks) > 0 {
					released |= releasedFields(pass, method)
					continue
				}
				methodsKnown = false
				continue
			}
			summary = imported
		}
		released |= summary.Must.ReleasedFields
	}
	var views ParameterMask
	for index, parameter := range function.Params {
		if !fact.ReturnedOwner().contains(index) {
			continue
		}
		if parameterIsView(function, parameter, result, structure, released, methodsKnown) {
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
	released FieldMask,
	methodsKnown bool,
) bool {
	// Returning the same value under the same static type preserves the
	// caller-visible owner rather than hiding it behind a view. This is the
	// common status-checking shape: an error response may be closed locally,
	// while a successful response is handed back unchanged. Requiring every
	// normal return and the same type keeps wrappers and interface erasure out
	// of this exception.
	if parameterReturnedUnchangedOnEveryReturn(function, parameter) {
		return false
	}
	// A constructor that released the argument itself leaves the caller
	// nothing to keep, whatever becomes of the field afterwards.
	if parameterMayBeReleased(function, parameter) {
		return false
	}
	// A callee cannot release what it was handed if the parameter's own type
	// offers no way to. compress/gzip takes an io.Reader and documents that
	// its Close does not close it, which is the standard convention: a wrapper
	// closes what it constructed, never what it was given, and taking a plain
	// reader rather than a ReadCloser is the API saying so. Asking the
	// returned type instead gets that case wrong, because *gzip.Reader has a
	// Close that closes its own decompressor.
	if !typeCanRelease(parameter.Type()) {
		return true
	}
	if !typeCanRelease(result) {
		return true
	}
	// A missing receiver-method summary cannot prove that a stored field is
	// never released. Type-only view evidence above does not depend on that
	// field census; the field-based claim requires complete method knowledge.
	if !methodsKnown {
		return false
	}
	for _, field := range storedFieldIndices(parameter, structure) {
		if !released.contains(field) {
			return true
		}
	}
	return false
}

func parameterReturnedUnchangedOnEveryReturn(function *ssa.Function, parameter ssa.Value) bool {
	for index := range function.Signature.Results().Len() {
		if lifecycle.ReturnsParameterUnchanged(function, parameter, index) {
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
	return ok && fact.ReturnsView(instruction, target)
}

// ReturnsView binds this declaration's returned-view mask to the supplied
// call and target using the same argument policy as lifecycle evidence.
func (fact *Fact) ReturnsView(instruction ssa.Instruction, target ssa.Value) bool {
	return fact.ProveReturnsViewWithin(instruction, target, nil).Proven()
}

// ProveReturnsViewWithin binds the returned-view mask using existing exact
// storage and guarded containment policy. Visits share budget; storage keeps
// its QueryBudget cap. Cutoff is unknown, never evidence of a non-view.
func (fact *Fact) ProveReturnsViewWithin(instruction ssa.Instruction, target ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
	return proveFactOwnsArgumentWithin(instruction, target, fact.Must.ReturnedView, nil, budget)
}
