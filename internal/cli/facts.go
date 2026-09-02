package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"slices"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
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
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk facts [-func NAME] [-tests] package...")
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
	slices.SortFunc(facts, func(left, right analysis.ObjectFact) int {
		if left.Object.Pos() != right.Object.Pos() {
			return int(left.Object.Pos() - right.Object.Pos())
		}
		return strings.Compare(left.Object.Name(), right.Object.Name())
	})
	referenced := referencedCallees(action)
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
		fmt.Fprintf(buffer, "%s %s (%s, %s)\n", action.Analyzer.Name, objectName(fact.Object), origin, position(action, fact.Object.Pos()))
		lines := []string{fmt.Sprint(fact.Fact)}
		if describer, ok := fact.Fact.(factDescriber); ok {
			lines = describer.DescribeFact(fact.Object)
		}
		if len(lines) == 0 {
			lines = []string{"no parameter is proven on every return"}
		}
		for _, line := range lines {
			fmt.Fprintf(buffer, "  %s\n", line)
		}
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
