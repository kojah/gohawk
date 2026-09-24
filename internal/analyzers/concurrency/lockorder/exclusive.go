package lockorder

import (
	"strconv"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// An acquisition a callee makes is deliberately not discharged from the
// call: the callee's acquisitions are summarized per lock class, so one
// record can stand for several parameters, and a fresh argument in one
// position must not excuse an acquisition fed by a shared argument in
// another.
//
// An acquisition on an object no other goroutine can reach orders nothing:
// the lock cannot be contended, so taking it while another lock is held
// says nothing about the order the program takes the two in elsewhere. The
// common shape is initialization, a fresh object locked while a registry
// lock is held and published only afterwards, whose reverse of the steady
// state order is not a deadlock. The proof is a precondition in two halves.
// In the function that locks, the points-to graph shows the object has not
// escaped at the acquisition; it is either a local allocation that is
// published later, or a parameter. For a parameter, every caller in the
// package must hand in a fresh local that has not escaped at the call, and
// the function must be unexported, so that no caller is unseen. A local
// that never escapes at all is deliberately not covered: the existing
// fixtures report an inversion between two purely local locks, and this
// change does not revisit that policy.
// https://github.com/ozontech/file.d, gauge, and gopcua/opcua are the
// dogfood shapes: a job, gauge, or channel registered under a global lock
// after its own lock is taken.

// exclusiveCallers indexes, per unexported function, the call sites in the
// package, so the caller half of the proof can be asked once per function.
type exclusiveCallers struct {
	pass  *analysis.Pass
	sites map[*ssa.Function][]*ssa.Call
	// exclusive caches the answer per function and parameter.
	exclusive map[exclusiveKey]bool
}

type exclusiveKey struct {
	function *ssa.Function
	index    int
}

func newExclusiveCallers(pass *analysis.Pass, functions []*ssa.Function) *exclusiveCallers {
	callers := &exclusiveCallers{pass: pass, sites: map[*ssa.Function][]*ssa.Call{}, exclusive: map[exclusiveKey]bool{}}
	for _, function := range functions {
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
			if callee := call.Common().StaticCallee(); callee != nil && !call.Common().IsInvoke() {
				callers.sites[callee] = append(callers.sites[callee], call)
			}
		}
	}
	return callers
}

// parameterExclusive reports whether every call of the function in the
// package passes, at the parameter's position, a fresh local object that has
// not escaped at the call. An exported function, a function no call site
// reaches, or a call through a closure or interface is never proven.
func (callers *exclusiveCallers) parameterExclusive(function *ssa.Function, index int) bool {
	key := exclusiveKey{function: function, index: index}
	if answer, ok := callers.exclusive[key]; ok {
		return answer
	}
	callers.exclusive[key] = false
	object := function.Object()
	sites := callers.sites[function]
	if object == nil || object.Exported() || len(sites) == 0 {
		return false
	}
	for _, call := range sites {
		if index >= len(call.Common().Args) {
			return false
		}
		exclusive, ok := heapmodel.ExclusiveAt(call.Common().Args[index], call)
		if !ok || !exclusive.Local {
			return false
		}
	}
	callers.exclusive[key] = true
	return true
}

// acquisitionExclusive decides whether an acquisition orders nothing
// because its object is exclusively owned at the instruction, and traces
// the half of the proof that decided it.
func (callers *exclusiveCallers) acquisitionExclusive(function *ssa.Function, instruction ssa.Instruction, receiver ssa.Value) bool {
	if receiver == nil {
		return false
	}
	exclusive, ok := heapmodel.ExclusiveAt(receiver, instruction)
	if !ok {
		return false
	}
	probe := analysisTrace.For(callers.pass, "lockorder", string(check.LockContradictoryOrder), instruction.Pos())
	switch {
	case exclusive.Local && exclusive.Published:
		probe.Decision(analysisTrace.Step{Reason: "exclusive-object-before-publication", Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos()})
		return true
	case exclusive.Local:
		return false
	case callers.parameterExclusive(function, exclusive.Parameter):
		probe.Decision(analysisTrace.Step{
			Reason: "exclusive-parameter-from-fresh-callers", Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos(),
			Details: map[string]string{"parameter": strconv.Itoa(exclusive.Parameter), "callers": strconv.Itoa(len(callers.sites[function]))},
		})
		return true
	}
	return false
}
