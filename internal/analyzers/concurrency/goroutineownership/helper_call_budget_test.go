package goroutineownership

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestHelperCallSharesCandidateAllowance(t *testing.T) {
	tracePath := enableSummaryJoinTrace(t)
	pkg := ssaflowtest.BuildPackage(t, "helpercall", "package helpercall; func tick()int{return 1}; func slow(ch chan int)int{n:=0;"+
		strings.Repeat("n+=tick();", 32)+"<-ch;return n}; func subject(){done:=make(chan int);go func(){done<-1}();slow(done)}")
	function := pkg.Func("subject")
	worker := pkg.Func("slow")
	var dump bytes.Buffer
	if _, err := worker.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	target := ssaflow.InstructionsOf[*ssa.MakeChan](function)[0]
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	probe := analysisTrace.For(pass, "goroutineownership", string(check.GoroutineJoin), spawn.Pos())
	query := func(limit int) helperCallProof {
		candidate := &spawnAnalysis{
			pass: pass, function: function, spawn: spawn, probe: probe,
			pool: ssaflow.NewSearchBudget(limit).Observed(probe.Observer()),
		}
		return candidate.helperAction(call.Common(), worker, nil, []trackedValue{{value: target, kind: trackedSignal}})
	}
	// This allowance cannot cover the helper's body. A standalone recursive
	// search must not lend a completed join to this interrupted caller query.
	for _, limit := range []int{0, 1, 32, 64} {
		if proof := query(limit); proof.action != actionUnknown || proof.reason != reasonHelperCallBudgetExhausted {
			t.Fatalf("limit %d returned %+v; want attributed unknown", limit, proof)
		}
	}
	if fresh := query(spawnPoolBudget); fresh.action != actionJoin || fresh.reason != reasonLabelHelper {
		t.Fatalf("fresh helper query = %+v; want exact join", fresh)
	}
	assertHelperCallCutoffTrace(t, tracePath, pass.Fset.Position(spawn.Pos()).String())
}

func assertHelperCallCutoffTrace(t *testing.T, path, candidate string) {
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
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate == candidate &&
			event.Details["phase"] == "helper-call" {
			return
		}
	}
	t.Fatal("missing attributed helper-call cutoff")
}

func TestHelperCallMemoKeepsFormalBinding(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpercall", `package helpercall
func second(first, later chan int) { <-later }
func subject() { done:=make(chan int); go func(){done<-1}(); second(done,done) }
`)
	function := pkg.Func("subject")
	worker := pkg.Func("second")
	call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
	target := ssaflow.InstructionsOf[*ssa.MakeChan](function)[0]
	candidate := &spawnAnalysis{
		function: function, spawn: ssaflow.InstructionsOf[*ssa.Go](function)[0],
		pool: ssaflow.NewSearchBudget(spawnPoolBudget),
	}
	// The first formal has no effect. Its completed answer must not hide the
	// second formal's exact receive when one memo serves both bindings.
	proof := candidate.helperAction(call.Common(), worker, nil, []trackedValue{{value: target, kind: trackedSignal}})
	if proof.action != actionJoin || proof.reason != reasonLabelHelper {
		t.Fatalf("distinct formal binding = %+v; want exact join", proof)
	}
}
