package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// The ssa subcommand prints the SSA form the analyzers see, built with the
// same builder mode as the buildssa pass, so a reader can check what an
// instruction lowered to instead of simulating the lowering. It is a
// debugging aid for one package or function at a time, not an analysis.

func printSSA(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("ssa", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	functionFilter := flags.String("func", "", "print only functions whose name or enclosing function name matches")
	includeTests := flags.Bool("tests", false, "also load the package's test variant")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk ssa [-func NAME] [-tests] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		return errors.New("at least one package pattern is required")
	}
	rendered, err := RenderSSA(patterns, *functionFilter, *includeTests)
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, rendered)
	return err
}

// RenderSSA returns the SSA form of the source functions in the matched
// packages whose name matches functionFilter, exactly as the ssa subcommand
// prints it. The documentation generator uses it so the dump on the
// Understanding SSA page is the real output rather than a transcript.
func RenderSSA(patterns []string, functionFilter string, includeTests bool) (string, error) {
	functions, fset, err := loadSSAFunctions(patterns, includeTests)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	for _, function := range functions {
		if !ssaFunctionSelected(function, functionFilter) {
			continue
		}
		fmt.Fprintf(&buffer, "// %s\n", fset.Position(function.Pos()))
		ssa.WriteFunction(&buffer, function)
		buffer.WriteString("\n")
	}
	if buffer.Len() == 0 {
		return "", fmt.Errorf("no function matched %q", functionFilter)
	}
	return buffer.String(), nil
}

// loadSSAFunctions builds SSA for the matched packages and returns their
// source functions in position order, including function literals.
func loadSSAFunctions(patterns []string, includeTests bool) ([]*ssa.Function, *token.FileSet, error) {
	config := &packages.Config{Mode: packages.LoadAllSyntax, Tests: includeTests}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, nil, err
	}
	if packages.PrintErrors(loaded) > 0 {
		return nil, nil, errors.New("packages have load errors")
	}
	program, built := ssautil.Packages(loaded, ssa.BuilderMode(0))
	program.Build()
	roots := map[*ssa.Package]bool{}
	for _, pkg := range built {
		if pkg != nil {
			roots[pkg] = true
		}
	}
	var functions []*ssa.Function
	for function := range ssautil.AllFunctions(program) {
		if function.Pkg == nil || !roots[function.Pkg] || function.Synthetic != "" && function.Parent() == nil {
			continue
		}
		functions = append(functions, function)
	}
	slices.SortFunc(functions, func(left, right *ssa.Function) int {
		if left.Pos() != right.Pos() {
			return int(left.Pos() - right.Pos())
		}
		return strings.Compare(left.String(), right.String())
	})
	return functions, program.Fset, nil
}

// ssaFunctionSelected matches the filter against the function's short name,
// its qualified name, and its enclosing functions, so `-func run` also prints
// the literals created inside run.
func ssaFunctionSelected(function *ssa.Function, filter string) bool {
	if filter == "" {
		return true
	}
	for candidate := function; candidate != nil; candidate = candidate.Parent() {
		if candidate.Name() == filter || candidate.String() == filter || candidate.RelString(nil) == filter {
			return true
		}
	}
	return false
}
