package goroutineownership

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/reporting/check"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
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

func TestCompletionDiscoveryNilAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilallowance", `package nilallowance
import "sync"
func signal(done chan int) { done <- 1 }
func direct(group *sync.WaitGroup) { group.Done() }
func deferred(group *sync.WaitGroup) { defer group.Done() }
func nilSignal() { go signal(nil) }
func nilDirect() { go direct(nil) }
func nilDeferred() { go deferred(nil) }
func liveSignal() { go signal(make(chan int)) }
`)
	for _, test := range []struct {
		name       string
		beforeFold int
		signals    int
	}{
		{"nilSignal", 11, 0}, {"nilDirect", 10, 0}, {"nilDeferred", 14, 0}, {"liveSignal", 11, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
			t.Log(spawn.String())
			// The parent completed these actual SSA queries at these allowances
			// without charging its nil folds. They now cut before discovery can
			// publish even an already found completion handle.
			pool := proofs.NewSearchBudget(spawnPoolBudget)
			child := pool.Within(test.beforeFold)
			signals, groups, _ := spawnedCompletionValues(nil, spawn, child)
			if !child.Exhausted() || pool.Exhausted() {
				t.Fatal("nil query bypassed its child allowance or exhausted the outer pool")
			}
			candidate := &spawnAnalysis{function: fn, spawn: spawn, signals: signals, groups: groups, discoveryBudget: child}
			got := candidate.prove()
			if got.Outcome != GoroutineUnknown || got.Reason != reasonDiscoveryBudgetExhausted {
				t.Fatalf("nil evidence bypassed the discovery allowance: %+v, exhausted %v/%v", got, child.Exhausted(), pool.Exhausted())
			}
			complete := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				signals, groups, _ := spawnedCompletionValues(nil, spawn, budget)
				if budget.Exhausted() {
					continue
				}
				if len(signals) != test.signals || len(groups) != 0 {
					t.Fatalf("fresh nil policy changed: signals=%d groups=%d", len(signals), len(groups))
				}
				complete = true
				break
			}
			if !complete {
				t.Fatal("fresh completion discovery never finished after cutoff")
			}
		})
	}
}

func TestDominatingCensusCutoff(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "spawncensus", `package spawncensus
 func subject(){done:=make(chan int);go func(){close(done)}();<-done}
 `)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	candidate.pool = proofs.NewSearchBudget(0).Observed(candidate.probe.Observer())
	proof, decided := candidate.dominatingProof()
	if !decided || proof.Outcome != GoroutineUnknown || proof.Reason != reasonPreSpawnCensusCutoff {
		t.Fatalf("cutoff: %+v decided=%v", proof, decided)
	}
	requireCensusCutoffTrace(t, path, "pre-spawn-census")
	candidate.pool = proofs.NewSearchBudget(spawnPoolBudget)
	if proof, decided := candidate.dominatingProof(); decided {
		t.Fatalf("fresh prefix invented cleanup: %+v", proof)
	}
}

func TestWorkerOutputNilAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "outputnil", `package outputnil
func publish(done chan int)
func disabled(){publish(nil)}
func enabled(done chan int){publish(done)}
`)
	for _, test := range []struct {
		name        string
		parentLimit int
		publishes   bool
	}{
		{"disabled", 3, false}, {"enabled", 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			pool := proofs.NewSearchBudget(spawnPoolBudget)
			child := pool.Within(test.parentLimit)
			got := workerHandsOffOutputChannelWithin(fn, child)
			t.Logf("%s: publishes=%v child exhausted=%v pool exhausted=%v", test.name, got, child.Exhausted(), pool.Exhausted())
			if !child.Exhausted() || pool.Exhausted() || got {
				t.Fatalf("unfinished publication query=%v, exhausted=%v/%v", got, child.Exhausted(), pool.Exhausted())
			}
			fresh := proofs.NewSearchBudget(proofs.SummaryBudget)
			if got := workerHandsOffOutputChannelWithin(fn, fresh); got != test.publishes || fresh.Exhausted() {
				t.Fatalf("fresh publication query=%v, exhausted=%v", got, fresh.Exhausted())
			}
		})
	}
}

func TestCompletionCapturePrecedesArgumentMetadata(t *testing.T) {
	var parameters, arguments []string
	for index := range 64 {
		parameters = append(parameters, fmt.Sprintf("p%d int", index))
		arguments = append(arguments, "0")
	}
	pkg := ssaflowtest.BuildPackage(t, "bindingmetadata", "package bindingmetadata; type owner struct{n int}; func subject(){var group owner; go func("+
		strings.Join(parameters, ",")+"){group.n++}("+strings.Join(arguments, ",")+")}")
	function := pkg.Func("subject")
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	worker, closure := ssacall.DirectCallee(spawn.Common())
	owner := closure.Bindings[0]
	// The capture is exact without consulting any of the unrelated parameters.
	// A fixed allowance must not be spent preparing their metadata first.
	budget := proofs.NewSearchBudget(32)
	if got := completionValueAtCall(spawn, worker, closure, worker.FreeVars[0], budget); got != owner || budget.Exhausted() {
		t.Fatalf("capture binding = %v, exhausted=%v; want %v", got, budget.Exhausted(), owner)
	}
	for limit := range 32 {
		budget := proofs.NewSearchBudget(limit)
		got := completionValueAtCall(spawn, worker, closure, worker.FreeVars[0], budget)
		if budget.Exhausted() && got != nil {
			t.Fatalf("limit %d published capture after cutoff: %v", limit, got)
		}
		if got != nil && got != owner {
			t.Fatalf("limit %d selected another binding: %v", limit, got)
		}
	}
}
