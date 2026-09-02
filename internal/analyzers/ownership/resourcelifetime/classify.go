package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The classifier labels each instruction after an acquisition once, and the
// flow asks one question of the labels: does a release or transfer cover
// every feasible return, and if not, did anything the analysis cannot see
// through consume the resource on that path? A diagnostic needs the second
// answer to be no. Summaries are the model's knowledge: a summarized callee
// that neither releases, stores, nor owns the resource is transparent, so a
// read helper leaves the obligation in place. Unknown is reserved for real
// boundaries: an interface method or function value that receives the
// resource here, a callee with neither body nor summary, a literal capturing
// the resource that is launched or deferred without a proven release, and a
// send or append that hands it to storage the analysis does not track.

type resourceAction uint8

const (
	actionNone resourceAction = iota
	// actionSettled covers both a proven release and a proven transfer; the
	// flow treats them alike and the evidence trace records which one held.
	actionSettled
	actionUnknown
)

func (action resourceAction) String() string {
	switch action {
	case actionSettled:
		return "settled"
	case actionUnknown:
		return "opaque-use"
	case actionNone:
	}
	return "none"
}

// resourceAnalysis holds one acquisition's inputs and its memoized labels.
type resourceAnalysis struct {
	pass     *analysis.Pass
	evidence *lifecyclefacts.LifecycleEvidence
	function *ssa.Function
	resource ssa.Value
	owners   []ssa.Value
	contract resourceContract
	optional optionalAcquisitionProof
	actions  map[ssa.Instruction]resourceAction
}

func (analysis *resourceAnalysis) action(instruction ssa.Instruction) resourceAction {
	if action, ok := analysis.actions[instruction]; ok {
		return action
	}
	action := analysis.classify(instruction)
	analysis.actions[instruction] = action
	analysis.emitAction(instruction, action)
	return action
}

func (analysis *resourceAnalysis) classify(instruction ssa.Instruction) resourceAction {
	if releasesResource(analysis.evidence, instruction, analysis.resource, analysis.owners, analysis.contract.cleanup, analysis.optional) ||
		analysis.contract.consumable && consumesResource(instruction, analysis.resource) {
		return actionSettled
	}
	if analysis.opaqueConsumption(instruction) {
		return actionUnknown
	}
	return actionNone
}

// opaqueConsumption reports whether the instruction hands the resource to
// something the analysis cannot see through.
func (analysis *resourceAnalysis) opaqueConsumption(instruction ssa.Instruction) bool {
	switch typed := instruction.(type) {
	case *ssa.Send:
		return analysis.carries(typed.X)
	case *ssa.MapUpdate:
		return analysis.carries(typed.Value)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Send != nil && analysis.carries(state.Send) {
				return true
			}
		}
		return false
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.opaqueCall(instruction, ssaflow.InstructionCall(instruction))
	}
	return false
}

func (analysis *resourceAnalysis) opaqueCall(instruction ssa.Instruction, common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}
	carried := false
	for _, argument := range common.Args {
		carried = carried || analysis.carries(argument)
	}
	if common.IsInvoke() {
		// The receiver of an interface method is not consumed by being the
		// receiver; only the resource handed to the method is.
		return carried
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name() == "append" && carried
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		// A literal that captures the resource and was not proven to release
		// it may release it later or on another path.
		return carried || analysis.closureCarries(closure)
	}
	callee := common.StaticCallee()
	if callee == nil {
		// A function value: the callee is decided at run time.
		return carried
	}
	if !carried {
		return false
	}
	// A summarized callee proven to release, store, or own the resource was
	// classified as settled above; one summarized as doing none of those is
	// transparent. A callee with a body but no summary is judged by its body
	// through the completion proof already consulted; without either, the
	// callee is a boundary.
	return !analysis.evidence.CalleeSummarized(instruction) && len(callee.Blocks) == 0
}

// carries reports whether value is the resource, derives from it, or is an
// aggregate holding it or a projection of it. A struct literal wrapping a
// type-asserted response body and handed to a function value is such an
// aggregate; kandev upgrades an SPDY response this way:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/kubernetes/portforward.go#L464-L491
func (analysis *resourceAnalysis) carries(value ssa.Value) bool {
	if ssaflow.SameValue(value, analysis.resource) ||
		ssaflow.ValueDerivesFrom(value, analysis.resource, map[ssa.Value]bool{}) ||
		ssaflow.ValueContainsValue(value, analysis.resource) {
		return true
	}
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	return ssaflow.NewReachingWalk(forms).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		if _, ok := value.(*ssa.Alloc); !ok {
			return false
		}
		for stored := range ssaflow.StoredInto(value) {
			if ssaflow.ValueDerivesFrom(stored, analysis.resource, map[ssa.Value]bool{}) {
				return true
			}
		}
		return false
	})
}

func (analysis *resourceAnalysis) closureCarries(closure *ssa.MakeClosure) bool {
	for _, binding := range closure.Bindings {
		if ssaflow.CapturedBindingMatches(binding, analysis.resource) || analysis.carries(binding) {
			return true
		}
	}
	return false
}

func (analysis *resourceAnalysis) emitAction(instruction ssa.Instruction, action resourceAction) {
	if action == actionNone || !analysisTrace.Enabled("resourcelifetime", string(check.ResourceRelease)) {
		return
	}
	analysisTrace.Emit(analysis.pass, analysisTrace.Event{
		Analyzer: "resourcelifetime",
		Check:    string(check.ResourceRelease),
		Phase:    "evidence",
		Reason:   action.String(),
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      instruction.Pos(),
		Function: analysis.function.String(),
		Details:  map[string]string{"instruction": instruction.String()},
	})
}
