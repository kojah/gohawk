package processownership

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

func TestCommandUseAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "commanduse", `package commanduse
 import "os/exec"
 func consume(*exec.Cmd){}
 func direct(cmd *exec.Cmd){cmd.Start();consume(cmd)}
 func unused(cmd *exec.Cmd){cmd.Start()}
 func errorOnly(cmd *exec.Cmd)error{return cmd.Start()}
 func pidOnly(cmd *exec.Cmd){cmd.Start();println(cmd.Process.Pid)}
 func captured(cmd *exec.Cmd){cmd.Start();go func(){consume(cmd)}()}
 func aggregate(cmd *exec.Cmd){cmd.Start();x:=struct{cmd *exec.Cmd}{cmd};println(&x)}
 func backedge(cmd *exec.Cmd,flag bool){for{consume(cmd);if flag{cmd.Start();continue};return}}
 `)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"direct", ssaflow.EvidenceProven},
		{"unused", ssaflow.EvidenceDisproven},
		{"errorOnly", ssaflow.EvidenceDisproven},
		{"pidOnly", ssaflow.EvidenceDisproven},
		{"captured", ssaflow.EvidenceProven},
		{"aggregate", ssaflow.EvidenceProven},
		{"backedge", ssaflow.EvidenceProven},
	} {
		fn := pkg.Func(test.name)
		start := startupTestCall(t, fn)
		cmd := ssaflow.CallReceiver(start.Common())
		checkProcessQuery(t, test.name, test.want, func(budget *ssaflow.SearchBudget) ssaflow.Proof { return proveCommandUseAfterStart(start, cmd, budget) })
	}
}

func TestReturnedProcessOwnerAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnedhandle", `package returnedhandle
 import("os";"os/exec")
 type owner struct{process *os.Process}
 func held(cmd *exec.Cmd)*owner{cmd.Start();return &owner{cmd.Process}}
 func pid(cmd *exec.Cmd)int{cmd.Start();return cmd.Process.Pid}
 func unrelated(cmd *exec.Cmd)*owner{cmd.Start();return &owner{}}
 `)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"held", ssaflow.EvidenceProven}, {"pid", ssaflow.EvidenceDisproven}, {"unrelated", ssaflow.EvidenceDisproven},
	} {
		fn := pkg.Func(test.name)
		start := startupTestCall(t, fn)
		cmd := ssaflow.CallReceiver(start.Common())
		returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
		checkProcessQuery(t, test.name, test.want, func(budget *ssaflow.SearchBudget) ssaflow.Proof {
			return proveReturnedProcessOwner(returned, cmd, budget)
		})
	}
}

func TestCommandUseCutoffDecisionTrace(t *testing.T) {
	path := processTraceFile(t)
	pkg := ssaflowtest.BuildPackage(t, "usecut", `package usecut
 import "os/exec"
 func consume(*exec.Cmd){}
 func subject(cmd *exec.Cmd){cmd.Start();consume(cmd)}
 `)
	fn := pkg.Func("subject")
	start := startupTestCall(t, fn)
	cmd := ssaflow.CallReceiver(start.Common())
	witness := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	decision := decideProcessReturn(start, cmd, witness, false, ssaflow.NewSearchBudget(0))
	if decision.state != ssaflow.EvidenceUnknown || decision.reason != reasonCommandUseCutoff {
		t.Fatalf("cutoff: %+v", decision)
	}
	emitProcessDecision(&analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}, fn, start, cmd, decision)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event processTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		cutoffDecision := event.Phase == "decision" && event.Reason == "command-use-budget-exhausted" && event.Outcome == "unknown"
		if cutoffDecision && strings.Contains(event.Candidate, "usecut.go:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing attributed cutoff decision: %s", data)
	}
	fresh := decideProcessReturn(start, cmd, witness, false, ssaflow.NewSearchBudget(processQueryBudget))
	if fresh.state != ssaflow.EvidenceProven || fresh.reason != reasonUnownedReturn {
		t.Fatalf("fresh: %+v", fresh)
	}
}

func TestProcessHandleArgumentAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "handleargs", `package handleargs
 import "os/exec"
 func subject(cmd *exec.Cmd){cmd.Start();println(1,2,3,4)}
 `)
	fn := pkg.Func("subject")
	start := startupTestCall(t, fn)
	cmd := ssaflow.CallReceiver(start.Common())
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) != "println" {
			continue
		}
		proof := &commandProof{pool: ssaflow.NewSearchBudget(0)}
		if got := processHandleOwnershipAction(proof, call, cmd); got != ssaflow.EvidenceUnknown {
			t.Fatalf("cutoff: %v", got)
		}
		proof.pool = ssaflow.NewSearchBudget(processPoolBudget)
		if got := processHandleOwnershipAction(proof, call, cmd); got != ssaflow.EvidenceDisproven {
			t.Fatalf("fresh: %v", got)
		}
		return
	}
	t.Fatal("missing compiled argument consumer")
}
