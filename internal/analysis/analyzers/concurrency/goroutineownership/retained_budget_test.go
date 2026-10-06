package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
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
