package lockorder

import (
	"testing"
	"time"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "lockorder")
}

// boundedSearchDeadline is generous: the bounded analysis of the fixture below
// takes about a tenth of a second, and the unbounded search took over
// forty-five. Anything between the two means the budget stopped applying.
const boundedSearchDeadline = 15 * time.Second

// TestRecursiveCalleeSearchStaysBounded fails if the release search is once
// again unbounded on mutually recursive callees. It is a cost test, so it
// asserts elapsed time rather than diagnostics; the fixture expects none.
func TestRecursiveCalleeSearchStaysBounded(t *testing.T) {
	start := time.Now()
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "lockrecursion")
	if elapsed := time.Since(start); elapsed > boundedSearchDeadline {
		t.Errorf("analyzing mutually recursive callees took %s, want under %s; the completion search is not bounded",
			elapsed, boundedSearchDeadline)
	}
}
