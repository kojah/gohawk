package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
			checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
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
			query.pool = ssaflow.NewSearchBudget(2)
			if reason, opaque := query.opaqueClosureCall(call, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted owner classifier = %v/%v", reason, opaque)
			}
			query.pool = ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
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
			pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			proof := query.proveCapturedAggregateOwnerWithin(closure, pool.Within(2))
			if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(pool) {
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
	proof := query.proveCapturedAggregateOwnerWithin(closure, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
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
