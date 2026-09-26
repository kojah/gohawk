package cli

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

// The budget view shows where the analysis spent its time and where it gave
// up. A search that runs out of budget answers conservatively, which can hide
// a real defect, and nothing else reports it. Each exhaustion is filed under
// the analyzer run it happened in and the code that asked the question; the
// budget does not know which analyzed function the question was about, so
// the trace view, not this one, locates a give-up in a function.

func printBudget(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("budget", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	analyzerList := flags.String("analyzer", "", "comma-separated analyzers to run (default all)")
	top := flags.Int("top", 15, "print the N slowest analyzer runs")
	withDependencies := flags.Bool("deps", false, "also list exhaustions while summarizing dependencies")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk dump budget [-analyzer NAMES] [-top N] [-deps] [-tests] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	analyzers, err := catalogAnalyzers(*analyzerList)
	if err != nil {
		return err
	}
	exhaustions := map[exhaustionKey]int{}
	var running exhaustionKey
	restore := labelRuns(analyzers, func(analyzer, pkg string) { running.analyzer, running.pkg = analyzer, pkg })
	stop := ssaflow.RecordExhaustions(func(exhaustion ssaflow.Exhaustion) {
		running.Exhaustion = exhaustion
		exhaustions[running]++
	})
	graph, err := analyzeForDump(flags.Args(), *includeTests, analyzers)
	stop()
	restore()
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	roots := map[string]bool{}
	for _, action := range graph.Roots {
		roots[action.Package.PkgPath] = true
	}
	writeRunTimes(&buffer, graph, roots, *top)
	writeExhaustions(&buffer, exhaustions, roots, *withDependencies)
	_, err = output.Write(buffer.Bytes())
	return err
}

type exhaustionKey struct {
	analyzer, pkg string
	ssaflow.Exhaustion
}

// labelRuns wraps the Run of every analyzer the selection needs, passes
// included, to name the run in progress, and returns a function restoring
// them. The dump runs one action at a time, so the label is always the run
// an exhaustion happened in. The analyzers are process-wide values; this is
// safe only in a process that runs nothing else meanwhile, as a dump does.
func labelRuns(analyzers []*analysis.Analyzer, label func(analyzer, pkg string)) (restore func()) {
	original := map[*analysis.Analyzer]func(*analysis.Pass) (any, error){}
	var wrap func(*analysis.Analyzer)
	wrap = func(analyzer *analysis.Analyzer) {
		if _, done := original[analyzer]; done {
			return
		}
		run := analyzer.Run
		original[analyzer] = run
		analyzer.Run = func(pass *analysis.Pass) (any, error) {
			label(analyzer.Name, pass.Pkg.Path())
			return run(pass)
		}
		for _, required := range analyzer.Requires {
			wrap(required)
		}
	}
	for _, analyzer := range analyzers {
		wrap(analyzer)
	}
	return func() {
		for analyzer, run := range original {
			analyzer.Run = run
		}
	}
}

// writeRunTimes prints the slowest runs over the requested packages, then
// the time each analyzer spent on their dependencies, where the fact passes
// summarize every imported package.
func writeRunTimes(buffer *bytes.Buffer, graph *checker.Graph, roots map[string]bool, top int) {
	var runs []*checker.Action
	dependencies := map[string]time.Duration{}
	dependencyPackages := map[string]int{}
	for action := range graph.All() {
		if roots[action.Package.PkgPath] {
			runs = append(runs, action)
			continue
		}
		dependencies[action.Analyzer.Name] += action.Duration
		dependencyPackages[action.Analyzer.Name]++
	}
	slices.SortFunc(runs, func(left, right *checker.Action) int {
		return cmp.Or(cmp.Compare(right.Duration, left.Duration), cmp.Compare(left.String(), right.String()))
	})
	buffer.WriteString("slowest runs:\n")
	for _, action := range runs[:min(top, len(runs))] {
		fmt.Fprintf(buffer, "  %9s  %s on %s\n", action.Duration.Round(time.Millisecond), action.Analyzer.Name, action.Package.PkgPath)
	}
	if len(dependencies) != 0 {
		buffer.WriteString("dependencies:\n")
		names := slices.SortedFunc(maps.Keys(dependencies), func(left, right string) int {
			return cmp.Or(cmp.Compare(dependencies[right], dependencies[left]), cmp.Compare(left, right))
		})
		for _, name := range names {
			if dependencies[name] < time.Millisecond {
				continue
			}
			fmt.Fprintf(buffer, "  %9s  %s over %d packages\n", dependencies[name].Round(time.Millisecond), name, dependencyPackages[name])
		}
	}
}

// writeExhaustions lists the searches that ran out over the requested
// packages, and over dependencies only when asked: a standard library
// summary cut short is common and rarely the question.
func writeExhaustions(buffer *bytes.Buffer, exhaustions map[exhaustionKey]int, roots map[string]bool, withDependencies bool) {
	hidden := 0
	for key, count := range exhaustions {
		if !withDependencies && !roots[key.pkg] {
			hidden += count
			delete(exhaustions, key)
		}
	}
	if len(exhaustions) == 0 {
		buffer.WriteString("budgets: no search ran out\n")
	} else {
		buffer.WriteString("budgets exhausted:\n")
	}
	keys := slices.SortedFunc(maps.Keys(exhaustions), func(left, right exhaustionKey) int {
		return cmp.Or(cmp.Compare(left.pkg, right.pkg), cmp.Compare(left.analyzer, right.analyzer), cmp.Compare(exhaustions[right], exhaustions[left]),
			cmp.Compare(left.Site, right.Site), cmp.Compare(left.Limit, right.Limit), boolOrder(left.Pool, right.Pool))
	})
	run := ""
	for _, key := range keys {
		if label := key.analyzer + " on " + key.pkg; label != run {
			run = label
			fmt.Fprintf(buffer, "  %s\n", run)
		}
		cause := fmt.Sprintf("limit %d", key.Limit)
		if key.Pool {
			cause = fmt.Sprintf("pool ran out (own limit %d)", key.Limit)
		}
		fmt.Fprintf(buffer, "    %5d× %s, %s\n", exhaustions[key], key.Site, cause)
	}
	if hidden != 0 {
		fmt.Fprintf(buffer, "%d more while summarizing dependencies; -deps lists them\n", hidden)
	}
}

func boolOrder(left, right bool) int {
	switch {
	case left == right:
		return 0
	case left:
		return 1
	}
	return -1
}
