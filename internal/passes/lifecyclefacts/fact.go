package lifecyclefacts

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Fact is the compact cross-package ownership summary exported for a
// function. Each bit identifies an SSA parameter position. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Invoked       ParameterMask
	Closed        ParameterMask
	Finalized     ParameterMask
	Released      ParameterMask
	Shutdown      ParameterMask
	Stopped       ParameterMask
	Waited        ParameterMask
	Committed     ParameterMask
	RolledBack    ParameterMask
	ReturnedOwner ParameterMask
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See fields.go.
	ReturnedView ParameterMask
	// Retained marks parameters the callee may keep beyond the call; see
	// retention.go for the over-approximation it deliberately makes.
	Retained ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields    ParameterMask
	ReleasedFields ParameterMask
	ReceiverStore  ParameterMask
}

// ParameterMask is a set of SSA parameter positions in a lifecycle summary.
type ParameterMask uint64

// parameterMaskFor returns the mask containing index, or an empty mask when
// index cannot be represented by a lifecycle summary.
func parameterMaskFor(index int) ParameterMask {
	if index < 0 || index >= 64 {
		return 0
	}
	return ParameterMask(1) << index
}

// contains reports whether mask contains index.
func (mask ParameterMask) contains(index int) bool {
	return mask&parameterMaskFor(index) != 0
}

// Summaries is the pass result: the summary of every exported source function
// in the package plus the imported summary of every static callee.
type Summaries map[*ssa.Function]Fact

// DescribeFact renders the summary for the fact dump: one line per parameter
// that some mask covers, named from the function's signature. Mask positions
// follow SSA parameters, so a method's receiver is position zero.
func (fact *Fact) DescribeFact(object types.Object) []string {
	function, ok := object.(*types.Func)
	if !ok {
		return nil
	}
	var names []string
	signature := function.Signature()
	if signature.Recv() != nil {
		names = append(names, signature.Recv().Name())
	}
	for parameter := range signature.Params().Variables() {
		names = append(names, parameter.Name())
	}
	var lines []string
	for index, name := range names {
		if masks := fact.parameterMasks(index); len(masks) > 0 {
			lines = append(lines, fmt.Sprintf("%d %s: %s", index, name, strings.Join(masks, ", ")))
		}
	}
	for _, mask := range fieldMasks {
		if fields := fact.fieldNames(*mask.field(fact), signature); len(fields) > 0 {
			lines = append(lines, fmt.Sprintf("%s: %s", mask.name, strings.Join(fields, ", ")))
		}
	}
	return lines
}

// fieldNames renders a field mask against the method's receiver struct or,
// for a function, the struct behind its first pointer result.
func (fact *Fact) fieldNames(mask ParameterMask, signature *types.Signature) []string {
	if mask == 0 {
		return nil
	}
	var structure *types.Struct
	if signature.Recv() != nil {
		structure = structBehind(signature.Recv().Type())
	} else {
		for result := range signature.Results().Variables() {
			if structure = structBehind(result.Type()); structure != nil {
				break
			}
		}
	}
	if structure == nil {
		return nil
	}
	var names []string
	for index := range structure.NumFields() {
		if mask.contains(index) {
			names = append(names, structure.Field(index).Name())
		}
	}
	return names
}

func structBehind(value types.Type) *types.Struct {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, _ := pointer.Elem().Underlying().(*types.Struct)
	return structure
}

func (fact *Fact) parameterMasks(index int) []string {
	var names []string
	for _, mask := range lifecycleMasks {
		if mask.field(fact).contains(index) {
			names = append(names, mask.name)
		}
	}
	return names
}

func (*Fact) AFact() {}

// String decodes the masks by parameter position so the fact is readable in
// analysis debug output.
func (fact *Fact) String() string {
	var parts []string
	for index := range 64 {
		if masks := fact.parameterMasks(index); len(masks) > 0 {
			parts = append(parts, fmt.Sprintf("%d:%s", index, strings.Join(masks, "+")))
		}
	}
	for _, mask := range fieldMasks {
		if value := *mask.field(fact); value != 0 {
			parts = append(parts, fmt.Sprintf("%s:%#x", mask.name, uint64(value)))
		}
	}
	if len(parts) == 0 {
		return "lifecycle summary: none"
	}
	return "lifecycle summary: " + strings.Join(parts, " ")
}

// importFact imports the summary attached to a static callee.
func importFact(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	common := ssaflow.InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return Fact{}, false
	}
	object := common.StaticCallee().Object()
	if object == nil {
		return Fact{}, false
	}
	var fact Fact
	return fact, pass.ImportObjectFact(object, &fact)
}

// factOwnsArgument reports whether mask covers the argument which contains
// target at this callsite.
func factOwnsArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if !mask.contains(index) {
			continue
		}
		if ssaflow.SameValue(argument, target) || ssaflow.ValueContainsValue(argument, target) {
			return true
		}
	}
	return false
}

func factOwnsProjectedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if mask.contains(index) && ssaflow.UnmodifiedNonEmptyAccessPathAt(argument, target, instruction) {
			return true
		}
	}
	return false
}

// lifecycleMask names one mask and, for method masks, the lifecycle method
// whose call on every return sets it. This table is the one catalog shared by
// summarization, imported-mask selection, and the fact dump.
type lifecycleMask struct {
	name   string
	method string
	field  func(*Fact) *ParameterMask
}

var lifecycleMasks = []lifecycleMask{
	{name: "Invoked", field: func(fact *Fact) *ParameterMask { return &fact.Invoked }},
	{name: "Closed", method: "Close", field: func(fact *Fact) *ParameterMask { return &fact.Closed }},
	{name: "Finalized", method: "Finalize", field: func(fact *Fact) *ParameterMask { return &fact.Finalized }},
	{name: "Released", method: "Release", field: func(fact *Fact) *ParameterMask { return &fact.Released }},
	{name: "Shutdown", method: "Shutdown", field: func(fact *Fact) *ParameterMask { return &fact.Shutdown }},
	{name: "Stopped", method: "Stop", field: func(fact *Fact) *ParameterMask { return &fact.Stopped }},
	{name: "Waited", method: "Wait", field: func(fact *Fact) *ParameterMask { return &fact.Waited }},
	{name: "Committed", method: "Commit", field: func(fact *Fact) *ParameterMask { return &fact.Committed }},
	{name: "RolledBack", method: "Rollback", field: func(fact *Fact) *ParameterMask { return &fact.RolledBack }},
	{name: "ReturnedOwner", field: func(fact *Fact) *ParameterMask { return &fact.ReturnedOwner }},
	{name: "ReturnedView", field: func(fact *Fact) *ParameterMask { return &fact.ReturnedView }},
	{name: "ReceiverStore", field: func(fact *Fact) *ParameterMask { return &fact.ReceiverStore }},
	{name: "Retained", field: func(fact *Fact) *ParameterMask { return &fact.Retained }},
}

// fieldMasks are indexed by struct field of the result or receiver type.
var fieldMasks = []lifecycleMask{
	{name: "OwnedFields", field: func(fact *Fact) *ParameterMask { return &fact.OwnedFields }},
	{name: "ReleasedFields", field: func(fact *Fact) *ParameterMask { return &fact.ReleasedFields }},
}

// MethodMask selects the parameter mask for a lifecycle method.
func (fact *Fact) MethodMask(method string) ParameterMask {
	for _, mask := range lifecycleMasks {
		if mask.method == method && mask.method != "" {
			return *mask.field(fact)
		}
	}
	return 0
}
