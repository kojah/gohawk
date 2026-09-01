package lifecyclefacts

import (
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"

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

// summarySet memoizes source-visible summaries for one package pass.
type summarySet map[*ssa.Function]Fact

func (*Fact) AFact() {}

func (*Fact) String() string { return "lifecycle ownership summary" }

// importFact imports the summary attached to a static callee.
func importFact(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	common := ssautil.InstructionCall(instruction)
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
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if !mask.contains(index) {
			continue
		}
		if ssautil.SameValue(argument, target) || ssautil.ValueContainsValue(argument, target) {
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
	default:
		return 0
	}
}
