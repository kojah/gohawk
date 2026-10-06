package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
)

func BenchmarkDisabledLockBudgetTrace(b *testing.B) {
	pkg := ssaflowtest.BuildPackage(b, "budgettrace", "package budgettrace; func Work() {}")
	function := pkg.Func("Work")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	restore := analysisTrace.Capture(nil, "", nil)
	defer restore()
	b.ReportAllocs()
	for b.Loop() {
		traceLockStateBudget(pass, function)
	}
}

func TestLockBudgetTraceDisabledAllocations(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "budgettrace", "package budgettrace; func Work() {}")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	restore := analysisTrace.Capture(nil, "", nil)
	defer restore()
	allocations := testing.AllocsPerRun(100, func() {
		traceLockStateBudget(pass, pkg.Func("Work"))
	})
	if allocations != 0 {
		t.Fatalf("disabled budget tracing allocated %g times, want zero", allocations)
	}
}

func TestLockBudgetTracePreservesEnabledEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "budgettrace", "package budgettrace; func Work() {}")
	function := pkg.Func("Work")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	var records []analysisTrace.Record
	restore := analysisTrace.Capture([]string{"lockorder"}, "", func(record analysisTrace.Record) {
		records = append(records, record)
	})
	defer restore()
	traceLockStateBudget(pass, function)
	if len(records) != 1 {
		t.Fatalf("got %d events, want one budget decision", len(records))
	}
	record := records[0]
	if record.Phase != "decision" || record.Reason != "lock-state-budget-exhausted" || record.Outcome != analysisTrace.OutcomeUnknown ||
		record.Function != "budgettrace.Work" || record.Candidate != pass.Fset.Position(function.Pos()).String() {
		t.Fatalf("unexpected budget decision: %+v", record)
	}
}
