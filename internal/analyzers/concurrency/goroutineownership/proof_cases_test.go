package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

// assertSpawnProofs checks the authoritative proof as well as the fixture's
// diagnostics. Each case names the first launch: later waiter launches are
// separate obligations, not joins by the launching goroutine itself.
func assertSpawnProofs(t *testing.T, want map[string]GoroutineOutcome, patterns ...string) {
	t.Helper()
	for _, result := range analyzertest.Run(t, analysistest.TestData(), Analyzer(), patterns...) {
		functions, err := ssaflow.SourceSSAFunctions(result.Pass)
		if err != nil {
			t.Fatal(err)
		}
		for _, function := range functions {
			expected, ok := want[function.Name()]
			if !ok {
				continue
			}
			for _, spawn := range ssaflow.InstructionsOf[*ssa.Go](function) {
				analysis := newSpawnAnalysis(result.Pass, function, spawn)
				if proof := analysis.prove(); proof.Outcome != expected {
					t.Errorf("%s: got %+v, want %v", function.Name(), proof, expected)
				}
				delete(want, function.Name())
				break
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing spawn proof cases: %v", want)
	}
}
