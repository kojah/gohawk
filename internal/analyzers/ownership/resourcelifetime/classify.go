package resourcelifetime

import (
	"go/token"
	"go/types"

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
	// candidate identifies the acquisition every step of this proof serves, and
	// probe tags every trace event with it so one acquisition's proof can be
	// read without the interleaved steps of the others in the same function.
	candidate token.Pos
	probe     analysisTrace.Probe
	owners    []ssa.Value
	contract  resourceContract
	optional  optionalAcquisitionProof
	actions   map[ssa.Instruction]resourceAction
}

func (analysis *resourceAnalysis) action(instruction ssa.Instruction) resourceAction {
	if action, ok := analysis.actions[instruction]; ok {
		return action
	}
	action, reason := analysis.classify(instruction)
	analysis.actions[instruction] = action
	analysis.emitAction(instruction, action, reason)
	return action
}

// classify labels the instruction and names why. The reason is the label for
// a settled or untouched resource, and for an opaque one it is the boundary
// that stopped the proof, so a reader can tell an interface call from a
// callee with no body without rereading this code.
func (analysis *resourceAnalysis) classify(instruction ssa.Instruction) (resourceAction, string) {
	if releasesResource(analysis.evidence, instruction, analysis.resource, analysis.owners, analysis.contract.cleanup, analysis.optional) ||
		analysis.contract.consumable && consumesResource(instruction, analysis.resource) {
		return actionSettled, actionSettled.String()
	}
	if boundary, opaque := analysis.opaqueConsumption(instruction); opaque {
		return actionUnknown, boundary
	}
	return actionNone, actionNone.String()
}

// opaqueConsumption reports whether the instruction hands the resource to
// something the analysis cannot see through.
func (analysis *resourceAnalysis) opaqueConsumption(instruction ssa.Instruction) (string, bool) {
	switch typed := instruction.(type) {
	case *ssa.Send:
		return "sent-to-channel", analysis.carries(typed.X)
	case *ssa.MapUpdate:
		return "stored-in-map", analysis.carries(typed.Value)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Send != nil && analysis.carries(state.Send) {
				return "sent-to-channel", true
			}
		}
		return "", false
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.opaqueCall(instruction, ssaflow.InstructionCall(instruction))
	}
	return "", false
}

func (analysis *resourceAnalysis) opaqueCall(instruction ssa.Instruction, common *ssa.CallCommon) (string, bool) {
	if common == nil {
		return "", false
	}
	carried := false
	for _, argument := range common.Args {
		carried = carried || analysis.carries(argument)
	}
	if common.IsInvoke() {
		// The receiver of an interface method is not consumed by being the
		// receiver; only the resource handed to the method is. The body behind
		// an interface method is chosen at run time, so no summary describes it.
		return "interface-method", carried
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return "appended", builtin.Name() == "append" && carried
	}
	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		if !carried && !analysis.closureCarries(closure) {
			return "", false
		}
		// A started literal runs on another goroutine, so a release inside it
		// cannot be ordered against this function's returns and the resource
		// is beyond what this flow can judge.
		if _, started := instruction.(*ssa.Go); started {
			return "captured-by-started-literal", true
		}
		// A called or deferred literal runs in this frame, so its body is as
		// readable as a named callee's. A release inside it was already proved
		// before this point, so what is left to ask is whether it keeps the
		// resource: if it does the obligation moved, and if it does not the
		// literal is transparent and this function still owns the resource.
		return "captured-by-retaining-literal", analysis.evidence.ClosureRetainsValue(closure, analysis.resource)
	}
	callee := common.StaticCallee()
	if callee == nil {
		// A function value: the callee is decided at run time.
		return "dynamic-callee", carried
	}
	if !carried {
		return "", false
	}
	// A resource that reaches the callee only inside an aggregate argument is
	// beyond a parameter-level completion proof: that proof follows the
	// parameter value, not a resource nested in one of its fields. Treat such a
	// call as a boundary only when its own result reaches a return, because
	// only then can the resource have been transferred to the value the caller
	// receives; a call whose result is discarded transfers nothing, so a
	// resource left behind in its argument is still leaked. oss-rebuild wraps a
	// zip reader in an fs.FS wrapper and returns the loader's result:
	// https://github.com/google/oss-rebuild/blob/9ce0528dd68bf209b52cc9fdc90bd63742cbb3a0/pkg/sysgraph/sgstorage/loader.go#L173-L179
	if analysis.carriedOnlyWithinAggregate(common) && callResultReachesReturn(instruction) {
		return "nested-in-transferred-argument", true
	}
	// A summarized callee proven to release, store, or own the resource was
	// classified as settled above; one summarized as doing none of those is
	// transparent. A callee with a body but no summary is judged by its body
	// through the completion proof already consulted; without either, the
	// callee is a boundary.
	return "unsummarized-callee", !analysis.evidence.CalleeSummarized(instruction) && len(callee.Blocks) == 0
}

// carriedOnlyWithinAggregate reports whether the resource reaches the call only
// as a field of an aggregate argument, never as an argument value in its own
// right. A resource passed directly stays visible to the callee's completion
// proof; one buried in a struct does not.
func (analysis *resourceAnalysis) carriedOnlyWithinAggregate(common *ssa.CallCommon) bool {
	within := false
	for _, argument := range common.Args {
		if analysis.carriesDirectly(argument) {
			return false
		}
		// A closure that captures the resource is not a struct aggregate; the
		// launch and closure analyses already decide its fate, so this rule
		// must not intercept it.
		if analysis.carriedWithinClosure(argument) {
			return false
		}
		within = within || analysis.carriesWithin(argument)
	}
	return within
}

// carriedWithinClosure reports whether the argument is a closure value that
// captures the resource.
func (analysis *resourceAnalysis) carriedWithinClosure(argument ssa.Value) bool {
	value := argument
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	if inner, ok := ssaflow.UnwrapTransparentValue(value, forms); ok {
		value = inner
	}
	closure, ok := value.(*ssa.MakeClosure)
	return ok && analysis.closureCarries(closure)
}

// callResultReachesReturn reports whether a value the call produces, other than
// an error, flows to a return of the enclosing function. Only such a result can
// hold the resource nested in one of the call's arguments, so a call whose
// error alone is propagated has transferred nothing.
func callResultReachesReturn(instruction ssa.Instruction) bool {
	result, ok := instruction.(ssa.Value)
	if !ok || instruction.Parent() == nil {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](instruction.Parent()) {
		for _, value := range returned.Results {
			if types.Identical(value.Type(), errorType) {
				continue
			}
			if ssaflow.ValueDerivesFrom(value, result, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

// carries reports whether value is the resource, derives from it, or is an
// aggregate holding it or a projection of it. A struct literal wrapping a
// type-asserted response body and handed to a function value is such an
// aggregate; kandev upgrades an SPDY response this way:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/kubernetes/portforward.go#L464-L491
func (analysis *resourceAnalysis) carries(value ssa.Value) bool {
	return analysis.carriesDirectly(value) || analysis.carriesWithin(value)
}

// carriesDirectly reports whether value is the resource itself or is produced
// from it by a transparent value step, so a callee receives the resource as an
// argument in its own right.
func (analysis *resourceAnalysis) carriesDirectly(value ssa.Value) bool {
	return ssaflow.SameValue(value, analysis.resource) ||
		ssaflow.ValueDerivesFrom(value, analysis.resource, map[ssa.Value]bool{})
}

// carriesWithin reports whether value is an aggregate that holds the resource
// in one of its fields, so a callee receives the resource only nested inside a
// parameter.
func (analysis *resourceAnalysis) carriesWithin(value ssa.Value) bool {
	if ssaflow.ValueContainsValue(value, analysis.resource) {
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

func (analysis *resourceAnalysis) emitAction(instruction ssa.Instruction, action resourceAction, reason string) {
	if action == actionNone || !analysis.probe.Enabled() {
		return
	}
	analysis.probe.Evidence(analysisTrace.Step{
		Reason:   reason,
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      instruction.Pos(),
		Function: analysis.function.String(),
		Details:  map[string]string{"instruction": instruction.String()},
	})
}
