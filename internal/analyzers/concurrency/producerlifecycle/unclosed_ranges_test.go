package producerlifecycle

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

// Each accepted range must stay quiet for the reason its fixture documents,
// not by accident of an earlier link.
func TestUnclosedRangeReasons(t *testing.T) {
	flags := flag.NewFlagSet("unclosed-trace", flag.ContinueOnError)
	trace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	for name, value := range map[string]string{"gohawk-trace": "producerlifecycle", "gohawk-trace-file": path} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := flags.Set("gohawk-trace", "none"); err != nil {
			t.Error(err)
		}
	})
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "unclosedranges")
	want := map[string]string{
		"consumeDeferred":    "close-deferred",
		"consumeEveryReturn": "closed-on-every-return",
		"consumeSequential":  "closer-not-concurrent-with-range",
		"consumeRetried":     "closer-called-in-loop",
		"consumeTwoClosers":  "several-closing-functions",
		"consumeStepping":    "unclosed-return-error-unproven",
		"consumeBreaking":    "range-has-other-exit",
		"consumeEscaping":    "owned-channel-unknown",
		"consumeSupplied":    "owned-channel-unknown",
		"consumeOther":       "closer-not-concurrent-with-range",
		"consumePanicking":   "closed-on-every-return",
		"consumePolling":     "unclosed-return-error-unproven",
		"consumeProducer":    "range-waits-on-failed-closer",
		"consumeChecked":     "range-waits-on-failed-closer",
		"consumeStreaming":   "range-waits-on-failed-closer",
	}
	got := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Check     string `json:"check"`
			Phase     string `json:"phase"`
			Reason    string `json:"reason"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Check != "producerlifecycle/unclosed-range" || event.Phase != "decision" {
			continue
		}
		got[enclosingFunction(t, event.Candidate)] = event.Reason
	}
	for function, reason := range want {
		if got[function] != reason {
			t.Errorf("%s: decision %q, want %q", function, got[function], reason)
		}
	}
	// An exported field of a library type is not owned, so it is no candidate.
	if reason, ok := got["consumeExported"]; ok {
		t.Errorf("consumeExported: decision %q, want no candidate", reason)
	}
}

// enclosingFunction names the function declared last before a position.
func enclosingFunction(t *testing.T, position string) string {
	t.Helper()
	parts := strings.Split(position, ":")
	line, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(strings.Join(parts[:len(parts)-2], ":"))
	if err != nil {
		t.Fatal(err)
	}
	name := ""
	for index, text := range strings.Split(string(source), "\n") {
		if index >= line {
			break
		}
		if rest, ok := strings.CutPrefix(text, "func "); ok {
			name = strings.Fields(rest)[0]
			name = name[:strings.IndexAny(name+"(", "(")]
		}
	}
	return name
}
