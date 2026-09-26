package lifecyclefacts

import (
	"go/types"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
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
	Condition ssaflow.CallCondition
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
func (fact *Fact) dischargesArgument(instruction ssa.Instruction, target ssa.Value, method string, observer ssaflow.Observer) bool {
	return dischargesMatch(fact.unconditionalDischarges(), instruction, target, method, observer)
}

// caseDischargesArgument is dischargesArgument for the fact's argument cases
// that the call's constant arguments select, with known fixing the caller's
// own parameters when the call sits in a body searched under constants.
func (fact *Fact) caseDischargesArgument(
	instruction ssa.Instruction, target ssa.Value, method string, known ssaflow.BooleanConstants, observer ssaflow.Observer,
) bool {
	return dischargesMatch(fact.casesSelectedBy(method, suppliedConstants(instruction, known)), instruction, target, method, observer)
}

func dischargesMatch(discharges []Discharge, instruction ssa.Instruction, target ssa.Value, method string, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, discharge := range discharges {
		if discharge.Method != method || discharge.Parameter >= len(common.Args) {
			continue
		}
		argument := common.Args[discharge.Parameter]
		storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer))
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
	structure := structBehind(owner)
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
