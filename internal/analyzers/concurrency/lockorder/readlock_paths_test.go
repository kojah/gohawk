package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReadLockWritesConvergingPaths(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "readlockpaths")
}
