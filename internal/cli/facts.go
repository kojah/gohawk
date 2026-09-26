package cli

import (
	"bytes"
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/passes/resultfacts"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/ssa"
)

// The facts subcommand prints the facts a package exports and the imported
// facts its calls resolve to, as the analyzers would see them: lifecycle
// summaries, result guarantees and cases, and concurrency effects.
// Enumeration is generic over fact families; each family renders its own
// facts through DescribeFact, and a family without a renderer falls back to
// its String. A summarized function always carries a fact, even an empty one,
// so a function absent from the dump was never summarized and is unknown to
// consumers rather than proven harmless.

// factDescriber is implemented by fact types that can decode themselves for
// the object they are attached to.
type factDescriber interface {
	DescribeFact(object types.Object) []string
}

// heapDescriber is implemented by a fact that carries a heap projection it
// renders separately from its claims.
type heapDescriber interface {
	DescribeHeap(object types.Object) []string
}

// Fact kinds select what the dump prints: each fact family by name, and the
// heap projection lifecycle summaries carry, on its own.
const (
	kindLifecycle   = "lifecycle"
	kindHeap        = "heap"
	kindResult      = "result"
	kindConcurrency = "concurrency"
)

var factKinds = []string{kindLifecycle, kindHeap, kindResult, kindConcurrency}

// parseFactKinds reads a comma-separated list of fact kinds; empty selects
// every kind.
func parseFactKinds(list string) (map[string]bool, error) {
	selected := map[string]bool{}
	if strings.TrimSpace(list) == "" {
		for _, kind := range factKinds {
			selected[kind] = true
		}
		return selected, nil
	}
	for kind := range strings.SplitSeq(list, ",") {
		kind = strings.TrimSpace(kind)
		if !slices.Contains(factKinds, kind) {
			return nil, fmt.Errorf("unknown fact kind %q; choose from %s", kind, strings.Join(factKinds, ", "))
		}
		selected[kind] = true
	}
	return selected, nil
}

// actionKind names the fact kind an analyzer publishes.
func actionKind(analyzer *analysis.Analyzer) string {
	switch analyzer {
	case lifecyclefacts.Analyzer:
		return kindLifecycle
	case resultfacts.Analyzer:
		return kindResult
	case concurrencyfacts.Analyzer:
		return kindConcurrency
	}
	return analyzer.Name
}

func printFacts(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("facts", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	nameFilter := flags.String("func", "", "print only facts attached to the function with this name")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	kindList := flags.String("kind", "", "comma-separated fact kinds to print: "+strings.Join(factKinds, ", ")+" (default all)")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk dump facts [-func NAME] [-kind KINDS] [-tests] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	kinds, err := parseFactKinds(*kindList)
	if err != nil {
		return err
	}
	graph, err := analyzeForDump(flags.Args(), *includeTests, factAnalyzers(kinds))
	if err != nil {
		return err
	}
	// Every family lists the callees the lifecycle pass resolved for the
	// same package, so each dump shows the same imported functions.
	referenced := map[*types.Package]map[types.Object]bool{}
	for _, action := range graph.Roots {
		if action.Analyzer == lifecyclefacts.Analyzer {
			referenced[action.Package.Types] = referencedCallees(action)
		}
	}
	var buffer bytes.Buffer
	for _, action := range graph.Roots {
		if action.Err != nil {
			return action.Err
		}
		writeObjectFacts(&buffer, action, *nameFilter, referenced[action.Package.Types], kinds)
	}
	if buffer.Len() == 0 {
		return fmt.Errorf("no fact matched %q", *nameFilter)
	}
	_, err = output.Write(buffer.Bytes())
	return err
}

// factAnalyzers returns the passes the selected kinds need, and every catalog
// analyzer that exports facts of its own. The lifecycle pass always runs: it
// resolves the callees every family lists.
func factAnalyzers(kinds map[string]bool) []*analysis.Analyzer {
	analyzers := []*analysis.Analyzer{lifecyclefacts.Analyzer}
	if kinds[kindResult] {
		analyzers = append(analyzers, resultfacts.Analyzer)
	}
	if kinds[kindConcurrency] {
		analyzers = append(analyzers, concurrencyfacts.Analyzer)
	}
	for _, analyzer := range gohawk.Analyzers() {
		if len(analyzer.FactTypes) > 0 {
			analyzers = append(analyzers, analyzer)
		}
	}
	return analyzers
}

func writeObjectFacts(buffer *bytes.Buffer, action *checker.Action, filter string, referenced map[types.Object]bool, kinds map[string]bool) {
	facts := action.AllObjectFacts()
	// Positions are compared by file and offset, not by token.Pos: the
	// fileset's bases depend on the order files were parsed in, which is
	// parallel, and the dump must read the same on every run.
	slices.SortFunc(facts, func(left, right analysis.ObjectFact) int {
		if order := comparePositions(action, left.Object.Pos(), right.Object.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.Object.Name(), right.Object.Name())
	})
	listed := map[types.Object]bool{}
	for _, fact := range facts {
		if filter != "" && fact.Object.Name() != filter {
			continue
		}
		origin := "exported here"
		if fact.Object.Pkg() != action.Package.Types {
			// Facts of the whole dependency closure are visible; print only
			// the callees this package actually resolves.
			if !referenced[fact.Object] {
				continue
			}
			origin = "imported"
		}
		listed[fact.Object] = true
		writeFact(buffer, action, fact.Object, origin, fact.Fact, kinds)
	}
	// The lifecycle pass exports no fact for a function proven to do nothing
	// with its parameters; its in-memory summaries still list every function
	// it summarized, so those appear here with their empty claim.
	summaries, ok := action.Result.(lifecyclefacts.Summaries)
	if !ok {
		return
	}
	functions := slices.SortedFunc(maps.Keys(summaries), func(left, right *ssa.Function) int {
		if order := comparePositions(action, left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.String(), right.String())
	})
	for _, function := range functions {
		object := function.Object()
		if object == nil || listed[object] || filter != "" && object.Name() != filter {
			continue
		}
		fact := summaries[function]
		writeFact(buffer, action, object, "exported here", &fact, kinds)
	}
}

// writeFact prints the selected kinds of one fact under a header naming the
// publishing pass and the object; a fact with nothing selected prints nothing.
func writeFact(buffer *bytes.Buffer, action *checker.Action, object types.Object, origin string, fact any, kinds map[string]bool) {
	kind := actionKind(action.Analyzer)
	var lines []string
	if kinds[kind] || kind != kindLifecycle && kind != kindResult && kind != kindConcurrency {
		lines = []string{fmt.Sprint(fact)}
		if describer, ok := fact.(factDescriber); ok {
			lines = describer.DescribeFact(object)
		}
		if len(lines) == 0 {
			lines = []string{"no parameter is proven on every return"}
		}
	}
	if describer, ok := fact.(heapDescriber); ok && kinds[kindHeap] {
		lines = append(lines, describer.DescribeHeap(object)...)
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(buffer, "%s %s (%s, %s)\n", action.Analyzer.Name, objectName(object), origin, position(action, object.Pos()))
	for _, line := range lines {
		fmt.Fprintf(buffer, "  %s\n", line)
	}
}

// referencedCallees collects the objects of the static callees the lifecycle
// pass resolved for this package.
func referencedCallees(action *checker.Action) map[types.Object]bool {
	referenced := map[types.Object]bool{}
	summaries, ok := action.Result.(lifecyclefacts.Summaries)
	if !ok {
		return referenced
	}
	for function := range summaries {
		if object := function.Object(); object != nil {
			referenced[object] = true
		}
	}
	return referenced
}

func objectName(object types.Object) string {
	if function, ok := object.(*types.Func); ok {
		return function.FullName()
	}
	return object.Name()
}

func position(action *checker.Action, pos token.Pos) string {
	return action.Package.Fset.Position(pos).String()
}

// comparePositions orders two positions by file name, then offset, which
// is stable across runs where the raw token.Pos values are not.
func comparePositions(action *checker.Action, left, right token.Pos) int {
	a, b := action.Package.Fset.Position(left), action.Package.Fset.Position(right)
	if a.Filename != b.Filename {
		return strings.Compare(a.Filename, b.Filename)
	}
	return a.Offset - b.Offset
}
