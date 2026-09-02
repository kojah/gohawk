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
	ReceiverStore ParameterMask
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
	return lines
}

func (fact *Fact) parameterMasks(index int) []string {
	var names []string
	for _, mask := range []struct {
		name string
		mask ParameterMask
	}{
		{"Invoked", fact.Invoked},
		{"Closed", fact.Closed},
		{"Finalized", fact.Finalized},
		{"Released", fact.Released},
		{"Shutdown", fact.Shutdown},
		{"Stopped", fact.Stopped},
		{"Waited", fact.Waited},
		{"Committed", fact.Committed},
		{"RolledBack", fact.RolledBack},
		{"ReturnedOwner", fact.ReturnedOwner},
		{"ReceiverStore", fact.ReceiverStore},
	} {
		if mask.mask.contains(index) {
			names = append(names, mask.name)
		}
	}
	return names
}

func (*Fact) AFact() {}

func (*Fact) String() string { return "lifecycle ownership summary" }

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

// MethodMask selects the parameter mask for a lifecycle method.
func (fact *Fact) MethodMask(method string) ParameterMask {
	switch method {
	case "Close":
		return fact.Closed
	case "Finalize":
		return fact.Finalized
	case "Release":
		return fact.Released
	case "Shutdown":
		return fact.Shutdown
	case "Stop":
		return fact.Stopped
	case "Wait":
		return fact.Waited
	case "Commit":
		return fact.Committed
	case "Rollback":
		return fact.RolledBack
	default:
		return 0
	}
}
