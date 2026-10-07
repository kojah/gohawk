package goroutineownership

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/reporting/check"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestConcurrencyJoinProofs(t *testing.T) {
	tracePath := enableSummaryJoinTrace(t)
	want := map[string]GoroutineOutcome{
		"importedReceive":     GoroutineLifecycleHonored,
		"importedWait":        GoroutineLifecycleHonored,
		"deferredReceive":     GoroutineLifecycleHonored,
		"branchAwareJoin":     GoroutineLifecycleHonored,
		"opaqueReceive":       GoroutineUnknown,
		"conditionalReceive":  GoroutineUnknown,
		"asynchronousReceive": GoroutineUnknown,
		"differentSignal":     GoroutineLifecycleViolated,
		"differentArgument":   GoroutineUnknown,
		"missingPath":         GoroutineLifecycleViolated,
		"returnedWaiter":      GoroutineLifecycleHonored,
		"otherReturnedWaiter": GoroutineLifecycleViolated,
		"uninvokedWaiter":     GoroutineUnknown,
		"asynchronousWaiter":  GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "summaryjoins")
	assertSummaryJoinTrace(t, tracePath)
}

func enableSummaryJoinTrace(t *testing.T) string {
	t.Helper()
	flags := flag.NewFlagSet("summary-joins", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "goroutineownership", "gohawk-trace-file": path} {
		previous := flags.Lookup(name).Value.String()
		if previous == "" {
			previous = "none"
			if name == "gohawk-trace-file" {
				previous = os.DevNull
			}
		}
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := flags.Set(name, previous); err != nil {
				t.Error(err)
			}
		})
	}
	return path
}

func assertSummaryJoinTrace(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
			Position  string `json:"position"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "concurrency-summary-join" {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "accepted" || event.Candidate == "" || event.Position == "" {
			t.Errorf("invalid summary join event: %+v", event)
		}
	}
	if !found {
		t.Error("missing concurrency-summary-join evidence")
	}
}

func TestSummaryJoinSharesCandidateAllowance(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "summarybudget", "package summarybudget; func wait(done chan int)int{n:=0;"+
		strings.Repeat("n++;", 32)+"<-done;return n};func subject(){done:=make(chan int);go func(){done<-1}();wait(done)}")
	function := pkg.Func("subject")
	var dump bytes.Buffer
	if _, err := pkg.Func("wait").WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	target := ssaflow.InstructionsOf[*ssa.MakeChan](function)[0]
	engine := concurrencyfacts.NewEngine()
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg, ResultOf: map[*analysis.Analyzer]any{concurrencyfacts.Analyzer: engine}}
	probe := analysisTrace.For(pass, "goroutineownership", string(check.GoroutineJoin), spawn.Pos())
	query := func(limit int) helperCallProof {
		candidate := &spawnAnalysis{
			pass: pass, function: function, spawn: spawn, probe: probe,
			pool: proofs.NewSearchBudget(limit).Observed(probe.Observer()), tracked: []trackedValue{{value: target, kind: trackedSignal}},
		}
		return candidate.summarizedJoin(call)
	}
	for _, limit := range []int{0, 1, 32} {
		if got := query(limit); got.action != actionUnknown || got.reason != reasonSummaryJoinBudgetExhausted {
			t.Fatalf("limit%d returned %+v; want unavailable summary", limit, got)
		}
	}
	if got := query(spawnPoolBudget); got.action != actionJoin || got.reason != reasonLabelSummaryJoin {
		t.Fatalf("fresh query=%+v", got)
	}
	// Reusing a completed summary still requires admission to this candidate's
	// allowance; a cached join must not bypass an exhausted outer pool.
	if got := query(0); got.action != actionUnknown {
		t.Fatalf("cached join bypassed cutoff: %+v", got)
	}
	assertCandidateCutoffTrace(t, path, pass.Fset.Position(spawn.Pos()).String(), func(event followupTraceEvent) bool {
		return event.Details["phase"] == "summary-join"
	})
}

func TestSummaryJoinChildCutoffAndFreshCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summarychild", "package summarychild;func wait(done chan int)int{n:=0;"+
		strings.Repeat("n++;", helperUseBudget+32)+"<-done;return n};func subject(){done:=make(chan int);go func(){done<-1}();wait(done)}")
	fn := pkg.Func("subject")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	engine := concurrencyfacts.NewEngine()
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg, ResultOf: map[*analysis.Analyzer]any{concurrencyfacts.Analyzer: engine}}
	candidate := &spawnAnalysis{
		pass: pass, function: fn, spawn: ssaflow.InstructionsOf[*ssa.Go](fn)[0],
		pool: proofs.NewSearchBudget(spawnPoolBudget), tracked: []trackedValue{{value: ssaflow.InstructionsOf[*ssa.MakeChan](fn)[0], kind: trackedSignal}},
	}
	if got := candidate.summarizedJoin(call); got.action != actionUnknown || got.reason != reasonSummaryJoinBudgetExhausted || candidate.pool.Exhausted() {
		t.Fatalf("child cutoff with live pool returned %+v, pool exhausted=%v", got, candidate.pool.Exhausted())
	}
	// A larger independent engine query may complete the same callee. A child
	// cutoff must not have cached its interrupted prefix as a final answer.
	summary := engine.AtCall(call, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !summary.Complete() {
		t.Fatalf("fresh engine query unavailable: %+v", summary)
	}
	if got := candidate.summarizedJoin(call); got.action != actionJoin {
		t.Fatalf("fresh cached summary unavailable: %+v", got)
	}
}
