package concurrencyfacts

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func TestImportedEffects(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testeffects", Doc: "assert imported ordered effects", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				switch function.Name() {
				case "forward", "reverse":
					result := engine.Function(function, proofs.NewSearchBudget(2000))
					if result.Completeness() != CompleteWithEffects || len(result.Operations) != 4 {
						t.Fatalf("%s: %+v", function, result)
					}
					indices := []int{0, 1, 1, 0}
					if function.Name() == "reverse" {
						indices = []int{1, 0, 0, 1}
					}
					for i, kind := range []Kind{Lock, Lock, Unlock, Unlock} {
						if op := result.Operations[i]; op.Kind != kind || op.Resource.Value != function.Params[indices[i]] {
							t.Errorf("%s event %d: %+v", function, i, op)
						}
					}
					checked++
				case "spawning":
					assertImportedWorker(t, function, engine.Function(function, proofs.NewSearchBudget(2000)))
					checked++
				case "opaque", "conditional", "localOnly":
					result := engine.Function(function, proofs.NewSearchBudget(2000))
					if result.Completeness() != Incomplete || result.Complete() || result.Reason == ReasonNone {
						t.Errorf("%s unexpectedly complete: %+v", function, result)
					}
					checked++
				case "empty":
					result := engine.Function(function, proofs.NewSearchBudget(2000))
					if result.Completeness() != CompleteNoEffects || !result.Complete() || len(result.Operations) != 0 {
						t.Errorf("complete empty effects lost: %+v", result)
					}
					checked++
				}
			}
			if checked != 7 {
				t.Errorf("checked %d functions, want 7", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "consumer")
}

func assertImportedWorker(t *testing.T, function *ssa.Function, result Summary) {
	t.Helper()
	if result.Completeness() != CompleteWithEffects || len(result.Workers) != 1 ||
		len(result.Workers[0].Operations) != 2 {
		t.Errorf("imported child launch = %+v", result)
		return
	}
	for i, kind := range []Kind{Lock, Unlock} {
		if op := result.Workers[0].Operations[i]; op.Kind != kind || op.Resource.Value != function.Params[0] {
			t.Errorf("imported child operation %d = %+v", i, op)
		}
	}
}

var alternativeChecks = map[string]func(*testing.T, *ssa.Function, Summary){
	"pickTwice": func(t *testing.T, function *ssa.Function, got Summary) {
		t.Helper()
		if len(got.Paths) != 4 {
			t.Fatalf("pickTwice = %+v, want four paths", got)
		}
		for _, path := range got.Paths {
			if len(path.Conditions) != 2 || path.Conditions[0].Value != function.Params[2] ||
				path.Conditions[1].Value != function.Params[2] {
				t.Errorf("pickTwice conditions = %+v, want both on the caller's x", path.Conditions)
			}
		}
	},
	"modeConstant": func(t *testing.T, _ *ssa.Function, got Summary) {
		t.Helper()
		if len(got.Paths) != 2 {
			t.Fatalf("modeConstant = %+v, want two paths", got)
		}
		for _, path := range got.Paths {
			if len(path.Conditions) != 1 || path.Conditions[0].Compared == nil {
				t.Errorf("modeConstant conditions = %+v, want one comparison", path.Conditions)
			}
		}
	},
	"useAcquire": func(t *testing.T, _ *ssa.Function, got Summary) {
		t.Helper()
		implied := 0
		for _, path := range got.Paths {
			for _, condition := range path.Conditions {
				if condition.Implied {
					implied++
				}
			}
		}
		if len(got.Paths) == 0 || implied == 0 {
			t.Errorf("useAcquire = %+v, want paths with implied result conditions", got)
		}
	},
}

// Published alternatives bind at the importing call: conditions on a formal
// become conditions on the caller's argument, a constant argument stays
// comparable with the published constant, and a return fact becomes an
// implied condition on the caller's result.
func TestImportedAlternatives(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testalternatives", Doc: "assert imported path alternatives", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			checked := 0
			for _, function := range functions {
				if check, ok := alternativeChecks[function.Name()]; ok {
					checked++
					if function.Name() == "pickTwice" {
						assertImportedAlternativeCopy(t, engine, function)
					}
					check(t, function, engine.Function(function, proofs.NewSearchBudget(4000)))
				}
			}
			if checked != 3 {
				t.Errorf("checked %d functions, want 3", checked)
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "branchuse")
}

// Mutating a caller-owned bound path must not rewrite the imported fact, even
// before a summary of the caller has been cached.
func assertImportedAlternativeCopy(t *testing.T, engine *Engine, caller *ssa.Function) {
	t.Helper()
	call := ssaflow.InstructionsOf[*ssa.Call](caller)[0]
	bound := engine.AtCall(call, proofs.NewSearchBudget(proofs.SummaryBudget))
	if len(bound.Paths) != 2 || len(bound.Paths[0].Operations) != 1 || len(bound.Paths[0].Conditions) != 1 {
		t.Errorf("imported Pick binding = %+v", bound)
		return
	}
	operation := bound.Paths[0].Operations[0]
	condition := bound.Paths[0].Conditions[0]
	bound.Paths[0].Operations[0].Resource.Value = nil
	bound.Paths[0].Conditions[0].Holds = !condition.Holds
	bound.Paths[0].Operations = nil
	fresh := engine.AtCall(call, proofs.NewSearchBudget(proofs.SummaryBudget))
	if len(fresh.Paths) != 2 || len(fresh.Paths[0].Operations) != 1 || len(fresh.Paths[0].Conditions) != 1 ||
		fresh.Paths[0].Operations[0].Resource != operation.Resource || fresh.Paths[0].Conditions[0].Holds != condition.Holds {
		t.Errorf("bound mutation changed imported evidence: %+v", fresh)
	}
}

func TestImportedCancellationRequirements(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testcancellation", Doc: "assert conditional cancellation binding", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			for _, function := range functions {
				result := engine.Root(function, proofs.NewSearchBudget(2000))
				if function.Name() != "bound" {
					if result.Complete() || result.Reason == ReasonNone {
						t.Errorf("%s lost imported requirement: %+v", function, result)
					}
					continue
				}
				if !result.Complete() || len(result.Workers) != 1 || len(result.Workers[0].Operations) != 1 || len(result.Operations) != 1 {
					t.Fatalf("imported bound = %+v", result)
				}
				cancel, receive := result.Operations[0], result.Workers[0].Operations[0]
				if cancel.Kind != Cancel || receive.Kind != Receive || cancel.Resource != receive.Resource {
					t.Errorf("imported identity differs: cancel=%+v receive=%+v", cancel, receive)
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "cancelconsumer")
}

func TestDescribeFactRendersAlternatives(t *testing.T) {
	fact := Fact{Alternatives: []FactAlternative{
		{
			Effects:    []Effect{{Kind: Close, Parameter: 0}},
			Conditions: []FactCondition{{Parameter: 2, Constant: FactConstant{Kind: FactConstantInt, Exact: "1"}, Holds: true}},
		},
		{
			Effects:    []Effect{{Kind: Lock, Parameter: 1, Fields: []int{0}}},
			Conditions: []FactCondition{{Parameter: -1, Internal: 0}},
			Returned:   []FactReturned{{Index: 0, Constant: FactConstant{Kind: FactConstantNil}}},
		},
	}}
	want := []string{
		"path 1 when parameter 2 == 1:",
		"  close parameter 0",
		"path 2 when its own test 0 does not hold, returning result 0 nil:",
		"  Lock parameter 1 field 0",
	}
	if got := fact.describe(); !slices.Equal(got, want) {
		t.Errorf("describe() = %q, want %q", got, want)
	}
	if got := (Fact{}).describe(); !slices.Equal(got, []string{"no synchronization effects"}) {
		t.Errorf("empty describe() = %q", got)
	}
}

func TestReasonCodes(t *testing.T) {
	want := map[Reason]string{
		ReasonNone:                      "",
		ReasonComponentNotRequested:     "concurrency-component-not-requested",
		ReasonComponentUnavailable:      "concurrency-component-unavailable",
		ReasonAlternativeLimit:          "protocol-alternative-limit",
		ReasonBodyUnavailable:           "protocol-body-unavailable",
		ReasonBranchAlternatives:        "protocol-branch-alternatives",
		ReasonBranchEffectsDiffer:       "protocol-branch-effects-differ",
		ReasonBudgetExhausted:           "protocol-budget-exhausted",
		ReasonChannelBindingUnknown:     "protocol-channel-binding-unknown",
		ReasonChannelIdentityUnknown:    "protocol-channel-identity-unknown",
		ReasonCondLockerUnknown:         "protocol-cond-locker-unknown",
		ReasonContextBindingRequired:    "protocol-context-binding-required",
		ReasonContextBindingUnknown:     "protocol-context-binding-unknown",
		ReasonContextIdentityUnknown:    "protocol-context-identity-unknown",
		ReasonContextParentUnknown:      "protocol-context-parent-unknown",
		ReasonControlFlowUnknown:        "protocol-control-flow-unknown",
		ReasonCutoff:                    "protocol-cutoff",
		ReasonDeferredEffectsUnknown:    "protocol-deferred-effects-unknown",
		ReasonEffectUnknown:             "protocol-effect-unknown",
		ReasonFieldBindingUnknown:       "protocol-field-binding-unknown",
		ReasonGroupCountUnknown:         "protocol-group-count-unknown",
		ReasonLoadUnknown:               "protocol-load-unknown",
		ReasonLocalContextUnknown:       "protocol-local-context-unknown",
		ReasonParticipantsUnknown:       "protocol-participants-unknown",
		ReasonPayloadUnknown:            "protocol-payload-unknown",
		ReasonSelectAlternatives:        "protocol-select-alternatives",
		ReasonSelectAlternativesUnknown: "protocol-select-alternatives-unknown",
		ReasonSelectDispatchUnknown:     "protocol-select-dispatch-unknown",
		ReasonSelectNoFeasibleArm:       "protocol-select-no-feasible-arm",
		ReasonSummaryLimit:              "protocol-summary-limit",
		ReasonWorkerEffectsUnknown:      "protocol-worker-effects-unknown",
		ReasonSummarizing:               "summarizing-concurrency",
		ReasonExportUnknown:             "concurrency-export-unknown",
		ReasonExportComplete:            "concurrency-export-complete",
		ReasonRecursiveProtocol:         "recursive-protocol",
		ReasonCallbackBindingRequired:   "protocol-callback-binding-required",
		ReasonCallbackUnknown:           "protocol-callback-unknown",
		ReasonReplicatedWorkers:         "protocol-replicated-workers",
	}
	if len(want) != int(reasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range reasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []Reason{reasonCount, 255} {
		if reason.String() != "invalid-concurrency-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}
