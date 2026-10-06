package resourcelifetime

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
