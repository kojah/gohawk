package resourcelifetime

import (
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestOwnershipEffectsAllowance(t *testing.T) {
	pkg := ownershipEffectsFixture(t)
	for _, test := range []struct {
		name string
		wrap bool
		want bool
	}{
		{"borrowed", false, false},
		{"retained", false, true},
		{"async", false, true},
		{"unavailable", false, true},
		{"unrelated", false, false},
		{"wrapperBorrowed", true, false},
		{"wrapperRetained", true, true},
		{"wrapperUnavailable", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := aggregateEscapeAnalysis(fn)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				if test.wrap {
					return query.provePossibleWrapperWithin(call, 0, false, budget)
				}
				return query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
			}, test.want)
		})
	}
}

func TestOwnershipEffectsChildCutoff(t *testing.T) {
	pkg := ownershipEffectsFixture(t)
	for _, name := range []string{"longAggregate", "longWrapper", "publishSlow"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			query := aggregateEscapeAnalysis(fn)
			budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			var proof resourceProof
			switch name {
			case "longAggregate":
				proof = query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
			case "longWrapper":
				proof = query.provePossibleWrapperWithin(call, 0, false, budget)
			default:
				stores := ssaflow.InstructionsOf[*ssa.Store](fn)
				proof = query.proveWrapperStoredOnForeignOwnerWithin(stores[len(stores)-1], budget)
			}
			if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
				t.Fatalf("ownership effect child cutoff = %+v, caller exhausted %v; SSA:\n%s", proof, resourceFlowExhausted(budget), carriedSSA(t, fn))
			}
		})
	}
}

func TestOwnershipEffectsClassifierCutoff(t *testing.T) {
	pkg := ownershipEffectsFixture(t)
	for _, name := range []string{"longAggregate", "publishSlow"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			query := aggregateEscapeAnalysis(fn)
			query.pool = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
			classify := func(query *resourceAnalysis, fn *ssa.Function) (resourceLifetimeReason, bool) {
				if name == "longAggregate" {
					call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
					return query.opaqueFunctionCall(call, call.Common(), true)
				}
				stores := ssaflow.InstructionsOf[*ssa.Store](fn)
				return query.opaqueConsumption(stores[len(stores)-1])
			}
			reason, opaque := classify(query, fn)
			if !opaque || reason != resourceReasonBudgetExhausted || resourceFlowExhausted(query.pool) {
				t.Fatalf("effect child-cutoff classifier = %v/%v, pool exhausted %v", reason, opaque, resourceFlowExhausted(query.pool))
			}
			shortName := "borrowed"
			if name == "publishSlow" {
				shortName = "publishBorrowed"
			}
			short := pkg.Func(shortName)
			fresh := aggregateEscapeAnalysis(short)
			fresh.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
			if reason, opaque := classify(fresh, short); opaque {
				t.Fatalf("fresh short-helper classifier = %v/%v", reason, opaque)
			}
		})
	}
}

func ownershipEffectsFixture(t *testing.T) *ssa.Package {
	t.Helper()
	source := `package ownershipeffects
type resource struct { n int }
type holder struct { value *resource }
type owner struct { value any }
var kept *holder
func read(h *holder) { _ = h.value }
func retain(h *holder) { kept = h }
func launch(h *holder) { go read(h) }
func unknown(*holder)
func wrapUnknown(*holder) *owner
func wrapRead(h *holder) *owner { read(h); return &owner{} }
func wrapRetain(h *holder) *owner { retain(h); return &owner{} }
func borrowed(p *resource) { read(&holder{p}) }
func retained(p *resource) { retain(&holder{p}) }
func async(p *resource) { launch(&holder{p}) }
func unavailable(p *resource) { unknown(&holder{p}) }
func unrelated(p, other *resource) { retain(&holder{other}) }
func wrapperBorrowed(p *resource) *owner { return wrapRead(&holder{p}) }
func wrapperRetained(p *resource) *owner { return wrapRetain(&holder{p}) }
func wrapperUnavailable(p *resource) *owner { return wrapUnknown(&holder{p}) }
func slow(h *holder) {
` + strings.Repeat("read(h)\n", proofs.QueryBudget+100) + `}
func wrapSlow(h *holder) *owner { slow(h); return &owner{} }
func longAggregate(p *resource) { slow(&holder{p}) }
func longWrapper(p *resource) *owner { return wrapSlow(&holder{p}) }
func publishSlow(p *resource, dest *owner) { dest.value = wrapSlow(&holder{p}) }
func publishBorrowed(p *resource, dest *owner) { dest.value = wrapRead(&holder{p}) }
`
	return ssaflowtest.BuildPackage(t, "ownershipeffects", source)
}

func TestResourceOwnerDiscoveryAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerdiscovery", `package ownerdiscovery
type resource struct { n int }
type holder struct { value *resource }
func local(p, other *resource, dest *holder) { h := &holder{p}; h.value = p }
func foreign(p, other *resource, dest *holder) { dest.value = p }
func unrelated(p, other *resource, dest *holder) { h := &holder{other}; h.value = other }
func destination() **resource
func opaque(p, other *resource, dest *holder) { *destination() = p }
`)
	for _, test := range []struct {
		name  string
		count int
	}{{"local", 1}, {"foreign", 0}, {"unrelated", 0}, {"opaque", 1}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			baseline := proveLocalResourceOwnersWithin(fn, fn.Params[0], nil)
			if !baseline.Proven() || len(baseline.Owners) != test.count {
				t.Fatalf("default owner census = %+v; SSA:\n%s", baseline, carriedSSA(t, fn))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveLocalResourceOwnersWithin(fn, fn.Params[0], budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted || len(got.Owners) != 0 || len(got.Stores) != 0 {
						t.Fatalf("interrupted census = %+v", got)
					}
					continue
				}
				if !got.Proven() || !slices.Equal(got.Owners, baseline.Owners) {
					t.Fatalf("complete census = %+v, want %+v", got, baseline)
				}
				query := aggregateEscapeAnalysis(fn)
				query.stores = got.Stores
				query.pool = proofs.NewSearchBudget(0)
				for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
					if proof := query.resourceStorage(store); proof != got.Stores[store] || resourceFlowExhausted(query.pool) {
						t.Fatalf("discovered storage disposition repeated work: %+v", proof)
					}
				}
				return
			}
			t.Fatal("owner census never completed")
		})
	}
}

func TestResourceOwnerDiscoveryChildCutoff(t *testing.T) {
	pkg := ownerDiscoveryFixture(t)
	fn := pkg.Func("leak")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := &resourceAnalysis{function: fn, resource: call}
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.discoverResourceOwnersWithin(pool.Within(2))
	if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted || len(got.Owners) != 0 ||
		len(got.Stores) != 0 || resourceFlowExhausted(pool) || len(query.owners) != 0 || len(query.stores) != 0 {
		t.Fatalf("owner census child cutoff = %+v, parent exhausted %v", got, resourceFlowExhausted(pool))
	}
	fresh := query.discoverResourceOwnersWithin(pool.Within(releaseSearchBudget))
	if !fresh.Proven() || len(query.owners) != 1 || len(query.stores) == 0 {
		t.Fatalf("fresh discovery failed to commit complete inputs: %+v", fresh)
	}
	query.pool = proofs.NewSearchBudget(0)
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
		if proof := query.resourceStorage(store); proof != fresh.Stores[store] || resourceFlowExhausted(query.pool) {
			t.Fatalf("committed storage disposition repeated work: %+v", proof)
		}
	}
}

func TestResourceOwnerDiscoveryFlow(t *testing.T) {
	pkg := ownerDiscoveryFixture(t)
	evidence, _ := resourceSummaries.Provider(nil).LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	for _, name := range []string{"leak", "released"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			got := evaluateResourceFlow(nil, evidence, call, call, resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("undisposed resource lost leak witness = %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("released resource reported = %+v", got)
			}
		})
	}
}

func ownerDiscoveryFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "ownerdiscoveryflow", `package ownerdiscoveryflow
 type resource struct { n int }
 func (*resource) Close() {}
 type holder struct { value *resource }
 func acquire() *resource { return &resource{} }
 func leak() { p := acquire(); _ = &holder{p} }
 func released() { p := acquire(); _ = &holder{p}; p.Close() }
 `)
}

func TestAsynchronousExposureAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "asyncallowance", `package asyncallowance
type resource struct { n int }
func read(p *resource) { _ = p.n }
func async(p *resource) { go read(p) }
func imported(*resource)
func opaque(*resource)
func observed(p, other *resource) { read(p) }
func launched(p, other *resource) { async(p) }
func exported(p, other *resource) { imported(p) }
func unrelated(p, other *resource) { imported(other) }
func unreadable(p, other *resource) { opaque(p) }
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot: heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}}, Escape: heapmodel.HeapEscapedAsync,
		}},
	}}
	fact.DescribeFact(pkg.Func("imported").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("imported"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	for _, test := range []struct {
		name string
		want bool
	}{{"observed", false}, {"launched", true}, {"exported", true}, {"unrelated", false}, {"unreadable", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveAsynchronousExposureWithin(call, call.Common(), budget)
			}, test.want)
		})
	}
}

func TestAsynchronousEffectChildCutoff(t *testing.T) {
	pkg := asyncExposureCapFixture(t)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	proof := query.proveAsynchronousExposureWithin(call, call.Common(), budget)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
		t.Fatalf("effect child cutoff = %+v, caller exhausted %v", proof, resourceFlowExhausted(budget))
	}
}

func TestAsynchronousExposureClassifierCutoff(t *testing.T) {
	pkg := asyncExposureCapFixture(t)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	if reason, opaque := query.opaqueCall(call, call.Common()); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("effect child-cutoff classifier = %v/%v", reason, opaque)
	}
	if resourceFlowExhausted(query.pool) {
		t.Fatal("effect child cutoff exhausted candidate pool")
	}
	short := pkg.Func("short")
	borrow := ssaflow.InstructionsOf[*ssa.Call](short)[0]
	if reason, opaque := aggregateEscapeAnalysis(short).opaqueCall(borrow, borrow.Common()); opaque {
		t.Fatalf("fresh borrowing classifier = %v/%v", reason, opaque)
	}
}

func asyncExposureCapFixture(t *testing.T) *ssa.Package {
	t.Helper()
	source := `package asynccap
type resource struct { n int }
func read(p *resource) { _ = p.n }
func many(p *resource) {
` + strings.Repeat("read(p)\n", proofs.QueryBudget+100) +
		"}\nfunc caller(p *resource) { many(p) }\nfunc short(p *resource) { read(p) }"
	return ssaflowtest.BuildPackage(t, "asynccap", source)
}

func TestResourceStorageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storebudget", `package storebudget
type resource struct{}
type holder struct { value *resource }
type nested struct { owner *holder }
func foreign(value, other *resource, dst *holder) { dst.value = value }
func local(value, other *resource, dst *holder) { own := &holder{}; own.value = value }
func unrelated(value, other *resource, dst *holder) { dst.value = other }
func contained(value, other *resource, dst *nested) { dst.owner = &holder{value} }
func indirect(value, other *resource, dst *holder) {
 entry := struct { destination **resource }{&dst.value}; *entry.destination = value
}
func replaced(value, other *resource, dst *holder) {
 own := &holder{}
 entry := struct { destination **resource }{&dst.value}
 entry.destination = &own.value; *entry.destination = value
}
func destination() **resource
func opaque(value, other *resource, dst *holder) {
 entry := struct { destination **resource }{destination()}; *entry.destination = value
}
`)
	for _, test := range []struct {
		name  string
		state proofs.EvidenceState
		owner bool
	}{
		{"foreign", proofs.EvidenceProven, true},
		{"local", proofs.EvidenceDisproven, true},
		{"unrelated", proofs.EvidenceDisproven, false},
		{"contained", proofs.EvidenceProven, true},
		{"indirect", proofs.EvidenceProven, true},
		{"replaced", proofs.EvidenceDisproven, true},
		{"opaque", proofs.EvidenceUnknown, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			stores := ssaflow.InstructionsOf[*ssa.Store](fn)
			store := stores[len(stores)-1]
			baseline := proveResourceStorage(store, fn.Params[0], nil)
			if baseline.State != test.state || (baseline.Owner != nil) != test.owner {
				t.Fatalf("default storage = %+v, want %v/owner %v", baseline, test.state, test.owner)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := proveResourceStorage(store, fn.Params[0], budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted storage: %+v", limit, got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete storage = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("storage never completed")
		})
	}
}

func TestResourceStorageCutoffRetryAndReuse(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storereuse", `package storereuse
type resource struct{}
type holder struct { value *resource }
func foreign(value *resource, dst *holder) { dst.value = value }
`)
	fn := pkg.Func("foreign")
	store := ssaflow.InstructionsOf[*ssa.Store](fn)[0]
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	query := &resourceAnalysis{
		function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence,
		pool: proofs.NewSearchBudget(0),
	}
	if action, reason := query.releasesOrdinaryResource(store); action != actionUnknown || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted release = %v/%v", action, reason)
	}
	if len(query.stores) != 0 {
		t.Fatal("interrupted storage proof was memoized")
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if action, reason := query.releasesOrdinaryResource(store); action != actionSettled || reason != resourceReasonSettled {
		t.Fatalf("fresh release = %v/%v", action, reason)
	}
	if len(query.stores) != 1 {
		t.Fatal("completed storage proof was not memoized")
	}
	// Reuse needs no second containment/destination query. An empty replacement
	// pool exposes any repeated work through its exhaustion marker.
	query.pool = proofs.NewSearchBudget(0)
	if got := query.resourceStorage(store); !got.Proven() || query.pool.Exhausted() {
		t.Fatalf("completed proof repeated work: %+v, exhausted %v", got, query.pool.Exhausted())
	}
}
