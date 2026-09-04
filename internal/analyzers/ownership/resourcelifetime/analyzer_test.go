package resourcelifetime

import (
	"testing"
	"time"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "resourcelifetime", "resourcelifetime/useafter")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	for name, value := range map[string]string{"contracts": "http,compress", "require-reader-close": "false"} {
		if err := analyzer.Flags.Set(name, value); err != nil {
			t.Fatalf("set %s=%s: %v", name, value, err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "resourcelifetime/config")
}

// boundedSearchDeadline is generous: the bounded analysis of the fixture below
// takes about a third of a second, and the unbounded search did not finish in
// sixty. Anything between the two means the budget stopped applying.
const boundedSearchDeadline = 15 * time.Second

// TestRecursiveReleaseSearchStaysBounded fails if the release search is once
// again unbounded on mutually recursive callees. It is a cost test, so it
// asserts elapsed time rather than diagnostics; the fixture expects none.
func TestRecursiveReleaseSearchStaysBounded(t *testing.T) {
	start := time.Now()
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivecleanup")
	if elapsed := time.Since(start); elapsed > boundedSearchDeadline {
		t.Errorf("analyzing mutually recursive callees took %s, want under %s; the release search is not bounded",
			elapsed, boundedSearchDeadline)
	}
}
