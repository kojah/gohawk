package resourcelifetime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysis/summaries"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestCarriedValueAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "carried", `package carried
type resource struct { n int }
type holder struct { value *resource }
func direct(p, other *resource) any { return p }
func loaded(p, other *resource) any { h := &holder{p}; return h.value }
func nested(p, other *resource) any { return &holder{p} }
func unrelated(p, other *resource) any { return &holder{other} }
func derived(p, other *resource) any { return &struct{ value *int }{&p.n} }
func closure(p, other *resource) any { return func() { println(p) } }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"loaded", true}, {"nested", true}, {"unrelated", false}, {"derived", true}, {"closure", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			baseline := aggregateEscapeAnalysis(fn).proveCarriedValueWithin(value, nil).Proven()
			if baseline != test.want {
				t.Fatalf("default carry = %v, want %v; SSA:\n%s", baseline, test.want, carriedSSA(t, fn))
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				proof := aggregateEscapeAnalysis(fn).proveCarriedValueWithin(value, budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted carry: %+v", limit, proof)
					}
					continue
				}
				if proof.State == proofs.EvidenceUnknown || proof.Proven() != baseline {
					t.Fatalf("complete carry = %+v, want %v", proof, baseline)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("carry never completed")
			}
		})
	}
}

func TestCarriedStorageChildCutoff(t *testing.T) {
	var source strings.Builder
	source.WriteString("package storagecap\ntype resource struct{}\ntype a *resource\ntype b *resource\nfunc long(p, other a) a {\n")
	previous := "other"
	for index := range 1010 {
		typeName := "b"
		if index%2 == 1 {
			typeName = "a"
		}
		name := fmt.Sprintf("x%d", index)
		fmt.Fprintf(&source, "%s := %s(%s)\n", name, typeName, previous)
		previous = name
	}
	fmt.Fprintf(&source, "return %s\n}", previous)
	fn := ssaflowtest.BuildPackage(t, "storagecap", source.String()).Func("long")
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count != 1010 {
		t.Fatalf("conversion count = %d", count)
	}
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
	proof := aggregateEscapeAnalysis(fn).proveCarriedDirectlyWithin(value, budget)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
		t.Fatalf("storage child cutoff with available caller = %+v, caller exhausted %v", proof, resourceFlowExhausted(budget))
	}
}

func TestNestedCarryIncludesDerivedStoredValues(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "derivedcarry", `package derivedcarry
type resource struct { n int }
func derived(p *resource) any { return &struct{ value *int }{&p.n} }
`)
	fn := pkg.Func("derived")
	value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
	if contained := lifecycle.ProveMayContainValueWithin(value, fn.Params[0], nil); contained.Proven() {
		t.Fatalf("control already has object containment: %+v", contained)
	}
	proof := aggregateEscapeAnalysis(fn).proveNestedCarryWithin(value, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !proof.Proven() || proof.Reason != resourceReasonAggregateMayCarry {
		t.Fatalf("derived stored value = %+v; SSA:\n%s", proof, carriedSSA(t, fn))
	}
}

func TestCarriedPayloadClassifierCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "payload", `package payload
type resource struct{}
type holder struct { value *resource }
func send(p, other *resource, ch chan any) { ch <- &holder{p} }
func unrelated(p, other *resource, ch chan any) { ch <- &holder{other} }
func put(p, other *resource, m map[int]any) { m[0] = p }
func choose(p, other *resource, ch chan any) { select { case ch <- p: default: } }
func dynamic(p, other *resource, f func(any)) { f(p) }
`)
	for _, test := range []struct {
		name   string
		reason resourceLifetimeReason
		want   bool
	}{
		{"send", resourceReasonSentToChannel, true},
		{"unrelated", resourceReasonSentToChannel, false},
		{"put", resourceReasonStoredInMap, true},
		{"choose", resourceReasonSentToChannel, true},
		{"dynamic", resourceReasonDynamicCallee, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var payload ssa.Instruction
			for instruction := range ssaflow.InstructionsWithin(fn, nil) {
				switch instruction.(type) {
				case *ssa.Send, *ssa.MapUpdate, *ssa.Select, *ssa.Call:
					payload = instruction
				}
			}
			if payload == nil {
				t.Fatalf("no payload in SSA:\n%s", carriedSSA(t, fn))
			}
			query := aggregateEscapeAnalysis(fn)
			query.pool = proofs.NewSearchBudget(0)
			if reason, opaque := query.opaqueConsumption(payload); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted payload = %v/%v", reason, opaque)
			}
			query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			if reason, opaque := query.opaqueConsumption(payload); opaque != test.want || reason != test.reason {
				t.Fatalf("fresh payload = %v/%v, want %v/%v", reason, opaque, test.reason, test.want)
			}
		})
	}
}

func carriedSSA(t *testing.T, fn *ssa.Function) string {
	t.Helper()
	var output strings.Builder
	if _, err := fn.WriteTo(&output); err != nil {
		t.Fatalf("write SSA: %v", err)
	}
	return output.String()
}

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
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
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
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveClosureCarryWithin(closure, budget)
			}, name != "unrelated")
		})
	}
}

func TestCarriedCallArgumentsAllowance(t *testing.T) {
	pkg, provider := carriedCallbackFixture(t)
	type callbackFamily uint8
	const (
		retention callbackFamily = iota
		aggregate
		loop
	)
	labels := [...]string{retention: "retention", aggregate: "aggregate", loop: "loop"}
	for _, test := range []struct {
		family callbackFamily
		name   string
		want   bool
	}{
		{retention, "registered", true},
		{retention, "observed", false},
		{retention, "borrowed", false},
		{retention, "otherCallback", false},
		{retention, "directCall", false},
		{aggregate, "mixed", true},
		{aggregate, "aggregate", true},
		{aggregate, "callbackArg", false},
		{aggregate, "otherAggregate", false},
		{aggregate, "directCall", false},
		{loop, "loopDirect", true},
		{loop, "loopAggregate", true},
		{loop, "loopOther", false},
	} {
		t.Run(labels[test.family]+"/"+test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				switch test.family {
				case retention:
					return query.provePossiblyRetainedCallbackWithin(call, call.Common(), budget)
				case aggregate:
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
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueClosureCall(launched, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted closure classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if reason, opaque := query.opaqueClosureCall(launched, closure, false); !opaque || reason != resourceReasonCapturedByStartedLiteral {
		t.Fatalf("fresh closure classifier = %v/%v", reason, opaque)
	}
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

func TestCapturedAggregateOwnerAllowance(t *testing.T) {
	pkg := capturedOwnerFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"owner", true},
		{"reassigned", true},
		{"unrelated", false},
		{"self", false},
		{"scalar", false},
		{"value", false},
		{"empty", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			closure := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)[0]
			query := aggregateEscapeAnalysis(fn)
			query.owners = []ssa.Value{fn.Params[1]}
			if test.name == "self" {
				query.owners = []ssa.Value{fn.Params[0]}
			}
			if test.name == "empty" {
				query.owners = nil
			}
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveCapturedAggregateOwnerWithin(closure, budget)
			}, test.want)
			if got := query.proveCapturedAggregateOwnerWithin(closure, nil); test.want && got.Reason != resourceReasonCapturedAggregateOwner {
				t.Fatalf("owner capture reason = %+v; SSA:\n%s", got, carriedSSA(t, fn))
			}
		})
	}
}

func TestCapturedOwnerClassifierCutoff(t *testing.T) {
	pkg := capturedOwnerFixture(t)
	for _, name := range []string{"owner", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			closure := call.Common().Value.(*ssa.MakeClosure)
			query := aggregateEscapeAnalysis(fn)
			query.owners = []ssa.Value{fn.Params[1]}
			query.pool = proofs.NewSearchBudget(2)
			if reason, opaque := query.opaqueClosureCall(call, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted owner classifier = %v/%v", reason, opaque)
			}
			query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			reason, opaque := query.opaqueClosureCall(call, closure, false)
			if name == "owner" {
				if !opaque || reason != resourceReasonCapturedAggregateOwner {
					t.Fatalf("fresh captured-owner classifier = %v/%v", reason, opaque)
				}
			} else if opaque {
				t.Fatalf("fresh unrelated classifier = %v/%v", reason, opaque)
			}
		})
	}
}

func TestCapturedOwnerChildCutoff(t *testing.T) {
	pkg := capturedOwnerFixture(t)
	for _, name := range []string{"owner", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			closure := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)[0]
			query := aggregateEscapeAnalysis(fn)
			query.owners = []ssa.Value{fn.Params[1]}
			pool := proofs.NewSearchBudget(proofs.SummaryBudget)
			proof := query.proveCapturedAggregateOwnerWithin(closure, pool.Within(2))
			if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(pool) {
				t.Fatalf("owner child cutoff = %+v, parent exhausted %v", proof, resourceFlowExhausted(pool))
			}
		})
	}
}

func TestCapturedOwnerDiscoveredAfterClosure(t *testing.T) {
	fn := capturedOwnerFixture(t).Func("late")
	query := aggregateEscapeAnalysis(fn)
	query.owners = proveLocalResourceOwnersWithin(fn, query.resource, nil).Owners
	closure := ssaflow.InstructionsOf[*ssa.MakeClosure](fn)[0]
	// The SSA has both a captured pointer cell and the holder loaded from it.
	// Discovery may retain both; the capture proof owns their type exclusions.
	if len(query.owners) == 0 {
		t.Fatalf("discovered owners = %v; SSA:\n%s", query.owners, carriedSSA(t, fn))
	}
	proof := query.proveCapturedAggregateOwnerWithin(closure, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !proof.Proven() || proof.Reason != resourceReasonCapturedAggregateOwner {
		t.Fatalf("late-populated owner capture = %+v; SSA:\n%s", proof, carriedSSA(t, fn))
	}
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	if reason, opaque := query.opaqueClosureCall(call, closure, false); !opaque || reason != resourceReasonCapturedAggregateOwner {
		t.Fatalf("late-populated owner classifier = %v/%v", reason, opaque)
	}
}

func capturedOwnerFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "capturedowner", `package capturedowner
type resource struct { n int }
type holder struct { value *resource }
func owner(p *resource, h, other *holder) { func(){ println(h) }() }
func reassigned(p *resource, h, other *holder) { saved := other; saved = h; func(){ println(saved) }() }
func unrelated(p *resource, h, other *holder) { func(){ println(other) }() }
func self(p *resource, h, other *holder) { func(){ println(p) }() }
func scalar(p *resource, n *int) { func(){ println(n) }() }
func value(p *resource, h holder) { func(){ println(h.value) }() }
func empty(p *resource, h, other *holder) { func(){ println(h) }() }
func late(p *resource) { h := &holder{}; f := func(){ println(h.value) }; h.value = p; f() }
`)
}
