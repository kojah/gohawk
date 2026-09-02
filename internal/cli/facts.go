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

// The facts subcommand prints every fact the analyzers export for a package,
// as a lifecycle analyzer would import it. Enumeration is generic over fact
// families; each family renders its own facts through DescribeFact, and a
// family without a renderer falls back to the fact's String. A function that
// exports no fact is unknown to consumers, not proven harmless, so the
// lifecycle view can also list the functions it summarized without proving
// a mask.

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
	all := flags.Bool("all", false, "also list functions summarized without a proven mask")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk facts [-func NAME] [-tests] [-all] package...")
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
		if *all {
			writeUnprovenSummaries(&buffer, action, *nameFilter)
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
	slices.SortFunc(facts, func(left, right analysis.ObjectFact) int {
		if left.Object.Pos() != right.Object.Pos() {
			return int(left.Object.Pos() - right.Object.Pos())
		}
		return strings.Compare(left.Object.Name(), right.Object.Name())
	})
	for _, fact := range facts {
		if filter != "" && fact.Object.Name() != filter {
			continue
		}
		origin := "exported here"
		if fact.Object.Pkg() != action.Package.Types {
			origin = "imported"
		}
		fmt.Fprintf(buffer, "%s %s (%s, %s)\n", action.Analyzer.Name, objectName(fact.Object), origin, position(action, fact.Object.Pos()))
		lines := []string{fmt.Sprint(fact.Fact)}
		if describer, ok := fact.Fact.(factDescriber); ok {
			lines = describer.DescribeFact(fact.Object)
		}
		for _, line := range lines {
			fmt.Fprintf(buffer, "  %s\n", line)
		}
	}
}

// writeUnprovenSummaries lists the exported functions the lifecycle pass
// summarized without proving any mask. They export no fact, so a consumer
// treats calls to them as unknown; functions absent from both lists were not
// summarized at all.
func writeUnprovenSummaries(buffer *bytes.Buffer, action *checker.Action, filter string) {
	summaries, ok := action.Result.(lifecyclefacts.Summaries)
	if !ok {
		return
	}
	var names []string
	for function, fact := range summaries {
		if fact != (lifecyclefacts.Fact{}) || function.Pkg == nil || function.Pkg.Pkg != action.Package.Types {
			continue
		}
		if filter == "" || function.Name() == filter {
			names = append(names, function.String())
		}
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(buffer, "%s %s (summarized here)\n  no parameter is proven on every return\n", action.Analyzer.Name, name)
	}
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
