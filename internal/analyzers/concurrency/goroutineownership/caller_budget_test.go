package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestCallerLifetimeCutoffAvailability(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "bounds", `package bounds
import ("context"; "sync")
type owner struct { ctx context.Context }
func channel(stop <-chan int) { done:=make(chan int); go func(){<-stop;close(done)}() }
func receiver(p *owner) { done:=make(chan int); go func(){<-p.ctx.Done();close(done)}() }
func local() { ctx,cancel:=context.WithCancel(context.Background()); defer cancel(); done:=make(chan int); go func(){<-ctx.Done();close(done)}() }
func relay() { group:=new(sync.WaitGroup); done:=make(chan int); go func(){group.Wait();close(done)}(); group.Wait() }
`)
	for _, test := range []struct {
		name          string
		fresh, cutoff goroutineOwnershipReason
	}{
		{"channel", reasonStopLifecycle, reasonFactoryOriginBudgetExhausted},
		{"receiver", reasonReceiverContext, reasonFactoryOriginBudgetExhausted},
		{"local", reasonLocallyCanceledContext, reasonFactoryOriginBudgetExhausted},
		{"relay", reasonNone, reasonRelayDependencyBudgetExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
			pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
			candidate := newSpawnAnalysis(pass, function, spawn)
			candidate.pool = proofs.NewSearchBudget(0).Observed(candidate.probe.Observer())
			proof, decided := candidate.lifecycleProof()
			if !decided || proof.Outcome != GoroutineUnknown || proof.Reason != test.cutoff {
				t.Fatalf("cutoff=%+v decided=%v", proof, decided)
			}
			fresh := newSpawnAnalysis(pass, function, spawn)
			proof, decided = fresh.lifecycleProof()
			if proof.Reason != test.fresh || decided != (test.fresh != reasonNone) {
				t.Fatalf("fresh=%+v decided=%v, want %v", proof, decided, test.fresh)
			}
			if test.name != "relay" {
				assertCallerLifetimeQueryCutoff(t, fresh)
			}
		})
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	phases := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate != "" {
			phases[event.Details["phase"]] = true
		}
	}
	if !phases["factory-origin"] || !phases["caller-lifetime"] || !phases["relay-dependency"] {
		t.Fatalf("missing attributed cutoff phases: %v", phases)
	}
}

// Once origin admission is complete, the caller-lifetime query still owns
// its independent cutoff and attributed evidence phase.
func assertCallerLifetimeQueryCutoff(t *testing.T, candidate *spawnAnalysis) {
	t.Helper()
	candidate.pool = proofs.NewSearchBudget(0).Observed(candidate.probe.Observer())
	proof, decided := candidate.callerLifetimeProof()
	if !decided || proof.Outcome != GoroutineUnknown || proof.Reason != reasonReceiveBudgetExhausted {
		t.Fatalf("caller cutoff=%+v decided=%v", proof, decided)
	}
}

func TestCancellationCoverageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cancelcoverage", `package cancelcoverage
import "context"
func covered() { _,cancel:=context.WithCancel(context.Background()); go func(){}(); cancel() }
func deferred() { _,cancel:=context.WithCancel(context.Background()); defer cancel(); go func(){}() }
func conditional(skip bool) { _,cancel:=context.WithCancel(context.Background()); go func(){}(); if skip{return}; cancel() }
func asynchronous() { _,cancel:=context.WithCancel(context.Background()); go func(){}(); go cancel() }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"covered", true}, {"deferred", true}, {"conditional", false}, {"asynchronous", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
			calls := ssaflow.InstructionsOf[*ssa.Call](function)
			cancel := ssacall.CallResult(calls[1], 1)
			cutoff := proofs.NewSearchBudget(0)
			if cancelCoversSpawn(spawn, cancel, heapmodel.NewStorage(cutoff)) || !cutoff.Exhausted() {
				t.Fatal("unavailable coverage must not prove cancellation")
			}
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := cancelCoversSpawn(spawn, cancel, heapmodel.NewStorage(fresh)); got != test.want || fresh.Exhausted() {
				t.Fatalf("fresh coverage=%v exhausted=%v, want %v", got, fresh.Exhausted(), test.want)
			}
		})
	}
}
