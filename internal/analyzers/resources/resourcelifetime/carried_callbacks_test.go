package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/summaries"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestCarriedClosureAllowance(t *testing.T) {
	pkg, provider := carriedCallbackFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{{"capture", true}, {"nested", true}, {"unrelated", false}, {"boxed", true}, {"twice", false}, {"empty", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := callbackAnalysis(fn, provider)
			value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			checkCallbackAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return query.proveCarriedClosureWithin(value, budget)
			}, test.want)
			if test.name == "twice" && !query.proveCarriedValueWithin(value, nil).Proven() {
				t.Fatalf("two-wrapper control lacks broader carrying; SSA:\n%s", carriedSSA(t, fn))
			}
		})
	}
}

func TestClosureBindingAllowance(t *testing.T) {
	pkg, provider := carriedCallbackFixture(t)
	for _, name := range []string{"capture", "nested", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			closure := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)[0]
			query := callbackAnalysis(fn, provider)
			checkCallbackAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return query.proveClosureCarryWithin(closure, budget)
			}, name != "unrelated")
		})
	}
}

func TestCarriedCallArgumentsAllowance(t *testing.T) {
	pkg, provider := carriedCallbackFixture(t)
	for _, test := range []struct {
		family, name string
		want         bool
	}{
		{"retention", "registered", true},
		{"retention", "observed", false},
		{"retention", "borrowed", false},
		{"retention", "otherCallback", false},
		{"retention", "directCall", false},
		{"aggregate", "mixed", true},
		{"aggregate", "aggregate", true},
		{"aggregate", "callbackArg", false},
		{"aggregate", "otherAggregate", false},
		{"aggregate", "directCall", false},
		{"loop", "loopDirect", true},
		{"loop", "loopAggregate", true},
		{"loop", "loopOther", false},
	} {
		t.Run(test.family+"/"+test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkCallbackAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				switch test.family {
				case "retention":
					return query.provePossiblyRetainedCallbackWithin(call, call.Common(), budget)
				case "aggregate":
					return query.proveCarriedAggregateArgumentsWithin(call.Common(), budget)
				default:
					return query.proveImportedLoopReleaseWithin(call, call.Common(), budget)
				}
			}, test.want)
		})
	}
}

func TestClosureClassifierCutoff(t *testing.T) {
	pkg, provider := carriedCallbackFixture(t)
	fn := pkg.Func("launch")
	launched := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
	closure := launched.Common().Value.(*ssa.MakeClosure)
	query := callbackAnalysis(fn, provider)
	query.pool = ssaflow.NewSearchBudget(0)
	if reason, opaque := query.opaqueClosureCall(launched, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted closure classifier = %v/%v", reason, opaque)
	}
	query.pool = ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	if reason, opaque := query.opaqueClosureCall(launched, closure, false); !opaque || reason != resourceReasonCapturedByStartedLiteral {
		t.Fatalf("fresh closure classifier = %v/%v", reason, opaque)
	}
}

func checkCallbackAllowance(t *testing.T, prove func(*ssaflow.SearchBudget) resourceProof, want bool) {
	t.Helper()
	if got := prove(nil); got.State == ssaflow.EvidenceUnknown || got.Proven() != want {
		t.Fatalf("default callback evidence = %+v, want %v", got, want)
	}
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		got := prove(budget)
		if resourceFlowExhausted(budget) || limit == 0 {
			if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted callback evidence: %+v", limit, got)
			}
			continue
		}
		if got.State == ssaflow.EvidenceUnknown || got.Proven() != want {
			t.Fatalf("complete callback evidence = %+v, want %v", got, want)
		}
		return
	}
	t.Fatal("callback query never completed")
}

func callbackAnalysis(fn *ssa.Function, provider *summaries.Provider) *resourceAnalysis {
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence}
}

func carriedCallbackFixture(t *testing.T) (*ssa.Package, *summaries.Provider) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "carriedcallback", `package carriedcallback
type resource struct { n int }
type holder struct { value *resource }
type callback func()
func capture(p, other *resource) func() { return func(){ println(p) } }
func nested(p, other *resource) func() { h := &holder{p}; return func(){ println(h) } }
func unrelated(p, other *resource) func() { return func(){ println(other) } }
func boxed(p, other *resource) any { return func(){ println(p) } }
func twice(p, other *resource) any { return callback(func(){ println(p) }) }
func empty(p, other *resource) func() { return func(){} }
func register(func())
func observe(func()) {}
func borrow(func())
func use(any, any)
func loop(any)
func registered(p, other *resource) { register(func(){ println(p) }) }
func observed(p, other *resource) { observe(func(){ println(p) }) }
func borrowed(p, other *resource) { borrow(func(){ println(p) }) }
func otherCallback(p, other *resource) { register(func(){ println(other) }) }
func directCall(p, other *resource) { use(p, nil) }
func mixed(p, other *resource) { use(p, &holder{p}) }
func aggregate(p, other *resource) { use(&holder{p}, nil) }
func callbackArg(p, other *resource) { use(p, func(){ println(p) }) }
func otherAggregate(p, other *resource) { use(p, &holder{other}) }
func loopDirect(p, other *resource) { loop(p) }
func loopAggregate(p, other *resource) { loop(&holder{p}) }
func loopOther(p, other *resource) { loop(other) }
func launch(p, other *resource) { go func(){ println(p) }() }
`)
	borrow := lifecyclefacts.Fact{}
	borrow.DescribeFact(pkg.Func("borrow").Object())
	loop := lifecyclefacts.Fact{May: lifecyclefacts.MayClaims{LoopReleased: 1}}
	loop.DescribeFact(pkg.Func("loop").Object())
	retained := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot:   heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}},
			Escape: heapmodel.HeapEscapedGlobal,
		}},
	}}
	retained.DescribeFact(pkg.Func("register").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("borrow"): borrow, pkg.Func("loop"): loop, pkg.Func("register"): retained},
	}}
	return pkg, resourceSummaries.Provider(pass)
}
