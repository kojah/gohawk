package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

// These intentionally remain accepted by the precision-first reporter. The
// proof must still distinguish a possible handoff from exact ownership.
func TestTransferProofStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"mixedReturn":               GoroutineUnknown,
		"overwrittenAggregate":      GoroutineUnknown,
		"discardedWrapper":          GoroutineUnknown,
		"exactStore":                GoroutineLifecycleHonored,
		"mixedStore":                GoroutineUnknown,
		"overwrittenAggregateStore": GoroutineUnknown,
		"exactBesideOpaque":         GoroutineLifecycleHonored,
		"configuredMockResult":      GoroutineUnknown,
	}
	for _, result := range analyzertest.Run(t, analysistest.TestData(), Analyzer(), "transferlabels") {
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
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing transfer proof cases: %v", want)
	}
}
