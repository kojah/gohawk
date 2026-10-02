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

func TestDiscoveryBudgetIsObserved(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	source := "package discovery; func subject() { done:=make(chan int); go func() { n:=0;" +
		strings.Repeat("n++;", ssaflow.SummaryBudget+32) + "done<-n }() }"
	pkg := ssaflowtest.BuildPackage(t, "discovery", source)
	function := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	candidate := newSpawnAnalysis(pass, function, spawn)
	if proof := candidate.prove(); proof.Outcome != GoroutineUnknown {
		t.Fatalf("incomplete discovery must be unknown: %+v", proof)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	observed := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate != "" {
			observed = true
		}
	}
	if !observed {
		t.Fatal("discovery cutoff must be observed with its spawn candidate")
	}
}

func TestPartialDiscoveryCannotReport(t *testing.T) {
	source := "package discovery; func worker(done chan int) { n:=0;" + strings.Repeat("n++;", 32) +
		"done<-n }; func subject() { done:=make(chan int); go worker(done) }"
	pkg := ssaflowtest.BuildPackage(t, "discovery", source)
	function := pkg.Func("subject")
	candidate := &spawnAnalysis{
		pass: &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}, function: function,
		spawn: ssaflow.InstructionsOf[*ssa.Go](function)[0], pool: ssaflow.NewSearchBudget(64),
	}
	candidate.discoverCompletion()
	if len(candidate.signals) == 0 || !candidate.discoveryBudget.Exhausted() || !candidate.discoveryBudget.PoolExhausted() {
		t.Fatalf("expected signal then cutoff: signals=%d exhausted=%v pool=%v",
			len(candidate.signals), candidate.discoveryBudget.Exhausted(), candidate.discoveryBudget.PoolExhausted())
	}
	if proof := candidate.prove(); proof.Outcome != GoroutineUnknown || proof.Reason != reasonDiscoveryBudgetExhausted {
		t.Fatalf("partial evidence must remain unknown: %+v", proof)
	}
	// A new candidate with a full pool must rediscover the available promise;
	// neither a budget cutoff nor its dependent answer may poison this query.
	fresh := newSpawnAnalysis(candidate.pass, function, candidate.spawn)
	if proof := fresh.prove(); proof.Outcome != GoroutineLifecycleViolated {
		t.Fatalf("fresh discovery must retain missing-join diagnostic: %+v", proof)
	}
}
