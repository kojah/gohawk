package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestResourceMetadataTraceDisabledAllocations(t *testing.T) {
	pass, branch, phi := resourceMetadataTraceFixture(t)
	proof := optionalAcquisitionProof{proof: resourceProof{Reason: optionalAcquisitionSuccessPhi}, resourcePhi: phi}
	restore := analysisTrace.Capture(nil, "", nil)
	defer restore()
	for _, test := range []struct {
		name string
		emit func()
	}{
		{"acquisition error", func() {
			traceAcquisitionErrorProof(pass, branch, resourceReasonTestifyNoErrorGuard, branch.Parent().Pos())
		}},
		{"optional acquisition", func() { traceOptionalAcquisition(pass, proof, phi.Parent().Pos()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(100, test.emit); allocations != 0 {
				t.Fatalf("disabled tracing allocated %g times, want zero", allocations)
			}
		})
	}
}

func TestResourceMetadataTracePreservesEnabledEvidence(t *testing.T) {
	pass, branch, phi := resourceMetadataTraceFixture(t)
	var records []analysisTrace.Record
	restore := analysisTrace.Capture([]string{"resourcelifetime"}, "", func(record analysisTrace.Record) { records = append(records, record) })
	defer restore()
	candidate := branch.Parent().Pos()
	traceAcquisitionErrorProof(pass, branch, resourceReasonTestifyNoErrorGuard, candidate)
	traceOptionalAcquisition(pass, optionalAcquisitionProof{proof: resourceProof{Reason: optionalAcquisitionSuccessPhi}, resourcePhi: phi}, candidate)
	if len(records) != 2 {
		t.Fatalf("got %d events, want two evidence events", len(records))
	}
	for index, reason := range []string{"acquisition-error-proven", "optional-acquisition-success-phi"} {
		record := records[index]
		if record.Phase != "evidence" || record.Reason != reason || record.Outcome != analysisTrace.OutcomeAccepted ||
			record.Function != "tracealloc.Work" || record.Candidate != pass.Fset.Position(candidate).String() {
			t.Fatalf("unexpected evidence event: %+v", record)
		}
	}
}

func resourceMetadataTraceFixture(tb testing.TB) (*analysis.Pass, *ssa.If, *ssa.Phi) {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "tracealloc", `package tracealloc
func Work(flag bool) int { value := 0; if flag { value = 1 }; return value }`)
	function := pkg.Func("Work")
	branches := ssaflow.InstructionsOf[*ssa.If](function)
	phis := ssaflow.InstructionsOf[*ssa.Phi](function)
	if len(branches) != 1 || len(phis) != 1 {
		tb.Fatal("fixture must contain one branch and one merged value")
	}
	return &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}, branches[0], phis[0]
}
