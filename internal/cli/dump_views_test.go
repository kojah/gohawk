package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzers/concurrency/lockorder"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

// lockCycleModule takes two package-level mutexes in both orders, the
// smallest package that gives every analysis view something to print
// without loading more than sync.
func lockCycleModule(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/dumpviews\n\ngo 1.25\n",
		"lib.go": `package dumpviews

import "sync"

var first, second sync.Mutex

func forward() {
	first.Lock()
	defer first.Unlock()
	second.Lock()
	second.Unlock()
}

func backward() {
	second.Lock()
	defer second.Unlock()
	first.Lock()
	first.Unlock()
}
`,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(directory)
}

func TestDumpViewsOverALockCycle(t *testing.T) {
	lockCycleModule(t)
	for _, test := range []struct {
		name      string
		print     func([]string, *bytes.Buffer, *bytes.Buffer) error
		arguments []string
		want      []string
		without   []string
	}{
		{"trace", viewPrinter(printTrace), []string{"-analyzer", "lockorder", "-func", "backward", "."}, []string{
			"dumpviews.backward", "lockorder/contradictory-order at lib.go:17:12",
			"evidence   rejected cycle-order-recorded acquired=first", "diagnostic-candidate",
		}, []string{"dumpviews.forward"}},
		{
			"trace decisions", viewPrinter(printTrace),
			[]string{"-analyzer", "lockorder", "-decisions", "."},
			[]string{"diagnostic-candidate"},
			[]string{"evidence "},
		},
		{"locks", viewPrinter(printLocks), []string{"."}, []string{
			"example.com/dumpviews", "first -> second: held at lib.go:8:12, acquired at lib.go:10:13", "cycles reported:", "second -> first -> second",
		}, nil},
		{
			"locks dot", viewPrinter(printLocks),
			[]string{"-dot", "."},
			[]string{`digraph "example.com/dumpviews"`, `"second" -> "first" [label="lib.go:17", color=red]`},
			nil,
		},
		{"budget", viewPrinter(printBudget), []string{"-analyzer", "lockorder", "."}, []string{
			"slowest runs:", "lockorder on example.com/dumpviews", "dependencies:",
		}, nil},
	} {
		var output, errorsOutput bytes.Buffer
		if err := test.print(test.arguments, &output, &errorsOutput); err != nil {
			t.Fatalf("%s: error = %v, stderr %s", test.name, err, errorsOutput.String())
		}
		for _, want := range test.want {
			if !strings.Contains(output.String(), want) {
				t.Errorf("%s: dump lacks %q:\n%s", test.name, want, output.String())
			}
		}
		for _, unwanted := range test.without {
			if strings.Contains(output.String(), unwanted) {
				t.Errorf("%s: dump has %q:\n%s", test.name, unwanted, output.String())
			}
		}
	}
}

func viewPrinter(print func([]string, io.Writer, io.Writer) error) func([]string, *bytes.Buffer, *bytes.Buffer) error {
	return func(arguments []string, output, errorsOutput *bytes.Buffer) error {
		return print(arguments, output, errorsOutput)
	}
}

func TestLabelRunsRestoresEveryRun(t *testing.T) {
	analyzer := lockorder.Analyzer()
	originals := map[*analysis.Analyzer]string{}
	var record func(*analysis.Analyzer)
	record = func(current *analysis.Analyzer) {
		originals[current] = funcIdentity(current.Run)
		for _, required := range current.Requires {
			record(required)
		}
	}
	record(analyzer)
	restore := labelRuns([]*analysis.Analyzer{analyzer}, func(string, string) {})
	if funcIdentity(analyzer.Run) == originals[analyzer] {
		t.Fatal("labelRuns did not wrap the analyzer")
	}
	restore()
	for current, identity := range originals {
		if funcIdentity(current.Run) != identity {
			t.Errorf("%s: Run not restored", current.Name)
		}
	}
}

// funcIdentity names a function value by its code pointer, the only
// comparison Go allows between func values.
func funcIdentity(function func(*analysis.Pass) (any, error)) string {
	return fmt.Sprintf("%p", function)
}

func TestFoldUnansweredKeepsProofsAndLoneQuestions(t *testing.T) {
	no := func(question, reason string, outcome analysisTrace.Outcome) analysisTrace.Record {
		return analysisTrace.Record{
			Phase: "evidence", Outcome: outcome, Reason: reason, Position: "a.go:4:2",
			Details: map[string]string{"question": question, "instruction": "return nil", "target": "t1"},
		}
	}
	proven := no("release", "lifecycle-summary", analysisTrace.OutcomeAccepted)
	steps := []analysisTrace.Record{
		no("transfer", "evidence-not-found", analysisTrace.OutcomeRejected),
		no("release+summary", "evidence-unavailable", analysisTrace.OutcomeUnknown),
		proven,
		no("transfer", "evidence-not-found", analysisTrace.OutcomeRejected),
	}
	folded := foldUnanswered(steps)
	if len(folded) != 3 {
		t.Fatalf("folded into %d steps, want 3: %+v", len(folded), folded)
	}
	if folded[0].title != "unanswered" || folded[0].outcome != analysisTrace.OutcomeUnknown ||
		folded[0].details["answers"] != "transfer:not-found,release+summary:unavailable" || folded[0].details["instruction"] != "return nil" {
		t.Errorf("folded step = %+v", folded[0])
	}
	if folded[1].title != proven.Reason || folded[2].title != "evidence-not-found" {
		t.Errorf("proven answer or lone question was folded: %+v", folded[1:])
	}
}
