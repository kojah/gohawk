package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestUnobservedSignalSnapshots(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "signalsnapshot", `package signalsnapshot
 func subject(){var done chan int;before:=done;done=make(chan int);go func(){close(done)}();println(before)}
 `)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	proof := candidate.prove()
	if proof.Outcome != GoroutineUnknown || proof.Reason != reasonUnobservedSignal {
		t.Fatalf("nil snapshot invented a protocol: %+v", proof)
	}
}

func TestUnobservedSignalCensusCutoff(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	source := `package signalcut
 func ignore(done chan int){}
 func finish(done chan int){close(done)}
 func subject(){done:=make(chan int);` + strings.Repeat("ignore(done);", ssaflow.QueryBudget+1) + `go finish(done)}
 `
	pkg := ssaflowtest.BuildPackage(t, "signalcut", source)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	child := candidate.pool.Within(ssaflow.QueryBudget)
	got := candidate.proveUnobservedSignalsWithin(child)
	if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted || !child.Exhausted() || candidate.pool.Exhausted() {
		t.Fatalf("cut=%+v", got)
	}
	cutoff := candidate.lifetimeCutoff(child, querySignalCensus, reasonSignalCensusUnavailable)
	if cutoff.Outcome != GoroutineUnknown || cutoff.Reason != reasonSignalCensusUnavailable {
		t.Fatalf("signal cutoff projection: %+v", cutoff)
	}
	fresh := candidate.proveUnobservedSignalsWithin(candidate.pool.Within(2 * ssaflow.SummaryBudget))
	if !fresh.Proven() {
		t.Fatalf("fresh=%+v", fresh)
	}
	// The bounded pre-spawn census now encounters this huge prefix first;
	// the direct signal query above still pins its own cutoff and recovery.
	if proof := candidate.prove(); proof.Outcome != GoroutineUnknown || proof.Reason != reasonPreSpawnCensusCutoff {
		t.Fatalf("cutoff revived diagnostic: %+v", proof)
	}
	requireCensusCutoffTrace(t, path, "signal-census")
}

func requireCensusCutoffTrace(t *testing.T, path, label string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate != "" && event.Details["phase"] == label {
			return
		}
	}
	t.Fatalf("%s cutoff has no attributed evidence event", label)
}
