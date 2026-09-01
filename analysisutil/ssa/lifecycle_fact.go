package ssautil

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// LifecycleFact is the compact cross-package ownership summary exported for a
// function. Each bit identifies an SSA parameter position. The analysisutil
// package is internal infrastructure; this is not a public extension API.
type LifecycleFact struct {
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

// ParameterMaskFor returns the mask containing index, or an empty mask when
// index cannot be represented by a lifecycle summary.
func ParameterMaskFor(index int) ParameterMask {
	if index < 0 || index >= 64 {
		return 0
	}
	return ParameterMask(1) << index
}

// Contains reports whether mask contains index.
func (mask ParameterMask) Contains(index int) bool {
	return mask&ParameterMaskFor(index) != 0
}

// LifecycleSummaries memoizes source-visible summaries for one package pass.
type LifecycleSummaries map[*ssa.Function]LifecycleFact

func (*LifecycleFact) AFact() {}

func (fact *LifecycleFact) String() string { return "lifecycle ownership summary" }

// ImportLifecycleFact imports the summary attached to a static callee.
func ImportLifecycleFact(pass *analysis.Pass, instruction ssa.Instruction) (LifecycleFact, bool) {
	common := InstructionCall(instruction)
	if pass == nil || common == nil || common.StaticCallee() == nil {
		return LifecycleFact{}, false
	}
	object := common.StaticCallee().Object()
	if object == nil {
		return LifecycleFact{}, false
	}
	var fact LifecycleFact
	return fact, pass.ImportObjectFact(object, &fact)
}

// FactOwnsArgument reports whether mask covers the argument which contains
// target at this callsite.
func FactOwnsArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if !mask.Contains(index) {
			continue
		}
		if SameValue(argument, target) || ValueContainsValue(argument, target) {
			return true
		}
	}
	return false
}

// MethodMask selects the parameter mask for a lifecycle method.
func (fact LifecycleFact) MethodMask(method string) ParameterMask {
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
