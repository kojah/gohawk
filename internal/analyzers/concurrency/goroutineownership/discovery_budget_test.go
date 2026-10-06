package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/check"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestDiscoveryBudgetIsObserved(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	source := "package discovery; func subject() { done:=make(chan int); go func() { n:=0;" +
		strings.Repeat("n++;", proofs.SummaryBudget+32) + "done<-n }() }"
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
		spawn: ssaflow.InstructionsOf[*ssa.Go](function)[0], pool: proofs.NewSearchBudget(64),
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

func TestAdapterDiscoveryCutoffs(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "adapters", `package adapters
import "sync"
type owner struct{}
func (*owner) Close() {}
func relay() { group:=new(sync.WaitGroup); done:=make(chan int); go func(){group.Wait();close(done)}() }
func owned() { value:=new(owner); go func(){value.Close()}() }
func empty() { go func(){}() }
`)
	for _, test := range []struct {
		name, phase string
		limit       int
	}{
		{"relay", "relay-discovery", 0},
		{"owned", "owner-discovery", 1},
		{"empty", "pipe-peer-discovery", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
			pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
			probe := analysisTrace.For(pass, "goroutineownership", string(check.GoroutineJoin), spawn.Pos())
			candidate := &spawnAnalysis{
				pass: pass, function: function, spawn: spawn,
				discoveryBudget: proofs.NewSearchBudget(test.limit).Observed(probe.Observer()),
			}
			if test.name == "relay" {
				candidate.signals = []ssa.Value{ssaflow.InstructionsOf[*ssa.MakeChan](function)[0]}
			}
			candidate.discoverAdapters()
			if proof := candidate.prove(); proof.Outcome != GoroutineUnknown || proof.Reason != reasonDiscoveryBudgetExhausted {
				t.Fatalf("adapter cutoff must stay unknown: %+v", proof)
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
				if event.Phase == "evidence" && event.Reason == "budget-exhausted" &&
					event.Candidate == pass.Fset.Position(spawn.Pos()).String() && event.Details["phase"] == test.phase {
					observed = true
				}
			}
			if !observed {
				t.Fatalf("missing attributed %s cutoff", test.phase)
			}
		})
	}
}
