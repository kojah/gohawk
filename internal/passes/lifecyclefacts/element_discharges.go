package lifecyclefacts

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// EachElementPath is the discharge path meaning every element of a slice
// parameter. A helper such as closeAll(files) earns Discharge{Parameter: 0,
// Method: "Close", Path: EachElementPath} when it closes each element of the
// slice it is handed on every normal return, so a caller that passes its own
// collection whole has released every resource in it.
//
// The claim is narrow on purpose. The parameter's only uses are len and cap
// and range loops that release the element each iteration reads, as
// lifecycle.ElementLoopReleasesEach decides; and every normal return follows
// one of those loops running to completion. A loop that breaks or returns
// early, a release of only some elements, a release through a callback, or
// any other use of the slice, such as keeping, appending to, or returning
// it, earns nothing. Index loops, maps, and element paths deeper than one
// level are not modelled.
const EachElementPath = "index:*"

// releasesEachElement reports whether the function calls method on every
// element of the slice parameter on every normal return.
func releasesEachElement(function *ssa.Function, parameter ssa.Value, method string) bool {
	if _, ok := parameter.Type().Underlying().(*types.Slice); !ok || parameter.Referrers() == nil {
		return false
	}
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	exits := map[[2]*ssa.BasicBlock]bool{}
	for _, user := range *parameter.Referrers() {
		switch typed := user.(type) {
		case *ssa.DebugRef:
		case *ssa.Call:
			builtin, ok := typed.Call.Value.(*ssa.Builtin)
			if !ok || builtin.Name() != "len" && builtin.Name() != "cap" {
				return false
			}
		case *ssa.IndexAddr:
			exit, ok := elementReleaseExit(function, typed, method, budget)
			if !ok {
				return false
			}
			exits[exit] = true
		default:
			return false
		}
	}
	if len(exits) == 0 || len(function.Blocks) == 0 || len(function.Blocks[0].Instrs) == 0 {
		return false
	}
	return ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
		Start:       function.Blocks[0].Instrs[0],
		Budget:      budget,
		Instruction: func(ssa.Instruction) ssaflow.ObligationAction { return ssaflow.ObligationNone },
		Return:      func(*ssa.Return) ssaflow.ObligationAction { return ssaflow.ObligationNone },
		Edge: func(from, to *ssa.BasicBlock) ssaflow.ObligationAction {
			if exits[[2]*ssa.BasicBlock{from, to}] {
				return ssaflow.ObligationExact
			}
			return ssaflow.ObligationNone
		},
	}) == ssaflow.ObligationHonored
}

// elementReleaseExit returns the exit edge of the range loop whose element
// read is address, when that loop releases each element.
func elementReleaseExit(function *ssa.Function, address *ssa.IndexAddr, method string, budget *proofs.SearchBudget) ([2]*ssa.BasicBlock, bool) {
	for _, header := range function.Blocks {
		loop, ok := ssaflow.RangeElementLoop(header, budget)
		if ok && loop.ReadsElement(address) && lifecycle.ElementLoopReleasesEach(loop, address, []string{method}) {
			return [2]*ssa.BasicBlock{loop.Loop.Header, loop.Done}, true
		}
	}
	return [2]*ssa.BasicBlock{}, false
}

// ReleasesEachElement reports whether the call's static callee releases,
// with one of methods, every element of the slice argument at index on every
// normal return: by its summary, or, for a callee of this package that has
// none because it is not exported, by the same proof over its body.
func (evidence *LifecycleEvidence) ReleasesEachElement(instruction ssa.Instruction, index int, methods []string) bool {
	if fact, ok := factFor(evidence.pass, instruction); ok {
		return slices.ContainsFunc(fact.unconditionalDischarges(), func(discharge Discharge) bool {
			return discharge.Parameter == index && discharge.Path == EachElementPath && slices.Contains(methods, discharge.Method)
		})
	}
	callee := ssaflow.ResolvedCallee(ssaflow.InstructionCall(instruction))
	if callee == nil || evidence.pass == nil || callee.Pkg == nil || callee.Pkg.Pkg != evidence.pass.Pkg ||
		len(callee.Blocks) == 0 || index >= len(callee.Params) {
		return false
	}
	return slices.ContainsFunc(methods, func(method string) bool {
		return releasesEachElement(callee, callee.Params[index], method)
	})
}
