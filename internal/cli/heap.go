package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// The heap subcommand prints how the heap model was derived: each local
// function's points-to graph, private helpers and literals included, with
// the summary the function exports beneath it. The facts subcommand prints
// only what is published across packages; this one prints the working graph
// behind it. By default the graphs are the ones the analysis built, with
// every callee summary applied, so an applied, unsummarized, widened, or
// escaped line explains a surprising claim. -bare builds each graph without
// any summary, which is what a unit test of the points-to model sees.

func printHeap(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("heap", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	functionFilter := flags.String("func", "", "print only functions whose name or enclosing function name matches")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	withSSA := flags.Bool("ssa", false, "print each function's SSA before its graph")
	bare := flags.Bool("bare", false, "build each graph without callee summaries, as a unit test sees it")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk heap [-func NAME] [-tests] [-ssa] [-bare] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) == 0 {
		return errors.New("at least one package pattern is required")
	}
	functions, err := heapFunctions(flags.Args(), *includeTests, *bare)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	for _, function := range functions {
		if !ssaFunctionSelected(function, *functionFilter) {
			continue
		}
		fmt.Fprintf(&buffer, "// %s\n", function.String())
		if *withSSA {
			ssa.WriteFunction(&buffer, function)
		}
		if summary, ok := heapmodel.RegisteredHeapSummary(function); ok && !*bare {
			for line := range strings.SplitSeq(strings.TrimSpace(summary.String()), "\n") {
				if line != "" {
					fmt.Fprintf(&buffer, "//   summary %s\n", line)
				}
			}
		}
		buffer.WriteString(heapmodel.RenderRegions(function))
		buffer.WriteString("\n")
	}
	if buffer.Len() == 0 {
		return fmt.Errorf("no function matched %q", *functionFilter)
	}
	_, err = output.Write(buffer.Bytes())
	return err
}

// heapFunctions returns the matched packages' functions with bodies, in
// position order. Without bare, the lifecycle pass runs first, which builds
// and registers every summary a graph applies, callees' included.
func heapFunctions(patterns []string, includeTests, bare bool) ([]*ssa.Function, error) {
	if bare {
		functions, _, err := loadSSAFunctions(patterns, includeTests)
		return slices.DeleteFunc(functions, func(function *ssa.Function) bool { return len(function.Blocks) == 0 }), err
	}
	loaded, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax, Tests: includeTests}, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(loaded) > 0 {
		return nil, errors.New("packages have load errors")
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{lifecyclefacts.Analyzer}, loaded, &checker.Options{Sequential: true})
	if err != nil {
		return nil, err
	}
	var functions []*ssa.Function
	for _, action := range graph.Roots {
		if action.Err != nil {
			return nil, action.Err
		}
		functions = append(functions, packageFunctions(action)...)
	}
	return functions, nil
}

// packageFunctions returns the functions with bodies of the action's
// package, found through the program the lifecycle pass summarized.
func packageFunctions(action *checker.Action) []*ssa.Function {
	summaries, ok := action.Result.(lifecyclefacts.Summaries)
	if !ok {
		return nil
	}
	var program *ssa.Program
	for function := range summaries {
		if function.Pkg != nil && function.Pkg.Pkg == action.Package.Types {
			program = function.Prog
			break
		}
	}
	if program == nil {
		return nil
	}
	var functions []*ssa.Function
	for function := range ssautil.AllFunctions(program) {
		if function.Pkg != nil && function.Pkg.Pkg == action.Package.Types && len(function.Blocks) != 0 {
			functions = append(functions, function)
		}
	}
	slices.SortFunc(functions, func(left, right *ssa.Function) int {
		if order := comparePositions(action, left.Pos(), right.Pos()); order != 0 {
			return order
		}
		return strings.Compare(left.String(), right.String())
	})
	return functions
}
