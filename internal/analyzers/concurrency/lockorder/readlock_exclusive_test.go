package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReadLockWritesPrivateOwners(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "privateread")
}
