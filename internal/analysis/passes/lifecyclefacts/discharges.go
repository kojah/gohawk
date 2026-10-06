package lifecyclefacts

import (
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// A discharge is one exact cleanup claim: a method called on a parameter, or
// on a path beneath it, on every normal return of the case its condition
// names. The unconditional discharges are the Must cleanup claims every mask
// accessor reads; the others are summary cases, matched only at a call that
// satisfies their condition. Matching maps a discharge's parameter and path
// onto the caller's exact value, so closing j.other never credits the file
// the caller stored in j.out.

// Discharge is one exact cleanup claim: Method is called on the value at
// Path beneath Parameter on every normal return of the case Condition names,
// or every normal return when Condition is empty. Path is a joined access
// path, empty for the parameter itself.
type Discharge struct {
	Condition ssacall.CallCondition
	Parameter int
	Method    string
	Path      string
}

// unconditionalDischarges returns the Must cleanup claims: the discharges
// that hold on every normal return.
func (fact *Fact) unconditionalDischarges() []Discharge {
	var discharges []Discharge
	for _, discharge := range fact.Discharges {
		if discharge.Condition.Unconditional() {
			discharges = append(discharges, discharge)
		}
	}
	return discharges
}

// caseDischarges returns the summary cases: the discharges that hold only
// under a condition a caller must check.
func (fact *Fact) caseDischarges() []Discharge {
	var discharges []Discharge
	for _, discharge := range fact.Discharges {
		if !discharge.Condition.Unconditional() {
			discharges = append(discharges, discharge)
		}
	}
	return discharges
}

// DischargedParameters returns the parameters with any discharge, at any
// path, for a consumer that only asks whether the callee releases part of
// what it was handed.
func (fact *Fact) DischargedParameters() ParameterMask {
	var mask ParameterMask
	for _, discharge := range fact.unconditionalDischarges() {
		if discharge.Method != InvokeMethod && discharge.Method != SynchronousInvokeMethod {
			mask |= parameterMaskFor(discharge.Parameter)
		}
	}
	return mask
}

// InvokeMethod is the discharge method for calling a function parameter
// itself, now or later. SynchronousInvokeMethod is calling it in the same
// goroutine before returning. Neither is a valid Go identifier, so no real
// method matches them.
const (
	InvokeMethod            = "()"
	SynchronousInvokeMethod = "(sync)"
)

// MethodMask returns the parameters on which the callee calls method on the
// parameter itself on every normal return: the empty-path discharges. A
// cleanup of something beneath the parameter is not included.
func (fact *Fact) MethodMask(method string) ParameterMask {
	var mask ParameterMask
	for _, discharge := range fact.unconditionalDischarges() {
		if discharge.Method == method && discharge.Path == "" {
			mask |= parameterMaskFor(discharge.Parameter)
		}
	}
	return mask
}

// InvokedParameters returns the function parameters the callee calls on
// every normal return, whether before it returns or later.
func (fact *Fact) InvokedParameters() ParameterMask {
	return fact.MethodMask(InvokeMethod)
}

// SynchronouslyInvoked returns the function parameters the callee calls in
// the same goroutine before it returns, on every normal return. Calling one
// at all, possibly later, is InvokedParameters instead.
func (fact *Fact) SynchronouslyInvoked() ParameterMask {
	return fact.MethodMask(SynchronousInvokeMethod)
}

// dischargesArgument reports whether the call's static callee is summarized
// as calling method on exactly the target: the target is the argument
// itself for an empty-path discharge, or the value the caller stored at the
// discharge's path beneath the argument. Containment alone proves nothing
// here; that is the whole point of the path.
func (fact *Fact) dischargesArgument(instruction ssa.Instruction, target ssa.Value, method string, observer proofs.Observer) bool {
	return dischargesMatch(fact.unconditionalDischarges(), instruction, target, method, observer)
}

// caseDischargesArgument is dischargesArgument for the fact's argument cases
// that the call's constant arguments select, with known fixing the caller's
// own parameters when the call sits in a body searched under constants.
func (fact *Fact) caseDischargesArgument(
	instruction ssa.Instruction, target ssa.Value, method string, known ssacall.FixedValues, observer proofs.Observer,
) bool {
	return dischargesMatch(fact.casesSelectedBy(method, suppliedCondition(instruction, known)), instruction, target, method, observer)
}

func dischargesMatch(discharges []Discharge, instruction ssa.Instruction, target ssa.Value, method string, observer proofs.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, discharge := range discharges {
		if discharge.Method != method || discharge.Parameter >= len(common.Args) {
			continue
		}
		argument := common.Args[discharge.Parameter]
		storage := heapmodel.NewStorage(proofs.NewSearchBudget(proofs.QueryBudget).Observed(observer))
		path := ssaflow.SplitAccessPath(discharge.Path)
		if stored, ok := heapmodel.ValueAtPath(argument, path, instruction); ok && storage.Same(stored, target).Proven() {
			return true
		}
		// A resource that is an owner, such as an http.Response, is released
		// through its cleanup-bearing field: a helper closing resp.Body has
		// released resp. Only a direct resource-typed field qualifies; a
		// deeper path or an ordinary field is not the owner's cleanup.
		if len(path) == 1 && storage.Same(argument, target).Proven() && cleanupFieldPath(target.Type(), path[0]) {
			return true
		}
	}
	return false
}

// cleanupFieldPath reports whether step selects a field of owner whose type
// carries a cleanup obligation of its own.
func cleanupFieldPath(owner types.Type, step string) bool {
	index, ok := strings.CutPrefix(step, "field:")
	if !ok {
		return false
	}
	structure := syntax.PointerStruct(owner)
	if structure == nil {
		return false
	}
	for field := range structure.NumFields() {
		if strconv.Itoa(field) == index {
			_, cleanup := typeCleanup(structure.Field(field).Type())
			return cleanup
		}
	}
	return false
}

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
	return ssapath.EvaluateObligation(ssapath.ObligationFlow{
		Start:       function.Blocks[0].Instrs[0],
		Budget:      budget,
		Instruction: func(ssa.Instruction) ssapath.ObligationAction { return ssapath.ObligationNone },
		Return:      func(*ssa.Return) ssapath.ObligationAction { return ssapath.ObligationNone },
		Edge: func(from, to *ssa.BasicBlock) ssapath.ObligationAction {
			if exits[[2]*ssa.BasicBlock{from, to}] {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationNone
		},
	}) == ssapath.ObligationHonored
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
	callee := ssacall.ResolvedCallee(ssaflow.InstructionCall(instruction))
	if callee == nil || evidence.pass == nil || callee.Pkg == nil || callee.Pkg.Pkg != evidence.pass.Pkg ||
		len(callee.Blocks) == 0 || index >= len(callee.Params) {
		return false
	}
	return slices.ContainsFunc(methods, func(method string) bool {
		return releasesEachElement(callee, callee.Params[index], method)
	})
}

// A helper may release every element of the aggregate it receives inside a
// loop, as client-go's CloseAndRemove closes each of its variadic files:
// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/staging/src/k8s.io/client-go/util/testing/remove_file.go#L25-L39
// No every-return mask can carry that: the loop's exit edge skips the body,
// and which element an iteration releases is decided by iteration rather
// than by the path. The summary records the loop as a separate may-claim so
// an importing caller can treat the call as uncertain instead of untouched.
// It is never evidence that any particular element was released.

// releasesDerivedValueInLoop reports whether the function calls a lifecycle
// cleanup method, inside a block in a cycle, on a value derived from the
// parameter.
func releasesDerivedValueInLoop(function *ssa.Function, parameter ssa.Value) bool {
	for _, block := range function.Blocks {
		if blockReleasesDerivedValue(block, parameter) && cfg.BlockInCycle(block) {
			return true
		}
	}
	return false
}

func blockReleasesDerivedValue(block *ssa.BasicBlock, parameter ssa.Value) bool {
	for _, instruction := range block.Instrs {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		name := ssaflow.CallName(common)
		if slices.Contains(cleanupMethods, name) &&
			heapmodel.ValueDerivesFrom(ssaflow.CallReceiver(common), parameter) {
			return true
		}
	}
	return false
}
