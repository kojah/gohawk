package goroutineownership

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestRetainedOwnerCutoffIsPathLocal(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "retained", `package retained
type owner struct{}
func (*owner) Close() {}
func covered() { o:=new(owner); done:=make(chan int); go func(){close(done)}(); o.Close() }
func skipped(skip bool) { o:=new(owner); done:=make(chan int); go func(){close(done)}(); if skip{return}; o.Close() }
`)
	for _, test := range []struct {
		name string
		want ssapath.ObligationOutcome
	}{
		{"covered", ssapath.ObligationUncertain},
		{"skipped", ssapath.ObligationViolated},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
			call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
			pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
			candidate := newSpawnAnalysis(pass, function, spawn)
			candidate.pool = proofs.NewSearchBudget(0).Observed(candidate.probe.Observer())
			action, reason := candidate.callAction(call, call.Common())
			if action != actionUnknown || reason != reasonRetainedOwnerBudgetExhausted {
				t.Fatalf("cutoff label = %v/%v", action, reason)
			}
			// Submit the authoritative label at its own instruction. An early
			// return that bypasses this call receives no unknown credit.
			outcome := ssapath.EvaluateObligation(ssapath.ObligationFlow{
				Start: spawn, Instruction: func(instruction ssa.Instruction) ssapath.ObligationAction {
					if instruction == call {
						return action.obligation()
					}
					return ssapath.ObligationNone
				},
			})
			if outcome != test.want {
				t.Fatalf("flow = %v, want %v", outcome, test.want)
			}
			fresh := newSpawnAnalysis(pass, function, spawn)
			if proof := fresh.closesRetainedWorkerOwner(call, call.Common()); proof.State != proofs.EvidenceDisproven {
				t.Fatalf("unrelated fresh owner must not suppress: %+v", proof)
			}
		})
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
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate != "" &&
			event.Details["phase"] == "retained-owner" {
			observed = true
		}
	}
	if !observed {
		t.Fatal("missing attributed retained-owner cutoff")
	}
}

func TestFactoryCleanupBudgetAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "factory", `package factory
type owner struct{}
func (*owner) Close() {}
func factory() (*owner,func()) { o:=new(owner); return o,func(){o.Close()} }
func subject() { _,cleanup:=factory(); cleanup() }
`)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))
	factory := calls[0]
	cutoff := proofs.NewSearchBudget(0)
	if targets := factoryCleanupTargets(factory, 1, cutoff); len(targets) != 0 || !cutoff.Exhausted() {
		t.Fatalf("cutoff targets=%v exhausted=%v", targets, cutoff.Exhausted())
	}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	if targets := factoryCleanupTargets(factory, 1, fresh); len(targets) == 0 || fresh.Exhausted() {
		t.Fatalf("fresh targets=%v exhausted=%v", targets, fresh.Exhausted())
	}
}

func TestSelectedContextOwnerCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contextowner", `package contextowner
import "context"
func subject(ctx context.Context) { done:=make(chan int); go func(){close(done)}(); <-ctx.Done() }
`)
	function := pkg.Func("subject")
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	done := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, function, spawn)
	candidate.pool = proofs.NewSearchBudget(0)
	proof := candidate.observesOpaqueWorkerContext(done)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("selected context cutoff = %+v", proof)
	}
	fresh := newSpawnAnalysis(pass, function, spawn)
	if proof := fresh.observesOpaqueWorkerContext(done); proof.State != proofs.EvidenceDisproven {
		t.Fatalf("unrelated context must not supply ownership: %+v", proof)
	}
}

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

func TestFactoryOriginAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "origins", `package origins
type channel chan int
type group struct{}
type groupPointer *group
type owner struct{ member group }
func opaqueSignal() channel
func opaqueGroup() *group
func opaqueOwner() *owner
func freshGroup() *group { return new(group) }
func signalDirect() chan int { value:=opaqueSignal(); go func(){}(); return value }
func signalWrapped() chan int { value:=chan int(opaqueSignal()); go func(){}(); return value }
func signalPhi(flag bool) chan int { value:=chan int(opaqueSignal()); if flag{value=make(chan int)}; go func(){}(); return value }
func signalLocal() chan int { value:=make(chan int); go func(){}(); return value }
func groupDirect() *group { value:=opaqueGroup(); go func(){}(); return value }
func groupWrapped() groupPointer { value:=groupPointer(opaqueGroup()); go func(){}(); return value }
func groupPhi(flag bool) *group { value:=opaqueGroup(); if flag{value=new(group)}; go func(){}(); return value }
func groupField() *group { value:=&opaqueOwner().member; go func(){}(); return value }
func groupFresh() *group { value:=freshGroup(); go func(){}(); return value }
func groupLocal() *group { value:=new(group); go func(){}(); return value }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"signalDirect", true},
		{"signalWrapped", true},
		{"signalPhi", true},
		{"signalLocal", false},
		{"groupDirect", true},
		{"groupWrapped", true},
		{"groupPhi", true},
		{"groupField", true},
		{"groupFresh", false},
		{"groupLocal", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertFactoryOriginQuery(t, pkg.Func(test.name), strings.HasPrefix(test.name, "signal"), test.want)
		})
	}
}

func assertFactoryOriginQuery(t *testing.T, function *ssa.Function, signal, want bool) {
	t.Helper()
	var dump bytes.Buffer
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	value := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	query := func(budget *proofs.SearchBudget) proofs.Proof {
		if signal {
			return helperSignalOrigin(value, spawn, budget)
		}
		return opaqueGroupOrigin(value, budget)
	}
	fresh := query(proofs.NewSearchBudget(proofs.QueryBudget))
	if !fresh.Known() || fresh.Proven() != want {
		t.Fatalf("fresh origin = %+v, want proven=%v", fresh, want)
	}
	for _, limit := range []int{0, 1} {
		budget := proofs.NewSearchBudget(limit)
		proof := query(budget)
		if proof.Known() || !budget.Exhausted() {
			t.Fatalf("limit %d must stop before completing fold and storage leaf: %+v", limit, proof)
		}
	}
	for limit := range 64 {
		budget := proofs.NewSearchBudget(limit)
		proof := query(budget)
		if budget.Exhausted() && (proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted) {
			t.Fatalf("limit %d returned completed proof after cutoff: %+v", limit, proof)
		}
		if proof.Known() && proof.Proven() != want {
			t.Fatalf("limit %d changed origin policy: %+v", limit, proof)
		}
	}
}

func TestCancellationSummaryIsNotJoin(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "canceljoin", `package canceljoin
import "context"
func stop(cancel context.CancelFunc) { cancel() }
func root() {
 _, cancel := context.WithCancel(context.Background())
 done := make(chan struct{})
 go func() { defer close(done) }()
 stop(cancel)
}
`)
	root := pkg.Func("root")
	channels := ssaflow.InstructionsOf[*ssa.MakeChan](root)
	if len(channels) != 1 {
		t.Fatal("missing completion channel")
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](root) {
		if call.Common().StaticCallee() != pkg.Func("stop") {
			continue
		}
		engine := concurrencyfacts.NewEngine()
		summary := engine.AtCall(call, proofs.NewSearchBudget(2000))
		if !summary.Complete() || len(summary.Operations) != 1 || summary.Operations[0].Kind != concurrencyfacts.Cancel {
			t.Fatalf("cancellation not modeled: %+v", summary)
		}
		proof := proveSummaryJoin(engine, call, channels[0], trackedSignal, proofs.NewSearchBudget(2000))
		if proof.joined || proof.reason != summaryJoinConcurrencySummaryNoExactJoin {
			t.Errorf("cancel became join: %+v", proof)
		}
		return
	}
	t.Fatal("missing cancellation call")
}
