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
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk dump ssa [-func NAME] package...")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		return errors.New("at least one package pattern is required")
	}
	rendered, err := RenderSSA(patterns, *functionFilter)
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
func RenderSSA(patterns []string, functionFilter string) (string, error) {
	functions, fset, err := loadSSAFunctions(patterns)
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
func loadSSAFunctions(patterns []string) ([]*ssa.Function, *token.FileSet, error) {
	config := &packages.Config{Mode: packages.LoadAllSyntax}
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
	for function := range packageSourceFunctions(program, func(pkg *ssa.Package) bool { return roots[pkg] }) {
		if function.Synthetic != "" && function.Parent() == nil {
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

// packageSourceFunctions returns the functions of the selected packages,
// including every declared method and function literal. ssautil.AllFunctions
// alone is not enough: it finds methods only through the method sets of
// runtime types, so a method of a type never converted to an interface --
// the common case for a mutex-guarded server -- would silently be missing
// from a dump.
func packageSourceFunctions(program *ssa.Program, selected func(*ssa.Package) bool) map[*ssa.Function]bool {
	functions := map[*ssa.Function]bool{}
	var add func(*ssa.Function)
	add = func(function *ssa.Function) {
		if function == nil || function.Pkg == nil || !selected(function.Pkg) || functions[function] {
			return
		}
		functions[function] = true
		for _, literal := range function.AnonFuncs {
			add(literal)
		}
	}
	for function := range ssautil.AllFunctions(program) {
		add(function)
	}
	for _, pkg := range program.AllPackages() {
		if !selected(pkg) {
			continue
		}
		for _, member := range pkg.Members {
			named, ok := member.(*ssa.Type)
			if !ok {
				continue
			}
			for _, receiver := range []types.Type{named.Type(), types.NewPointer(named.Type())} {
				for selection := range program.MethodSets.MethodSet(receiver).Methods() {
					add(program.MethodValue(selection))
				}
			}
		}
	}
	return functions
}
