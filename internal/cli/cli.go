// Package cli implements the gohawk command.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

const filteredFlagsChild = "GOHAWK_FILTERED_FLAGS_CHILD"

type cliRuntime struct {
	output        io.Writer
	errorsOutput  io.Writer
	getenv        func(string) string
	filteredFlags func([]string, []*analysis.Analyzer, io.Writer, io.Writer) int
	richOutput    func([]string, io.Writer) int
}

type analysisInvocation struct {
	arguments []string
	analyzers []*analysis.Analyzer
}

type cliResult struct {
	exitCode   int
	invocation *analysisInvocation
}

// selectionRequest is the user's analyzer/check policy before defaults and
// ownership relationships are resolved.
func Main() {
	runtime := cliRuntime{
		output:        os.Stdout,
		errorsOutput:  os.Stderr,
		getenv:        os.Getenv,
		filteredFlags: printFilteredFlags,
		richOutput:    runWithRichOutput,
	}
	result := runCLI(os.Args, runtime)
	if result.invocation != nil {
		registerSelectionFlags()
		os.Args = result.invocation.arguments
		multichecker.Main(result.invocation.analyzers...)
		panic("unreachable")
	}
	if result.exitCode != 0 {
		os.Exit(result.exitCode)
	}
}

func runCLI(arguments []string, runtime cliRuntime) cliResult {
	if humanVersionRequested(arguments) {
		printHumanVersion(runtime.output)
		return cliResult{}
	}
	originalArguments := append([]string(nil), arguments...)
	if len(arguments) > 1 && arguments[1] == "list" {
		if err := printAnalyzerList(arguments[2:], runtime.output, runtime.errorsOutput); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return cliResult{}
			}
			fmt.Fprintln(runtime.errorsOutput, "gohawk list:", err)
			return cliResult{exitCode: 2}
		}
		return cliResult{}
	}
	if len(arguments) > 1 && arguments[1] == "doc" {
		if err := printDocumentation(arguments[2:], runtime.output, runtime.errorsOutput); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return cliResult{}
			}
			fmt.Fprintln(runtime.errorsOutput, "gohawk doc:", err)
			return cliResult{exitCode: 2}
		}
		return cliResult{}
	}
	analyzers := gohawk.Analyzers()
	if flagsRequested(arguments) && runtime.getenv(filteredFlagsChild) == "" {
		return cliResult{exitCode: runtime.filteredFlags(arguments, analyzers, runtime.output, runtime.errorsOutput)}
	}
	if generalHelpRequested(arguments) {
		printGeneralHelp(runtime.errorsOutput)
		return cliResult{}
	}
	metadata := gohawk.AnalyzerMetadata()
	plan, err := buildExecutionPlan(arguments, analyzers, gohawk.AnalyzerGroups(), metadata, runtime.getenv(richOutputChild) != "")
	if err != nil {
		fmt.Fprintln(runtime.errorsOutput, "gohawk:", err)
		return cliResult{exitCode: 2}
	}
	selectedArguments := plan.arguments
	richOutput := useRichOutput(originalArguments, runtime.getenv(richOutputChild) != "")
	if richOutput && len(plan.request.checks.enabled) > 0 {
		checks := make([]string, 0, len(plan.request.checks.enabled))
		for check := range plan.request.checks.enabled {
			checks = append(checks, check)
		}
		slices.Sort(checks)
		selectedArguments = slices.Insert(selectedArguments, 1, "-enable-checks="+strings.Join(checks, ","))
	}
	if richOutput && len(plan.disabledChecks) > 0 {
		checks := make([]string, 0, len(plan.disabledChecks))
		for check := range plan.disabledChecks {
			checks = append(checks, check)
		}
		slices.Sort(checks)
		selectedArguments = slices.Insert(selectedArguments, 1, "-disable-checks="+strings.Join(checks, ","))
	}
	if richOutput {
		return cliResult{exitCode: runtime.richOutput(selectedArguments, runtime.errorsOutput)}
	}
	return cliResult{invocation: &analysisInvocation{arguments: selectedArguments, analyzers: plan.analyzers}}
}
