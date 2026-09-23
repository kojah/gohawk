package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// The facts subcommand prints the facts a package exports and the imported
// facts its calls resolve to, as a lifecycle analyzer would see them.
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

func printFacts(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("facts", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	nameFilter := flags.String("func", "", "print only facts attached to the function with this name")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	regions := flags.Bool("regions", false, "also print each local function's points-to graph as the analysis saw it")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk facts [-func NAME] [-tests] [-regions] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) == 0 {
		return errors.New("at least one package pattern is required")
	}
	config := &packages.Config{Mode: packages.LoadAllSyntax, Tests: *includeTests}
	loaded, err := packages.Load(config, flags.Args()...)
	if err != nil {
		return err
	}
	if packages.PrintErrors(loaded) > 0 {
		return errors.New("packages have load errors")
	}
	graph, err := checker.Analyze(factAnalyzers(), loaded, &checker.Options{Sequential: true})
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	for _, action := range graph.Roots {
		if action.Err != nil {
			return action.Err
		}
		writeObjectFacts(&buffer, action, *nameFilter)
		if *regions {
			writeRegions(&buffer, action, *nameFilter)
		}
	}
	if buffer.Len() == 0 {
		return fmt.Errorf("no fact matched %q", *nameFilter)
	}
	_, err = output.Write(buffer.Bytes())
	return err
}

// factAnalyzers returns the lifecycle prerequisite and every catalog analyzer
// that exports facts of its own.
func factAnalyzers() []*analysis.Analyzer {
	analyzers := []*analysis.Analyzer{lifecyclefacts.Analyzer}
	for _, analyzer := range gohawk.Analyzers() {
		if len(analyzer.FactTypes) > 0 {
			analyzers = append(analyzers, analyzer)
		}
	}
	return analyzers
}

func writeObjectFacts(buffer *bytes.Buffer, action *checker.Action, filter string) {
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
	referenced := referencedCallees(action)
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
		writeFact(buffer, action, fact.Object, origin, fact.Fact)
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
		writeFact(buffer, action, object, "exported here", &fact)
	}
}

// writeRegions prints the points-to graph of each local function, from the
// graphs the analysis built, so the regions reflect the callee summaries
// that were applied. Every function of the package is printed, private
// helpers and literals included, each with the heap summary the registry
// holds for it: a private helper's projection is applied by its callers
// even though no fact carries it.
func writeRegions(buffer *bytes.Buffer, action *checker.Action, filter string) {
	summaries, ok := action.Result.(lifecyclefacts.Summaries)
	if !ok {
		return
	}
	var program *ssa.Program
	for function := range summaries {
		if function.Pkg != nil && function.Pkg.Pkg == action.Package.Types {
			program = function.Prog
			break
		}
	}
	if program == nil {
		return
	}
	var functions []*ssa.Function
	for function := range ssautil.AllFunctions(program) {
		if function.Pkg != nil && function.Pkg.Pkg == action.Package.Types && len(function.Blocks) != 0 &&
			(filter == "" || function.Name() == filter || function.Object() != nil && function.Object().Name() == filter) {
			functions = append(functions, function)
		}
	}
	slices.SortFunc(functions, func(left, right *ssa.Function) int {
		if order := comparePositions(action, left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.String(), right.String())
	})
	for _, function := range functions {
		fmt.Fprintf(buffer, "// %s\n", function.String())
		if summary, ok := ssaflow.RegisteredHeapSummary(function); ok {
			for line := range strings.SplitSeq(strings.TrimSpace(summary.String()), "\n") {
				if line != "" {
					fmt.Fprintf(buffer, "//   %s\n", line)
				}
			}
		}
		buffer.WriteString(ssaflow.RenderRegions(function))
	}
}

func writeFact(buffer *bytes.Buffer, action *checker.Action, object types.Object, origin string, fact analysis.Fact) {
	fmt.Fprintf(buffer, "%s %s (%s, %s)\n", action.Analyzer.Name, objectName(object), origin, position(action, object.Pos()))
	lines := []string{fmt.Sprint(fact)}
	if describer, ok := fact.(factDescriber); ok {
		lines = describer.DescribeFact(object)
	}
	if len(lines) == 0 {
		lines = []string{"no parameter is proven on every return"}
	}
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
