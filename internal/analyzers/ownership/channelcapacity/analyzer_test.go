package channelcapacity

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "channelcapacity")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	if err := analyzer.Flags.Set("max-unexplained-capacity", "10"); err != nil {
		t.Fatal(err)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelcapacity/config")
}
