// Package nilargument implements the nilargument gohawk analyzer.
package nilargument

import (
	"fmt"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The check is a precondition applied at a call site: the callee's summary
// says it dereferences the object at a slot beneath one of its parameters
// on every normal return, and the caller's points-to graph says that slot
// certainly holds nil where the call is made. Both halves are must-facts.
// A callee that dereferences on some path only requires nothing, and a
// caller whose slot is nil on one branch only knows nothing. Only
// pointer-typed slots are judged: an interface slot the caller filled from
// a nil pointer is not a nil interface, and the graph does not keep the
// two apart, so an interface slot is never a witness here.

var summaryKnowledge = summaries.Select(summaries.Requirements{Lifecycle: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "nilargument",
		Doc:      "checks calls that pass a nil value where the callee dereferences it on every path",
		Requires: summaryKnowledge.Requires(),
		Run:      runNilArgument,
	}
}

func runNilArgument(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	knowledge := summaryKnowledge.Provider(pass)
	evidence, _ := knowledge.LifecycleEvidence("nilargument", string(check.NilArgumentDereference))
	for _, function := range functions {
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
			callee := call.Common().StaticCallee()
			if callee == nil || call.Common().IsInvoke() {
				continue
			}
			evidence.ForCandidate(call.Pos())
			for index, argument := range call.Common().Args {
				for _, path := range evidence.ArgumentPathsRequiredNonNil(call, index) {
					judgeArgument(pass, function, call, callee, index, argument, path)
				}
			}
		}
	}
	return nil, nil
}

// judgeArgument reports the call when the slot the callee dereferences is
// certainly nil in the caller, and traces why it did not otherwise.
func judgeArgument(pass *analysis.Pass, function *ssa.Function, call *ssa.Call, callee *ssa.Function, index int, argument ssa.Value, path string) {
	probe := analysisTrace.For(pass, "nilargument", string(check.NilArgumentDereference), call.Pos())
	details := map[string]string{"callee": callee.String(), "argument": strconv.Itoa(index), "path": path}
	probe.Candidate(analysisTrace.Step{Reason: "callee-dereferences-argument", Outcome: analysisTrace.OutcomeObserved, Pos: call.Pos(), Details: details})
	slotType, ok := typeAtPath(argument.Type(), path)
	if !ok {
		probe.Decision(analysisTrace.Step{Reason: "slot-type-unknown", Outcome: analysisTrace.OutcomeUnknown, Pos: call.Pos(), Details: details})
		return
	}
	if _, pointer := slotType.Underlying().(*types.Pointer); !pointer {
		probe.Decision(analysisTrace.Step{Reason: "slot-not-pointer", Outcome: analysisTrace.OutcomeUnknown, Pos: call.Pos(), Details: details})
		return
	}
	if !ssainfer.ContentIsNilAt(argument, ssaflow.SplitAccessPath(path), call) {
		probe.Decision(analysisTrace.Step{Reason: "slot-not-proven-nil", Outcome: analysisTrace.OutcomeAccepted, Pos: call.Pos(), Details: details})
		return
	}
	if probe.Enabled() {
		traceCallApplications(probe, function, call)
	}
	probe.Decision(analysisTrace.Step{
		Reason: "nil-slot-dereferenced", Outcome: analysisTrace.OutcomeRejected, Pos: call.Pos(), Function: function.String(), Details: details,
	})
	what := "argument " + strconv.Itoa(index+1)
	if path != "" {
		what = describePath(argument.Type(), path) + " of " + what
	}
	source := syntax.SourceRange(pass, call.Pos())
	check.Report(pass, check.NilArgumentDereference, analysis.Diagnostic{
		Pos: source.Pos(), End: source.End(),
		Message: fmt.Sprintf("%s is nil, and %s dereferences it on every path", what, callee.RelString(pass.Pkg)),
	})
}

// tracedUnsummarizedLimit bounds the unsummarized calls one decision lists.
const tracedUnsummarizedLimit = 8

// traceCallApplications explains a nil proof by the calls before it: the
// slot stays nil only if no earlier call could have written it, so a call
// whose summary the graph did not apply is where a missed write would
// hide. It lists those calls that can reach the judged call, bounded, and
// counts the rest. It reads the graph's records and decides nothing.
func traceCallApplications(probe analysisTrace.Probe, function *ssa.Function, call *ssa.Call) {
	summarized, unsummarized := 0, 0
	for _, record := range heapmodel.CallApplications(function) {
		if !reachesCall(record.Instruction, call) {
			continue
		}
		if record.Reason == heapmodel.CallSummaryApplied {
			summarized++
			continue
		}
		unsummarized++
		if unsummarized > tracedUnsummarizedLimit {
			continue
		}
		details := map[string]string{"call": record.Instruction.String(), "reason": string(record.Reason)}
		if record.Callee != nil {
			details["callee"] = record.Callee.String()
			details["registered-now"] = strconv.FormatBool(record.RegisteredNow)
		}
		probe.Evidence(analysisTrace.Step{
			Reason: "earlier-call-unsummarized", Outcome: analysisTrace.OutcomeObserved, Pos: record.Instruction.Pos(), Details: details,
		})
	}
	probe.Evidence(analysisTrace.Step{
		Reason: "earlier-calls", Outcome: analysisTrace.OutcomeObserved, Pos: call.Pos(),
		Details: map[string]string{"summarized": strconv.Itoa(summarized), "unsummarized": strconv.Itoa(unsummarized)},
	})
}

// reachesCall reports whether control can pass from an earlier call to the
// judged one.
func reachesCall(earlier ssa.Instruction, call *ssa.Call) bool {
	return earlier != call && slices.Contains(ssaflow.InstructionsReachableAfter(earlier), ssa.Instruction(call))
}

// typeAtPath follows an access path through a type: a pointer to a struct
// or a struct at a field step, an array or slice at an index step.
func typeAtPath(root types.Type, path string) (types.Type, bool) {
	current := root
	for _, step := range ssaflow.SplitAccessPath(path) {
		if pointer, ok := current.Underlying().(*types.Pointer); ok {
			current = pointer.Elem()
		}
		switch {
		case strings.HasPrefix(step, "field:"):
			structure, ok := current.Underlying().(*types.Struct)
			field, err := strconv.Atoi(strings.TrimPrefix(step, "field:"))
			if !ok || err != nil || field < 0 || field >= structure.NumFields() {
				return nil, false
			}
			current = structure.Field(field).Type()
		case strings.HasPrefix(step, "index:"):
			switch container := current.Underlying().(type) {
			case *types.Array:
				current = container.Elem()
			case *types.Slice:
				current = container.Elem()
			default:
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return current, true
}

// describePath names an access path for a reader: the field names it
// selects, joined by dots, with an element step spelled out.
func describePath(root types.Type, path string) string {
	current := root
	var names []string
	for _, step := range ssaflow.SplitAccessPath(path) {
		if pointer, ok := current.Underlying().(*types.Pointer); ok {
			current = pointer.Elem()
		}
		if structure, ok := current.Underlying().(*types.Struct); ok && strings.HasPrefix(step, "field:") {
			if field, err := strconv.Atoi(strings.TrimPrefix(step, "field:")); err == nil && field >= 0 && field < structure.NumFields() {
				names = append(names, structure.Field(field).Name())
				current = structure.Field(field).Type()
				continue
			}
		}
		names = append(names, step)
	}
	return "field " + strings.Join(names, ".")
}
