package lifecyclefacts

// Cleanup contracts for project-defined types.
//
// Only the closing contract can be read from a method set, because io.Closer
// is documented and carries a signature to check. A bare Stop, Shutdown, or
// Release on a project type is a guess about what the name means, and the
// catalog of standard resource types cannot grow to cover every project.
//
// What is not a guess is the release itself, and it is visible in the package
// that DEFINES the type: a constructor takes ownership of resource fields, and
// a method releases them. Both are already proved, as OwnedFields on the
// constructor and ReleasedFields on the method. This file joins them onto the
// type and exports the result, so a method name is only ever a label on
// evidence the analysis already holds rather than the evidence itself.
//
// The contract must be complete. A method that releases some of what the
// constructor owns leaves the rest outstanding, and reading a partial release
// as a full one would discharge an obligation that still stands. A type whose
// methods could not be summarized therefore claims nothing, which is the same
// answer as a type that releases nothing: absence is not disproof.

import (
	"fmt"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// CleanupFact records that a named type releases what it owns, and which of
// its methods do it. It is exported for the type, not for a function, and it
// carries a proof forward, so it is only exported when the release is exact.
type CleanupFact struct {
	// Methods release the owned fields. A method earns a place here by
	// releasing them, never by being called Stop or Shutdown.
	Methods []string
	// Owned is the field mask a constructor of this type took ownership of.
	Owned ParameterMask
	// Released is what Methods cover together. The contract holds only when
	// Released covers Owned.
	Released ParameterMask
}

// AFact marks CleanupFact as an analysis fact.
func (fact *CleanupFact) AFact() {}

func (fact *CleanupFact) String() string {
	return fmt.Sprintf("cleanup(%s)", strings.Join(fact.Methods, ","))
}

// DescribeFact renders the contract for the fact dump, naming the fields the
// methods release so a reader can check the claim against the struct.
func (fact *CleanupFact) DescribeFact(object types.Object) []string {
	name, ok := object.(*types.TypeName)
	if !ok {
		return nil
	}
	structure, ok := name.Type().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	var fields []string
	for index := range structure.NumFields() {
		if fact.Owned.contains(index) {
			fields = append(fields, structure.Field(index).Name())
		}
	}
	return []string{fmt.Sprintf("cleanup: %s releases %s", strings.Join(fact.Methods, ", "), strings.Join(fields, ", "))}
}

// exportCleanupContracts joins each constructor's owned fields to the methods
// of the type it returns, and exports the contract when the methods cover
// every owned field.
//
// It runs after every function in the package is summarized, for the same
// reason returnedViews does: the releasing method usually lives beside the
// constructor, so neither half of the contract is available until both are.
func exportCleanupContracts(pass *analysis.Pass, summaries Summaries) {
	owned := map[*types.TypeName]constructed{}
	for function, fact := range summaries {
		if fact.OwnedFields == 0 {
			continue
		}
		// A constructor in another package cannot be the place this contract
		// is proved, because its type's methods are not summarized here.
		name, ok := constructedTypeName(function)
		if !ok || name.Pkg() != pass.Pkg {
			continue
		}
		entry := owned[name]
		entry.fields |= fact.OwnedFields
		entry.constructor = function
		owned[name] = entry
	}
	for name, entry := range owned {
		if !name.Exported() {
			continue
		}
		ownedFields := entry.fields
		released, methods := releasingMethods(name, entry.constructor, summaries)
		// An incomplete release is not a contract: the caller would be told the
		// obligation is discharged while part of it still stands.
		if len(methods) == 0 || ownedFields&released != ownedFields {
			continue
		}
		pass.ExportObjectFact(name, &CleanupFact{Methods: methods, Owned: ownedFields, Released: released})
	}
}

// constructedTypeName names the pointer-to-struct type a constructor returns.
func constructedTypeName(function *ssa.Function) (*types.TypeName, bool) {
	_, index, ok := returnedStruct(function)
	if !ok {
		return nil, false
	}
	pointer, ok := function.Signature.Results().At(index).Type().Underlying().(*types.Pointer)
	if !ok {
		return nil, false
	}
	named, ok := pointer.Elem().(*types.Named)
	if !ok {
		return nil, false
	}
	return named.Obj(), true
}

// constructed pairs the fields a type's constructors own with one of those
// constructors, which is how the method lookup reaches the SSA program without
// threading it through every call.
type constructed struct {
	fields      ParameterMask
	constructor *ssa.Function
}

// releasingMethods returns what the type's own methods release together, and
// the names of the methods that release anything. A method with no summary
// contributes nothing rather than counting as releasing nothing.
func releasingMethods(name *types.TypeName, constructor *ssa.Function, summaries Summaries) (ParameterMask, []string) {
	pointer := types.NewPointer(name.Type())
	var released ParameterMask
	var methods []string
	for selection := range types.NewMethodSet(pointer).Methods() {
		function, ok := selection.Obj().(*types.Func)
		if !ok || function.Pkg() == nil {
			continue
		}
		method := constructor.Prog.LookupMethod(pointer, function.Pkg(), function.Name())
		if method == nil {
			continue
		}
		summary, found := summaries[method]
		if !found || summary.ReleasedFields == 0 {
			continue
		}
		released |= summary.ReleasedFields
		methods = append(methods, function.Name())
	}
	slices.Sort(methods)
	return released, slices.Compact(methods)
}
