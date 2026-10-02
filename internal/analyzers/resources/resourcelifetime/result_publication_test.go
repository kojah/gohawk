package resourcelifetime

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallResultPublicationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "publication", `package publication
type owner struct { n int }
type holder struct { value *owner }
var kept any
var count int
var failure error
func makeOwner() *owner
func makeCount() int
func makeError() error
func returned() *owner { return makeOwner() }
func boxed() any { return makeOwner() }
func field() *int { return &makeOwner().n }
func global() { kept = makeOwner() }
func scalarGlobal() { count = makeCount() }
func scalarReturn() int { return makeCount() }
func errorGlobal() { failure = makeError() }
func errorReturn() error { return makeError() }
func unrelated() *owner { _ = makeOwner(); return &owner{} }
func discarded() { _ = makeOwner() }
func foreign(dest *holder) { dest.value = makeOwner() }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"returned", true},
		{"boxed", true},
		{"field", true},
		{"global", true},
		{"scalarGlobal", false},
		{"scalarReturn", true},
		{"errorGlobal", false},
		{"errorReturn", false},
		{"unrelated", false},
		{"discarded", false},
		{"foreign", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return proveCallResultMayTransferWithin(call, budget)
			}, test.want)
		})
	}
}

func TestResultPublicationCensusCutoff(t *testing.T) {
	pkg := resultPublicationCensusFixture(t)
	fn := pkg.Func("long")
	calls := ssaflow.InstructionsOf[*ssa.Call](fn)
	if len(calls) != ssaflow.SummaryBudget+101 {
		t.Fatalf("call census = %d, want %d", len(calls), ssaflow.SummaryBudget+101)
	}
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	if proof := proveCallResultMayTransferWithin(calls[0], budget); proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted publication census = %+v", proof)
	}
	if fresh := proveCallResultMayTransferWithin(calls[0], ssaflow.NewSearchBudget(10*ssaflow.SummaryBudget)); !fresh.Proven() {
		t.Fatalf("fresh publication census = %+v", fresh)
	}
}

func TestResultPublicationClassifierCutoff(t *testing.T) {
	pkg := resultPublicationCensusFixture(t)
	fn := pkg.Func("long")
	calls := ssaflow.InstructionsOf[*ssa.Call](fn)
	query := aggregateEscapeAnalysis(fn)
	query.pool = ssaflow.NewSearchBudget(10 * ssaflow.SummaryBudget)
	if reason, opaque := query.opaqueFunctionCall(calls[0], calls[0].Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("publication child-cutoff classifier = %v/%v", reason, opaque)
	}
	if resourceFlowExhausted(query.pool) {
		t.Fatal("publication child cutoff consumed the whole candidate pool")
	}
	short := pkg.Func("short")
	call := ssaflow.InstructionsOf[*ssa.Call](short)[0]
	shortQuery := aggregateEscapeAnalysis(short)
	if reason, opaque := shortQuery.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonNestedInTransferredArgument {
		t.Fatalf("complete publication classifier = %v/%v", reason, opaque)
	}
}

func resultPublicationCensusFixture(t *testing.T) *ssa.Package {
	t.Helper()
	var source strings.Builder
	source.WriteString(`package publicationcensus
type resource struct { n int }
type holder struct { value *resource }
type owner struct { n int }
func view(*holder) *owner { return &owner{} }
func noise() {}
func long(p *resource) *owner { result := view(&holder{p})
`)
	for range ssaflow.SummaryBudget + 100 {
		source.WriteString("noise()\n")
	}
	source.WriteString("return result\n}\nfunc short(p *resource) *owner { return view(&holder{p}) }")
	return ssaflowtest.BuildPackage(t, "publicationcensus", source.String())
}
