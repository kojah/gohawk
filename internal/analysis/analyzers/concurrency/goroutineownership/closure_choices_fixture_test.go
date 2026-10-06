package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestClosureChoiceFixtures(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "closurechoices")
}
