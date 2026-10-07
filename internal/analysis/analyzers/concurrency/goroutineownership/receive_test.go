package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReceiveSearchDistinctBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivetest", `
package receivetest
func receive(first, second <-chan int) { <-second }
func worker(target, other <-chan int) {
 receive(target, other)
 receive(other, target)
}

`)
	function := pkg.Func("worker")
	if !receivesAnywhere(function, function.Params[0], proofs.NewSearchBudget(1000)) {
		t.Fatal("second visit to receive must inspect the distinct formal binding")
	}
}

func TestWorkerReceiveSearchBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivetest", `
package receivetest
func leaf(ch <-chan int) { <-ch }
func ignore(ch <-chan int) {}
func left(ch <-chan int) { ignore(ch) }
func right(ch <-chan int) { leaf(ch) }
func diamond(ch <-chan int) { left(ch); right(ch) }
func captured(ch <-chan int) { func() { <-ch }() }
func asynchronous(ch <-chan int) { go leaf(ch) }
func unrelated(ch, other <-chan int) { <-other }
func recursive(ch <-chan int) { recursive(ch) }
func unavailable(ch <-chan int)
func opaque(ch <-chan int) { unavailable(ch) }
`)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"diamond", proofs.EvidenceProven},
		// A spilled parameter's capture cell is not the parameter value.
		// The shared traversal preserves that opaque mapping boundary.
		{"captured", proofs.EvidenceDisproven},
		// The search preserves possible consumption by nested workers.
		// That supplies uncertainty about ownership, never a join.
		{"asynchronous", proofs.EvidenceProven},
		{"unrelated", proofs.EvidenceDisproven},
		{"ignore", proofs.EvidenceDisproven},
		{"recursive", proofs.EvidenceUnknown},
		{"opaque", proofs.EvidenceUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			search := newWorkerReceiveSearch(proofs.NewSearchBudget(1000), func(_ *ssa.Function, local, channel ssa.Value) bool {
				return heapmodel.ValueDerivesFrom(channel, local)
			})
			if proof := search.prove(function, function.Params[0]); proof.State != test.want {
				t.Fatalf("receive proof = %+v, want state %v", proof, test.want)
			}
		})
	}
}

func TestWorkerReceiveSearchBudgetDoesNotPoisonMemo(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivetest", `
package receivetest
func leaf(ch <-chan int) { <-ch }
func forward(ch <-chan int) { leaf(ch) }
`)
	function := pkg.Func("forward")
	search := newWorkerReceiveSearch(proofs.NewSearchBudget(1), func(_ *ssa.Function, local, channel ssa.Value) bool {
		return heapmodel.ValueDerivesFrom(channel, local)
	})
	if proof := search.prove(function, function.Params[0]); proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("exhausted receive proof = %+v", proof)
	}
	search.budget = proofs.NewSearchBudget(1000)
	if proof := search.prove(function, function.Params[0]); !proof.Proven() {
		t.Fatalf("fresh budget reused incomplete answer: %+v", proof)
	}
}

func TestWorkerReceiveBudgetSuppressesDiagnostic(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivetest", `
package receivetest
func leaf(ch <-chan int) { <-ch }
func subject(ch <-chan int) { go func() { leaf(ch) }() }
`)
	function := pkg.Func("subject")
	analysis := &spawnAnalysis{
		function: function, spawn: ssaflow.InstructionsOf[*ssa.Go](function)[0],
		pool: proofs.NewSearchBudget(1),
	}
	proof, decided := analysis.lifecycleProof()
	if !decided || proof.Outcome != GoroutineUnknown || proof.Reason != reasonReceiveBudgetExhausted {
		t.Fatalf("receive budget candidate proof = %+v, decided=%v", proof, decided)
	}
}

func TestContextFieldReceiveDistinctBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivetest", `
package receivetest
import "context"
type owner struct { ctx context.Context }
func receive(first, second *owner) { <-second.ctx.Done() }
func worker(target, other *owner) { receive(target, other); receive(other, target) }
func unrelated(target, other *owner) { receive(target, other) }
func replaced(target, other *owner) { target.ctx = context.Background(); receive(other, target) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"worker", true},
		{"unrelated", false},
		{"replaced", false},
	} {
		function := pkg.Func(test.name)
		if got := contextFieldReceivedAnywhere(function, function.Params[0], function, proofs.NewSearchBudget(1000)); got != test.want {
			t.Fatalf("%s field receive = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestUnobservedSignalSnapshots(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "signalsnapshot", `package signalsnapshot
 func subject(){var done chan int;before:=done;done=make(chan int);go func(){close(done)}();println(before)}
 `)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	proof := candidate.prove()
	if proof.Outcome != GoroutineUnknown || proof.Reason != reasonUnobservedSignal {
		t.Fatalf("nil snapshot invented a protocol: %+v", proof)
	}
}

func TestUnobservedSignalCensusCutoff(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	source := `package signalcut
 func ignore(done chan int){}
 func finish(done chan int){close(done)}
 func subject(){done:=make(chan int);` + strings.Repeat("ignore(done);", proofs.QueryBudget+1) + `go finish(done)}
 `
	pkg := ssaflowtest.BuildPackage(t, "signalcut", source)
	fn := pkg.Func("subject")
	pass := &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}
	candidate := newSpawnAnalysis(pass, fn, ssaflow.InstructionsOf[*ssa.Go](fn)[0])
	child := candidate.pool.Within(proofs.QueryBudget)
	got := candidate.proveUnobservedSignalsWithin(child)
	if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || candidate.pool.Exhausted() {
		t.Fatalf("cut=%+v", got)
	}
	cutoff := candidate.lifetimeCutoff(child, querySignalCensus, reasonSignalCensusUnavailable)
	if cutoff.Outcome != GoroutineUnknown || cutoff.Reason != reasonSignalCensusUnavailable {
		t.Fatalf("signal cutoff projection: %+v", cutoff)
	}
	fresh := candidate.proveUnobservedSignalsWithin(candidate.pool.Within(2 * proofs.SummaryBudget))
	if !fresh.Proven() {
		t.Fatalf("fresh=%+v", fresh)
	}
	// The bounded pre-spawn census now encounters this huge prefix first;
	// the direct signal query above still pins its own cutoff and recovery.
	if proof := candidate.prove(); proof.Outcome != GoroutineUnknown || proof.Reason != reasonPreSpawnCensusCutoff {
		t.Fatalf("cutoff revived diagnostic: %+v", proof)
	}
	requireCensusCutoffTrace(t, path, "signal-census")
}

func requireCensusCutoffTrace(t *testing.T, path, label string) {
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
		if event.Phase == "evidence" && event.Reason == "budget-exhausted" && event.Candidate != "" && event.Details["phase"] == label {
			return
		}
	}
	t.Fatalf("%s cutoff has no attributed evidence event", label)
}
