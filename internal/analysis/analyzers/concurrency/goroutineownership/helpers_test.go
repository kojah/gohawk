package goroutineownership

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/reporting/check"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestHelperSummaryKeepsTargetAndKind(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(first, second chan struct{}) { <-first }
func forward(first, second chan struct{}) { receive(first, second) }
`)
	search := newHelperSearch()
	for _, name := range []string{"forward", "receive"} {
		function := pkg.Func(name)
		for _, test := range []struct {
			parameter int
			kind      trackedKind
			want      ownershipAction
		}{
			{0, trackedSignal, actionJoin},
			{1, trackedSignal, actionNone},
			{0, trackedOwner, actionNone},
			{0, trackedSignal, actionJoin},
		} {
			if got := search.use(function, function.Params[test.parameter], test.kind); got != test.want {
				t.Errorf("%s parameter %d kind %d: got %v, want %v", name, test.parameter, test.kind, got, test.want)
			}
		}
	}
}

func TestHelperRecursiveCutDoesNotPoisonRetry(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(ch chan struct{}) { <-ch }
`)
	function := pkg.Func("receive")
	search := newHelperSearch()
	var shortened ownershipAction
	if !search.memo.WithFunction(function, func() {
		shortened = search.use(function, function.Params[0], trackedSignal)
	}) {
		t.Fatal("could not enter helper")
	}
	if shortened != actionUnknown {
		t.Errorf("recursive cut = %v, want unknown", shortened)
	}
	if got := search.use(function, function.Params[0], trackedSignal); got != actionJoin {
		t.Errorf("fresh call path = %v, want join; recursive answer must not be cached", got)
	}
}

func TestHelperSummaryBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helpers", `package helpers
func receive(ch chan struct{}) { <-ch }
func forward(ch chan struct{}) { receive(ch) }
func ignore(ch chan struct{}) {}
func recursive(ch chan struct{}) { recursive(ch); receive(ch) }
`)
	for _, test := range []struct {
		name  string
		limit int
		want  ownershipAction
	}{
		{"forward", 5, actionJoin}, // Enters receive, then runs out before its return.
		{"ignore", 1, actionNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			search := newHelperSearch()
			search.budget = proofs.NewSearchBudget(test.limit)
			if got := search.use(function, function.Params[0], trackedSignal); got != actionUnknown {
				t.Fatalf("shortened answer = %v, want unknown (neither join nor absence)", got)
			}
			search.budget = proofs.NewSearchBudget(helperUseBudget)
			if got := search.use(function, function.Params[0], trackedSignal); got != test.want {
				t.Errorf("fresh budget = %v, want %v; incomplete answer must not be cached", got, test.want)
			}
		})
	}
	function := pkg.Func("recursive")
	search := newHelperSearch()
	search.budget = proofs.NewSearchBudget(1)
	if got := search.use(function, function.Params[0], trackedSignal); got != actionUnknown || !search.budget.Exhausted() {
		t.Errorf("recursive query = %v, exhausted=%v", got, search.budget.Exhausted())
	}
}

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
			pool: proofs.NewSearchBudget(limit).Observed(probe.Observer()),
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
	assertCandidateCutoffTrace(t, tracePath, pass.Fset.Position(spawn.Pos()).String(), func(event followupTraceEvent) bool {
		return event.Details["phase"] == "helper-call"
	})
}

func assertCandidateCutoffTrace(t *testing.T, path, candidate string, matches func(followupTraceEvent) bool) {
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
			matches(event) {
			return
		}
	}
	t.Fatal("missing attributed candidate cutoff")
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
		pool: proofs.NewSearchBudget(spawnPoolBudget),
	}
	// The first formal has no effect. Its completed answer must not hide the
	// second formal's exact receive when one memo serves both bindings.
	proof := candidate.helperAction(call.Common(), worker, nil, []trackedValue{{value: target, kind: trackedSignal}})
	if proof.action != actionJoin || proof.reason != reasonLabelHelper {
		t.Fatalf("distinct formal binding = %+v; want exact join", proof)
	}
}
