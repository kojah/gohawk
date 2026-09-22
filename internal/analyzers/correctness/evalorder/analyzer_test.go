package evalorder

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/passes/testvariant"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), testvariant.IncludeProductionFiles(Analyzer()), "evalorder")
}

func TestMapSnapshotTrace(t *testing.T) {
	flags := flag.NewFlagSet("evalorder-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Cleanup(func() {
		for name, value := range map[string]string{"gohawk-trace": "none", "gohawk-trace-file": os.DevNull} {
			if err := flags.Set(name, value); err != nil {
				t.Error(err)
			}
		}
	})
	for name, value := range map[string]string{"gohawk-trace": "evalorder", "gohawk-trace-file": path} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), testvariant.IncludeProductionFiles(Analyzer()), "evalorder")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "map-header-mutation-unknown" {
			found = true
			if event.Outcome != "unknown" || !strings.Contains(event.Candidate, "map_snapshots.go:") {
				t.Errorf("map snapshot was not explicit uncertainty: %+v", event)
			}
		}
	}
	if !found {
		t.Error("missing map snapshot uncertainty evidence")
	}
}
