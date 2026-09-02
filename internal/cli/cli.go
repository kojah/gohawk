// Package cli implements the gohawk command.
package cli

import (
	"errors"
	"flag"
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

func Main() int {
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
		return 0
	}
	return result.exitCode
}

func runCLI(arguments []string, runtime cliRuntime) cliResult {
	if humanVersionRequested(arguments) {
		printHumanVersion(runtime.output)
		return cliResult{}
	}
	originalArguments := append([]string(nil), arguments...)
	if result, handled := runInformationalCommand(arguments, runtime); handled {
		return result
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
		writeLine(runtime.errorsOutput, "gohawk:", err)
		return cliResult{exitCode: 2}
	}
	richOutput := useRichOutput(originalArguments, runtime.getenv(richOutputChild) != "")
	selectedArguments := richOutputArguments(plan, richOutput)
	if richOutput {
		return cliResult{exitCode: runtime.richOutput(selectedArguments, runtime.errorsOutput)}
	}
	return cliResult{invocation: &analysisInvocation{arguments: selectedArguments, analyzers: plan.analyzers}}
}

func runInformationalCommand(arguments []string, runtime cliRuntime) (cliResult, bool) {
	if len(arguments) <= 1 {
		return cliResult{}, false
	}
	var err error
	switch arguments[1] {
	case "list":
		err = printAnalyzerList(arguments[2:], runtime.output, runtime.errorsOutput)
	case "doc":
		err = printDocumentation(arguments[2:], runtime.output, runtime.errorsOutput)
	case "ssa":
		err = printSSA(arguments[2:], runtime.output, runtime.errorsOutput)
	default:
		return cliResult{}, false
	}
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return cliResult{}, true
	}
	writeLine(runtime.errorsOutput, "gohawk "+arguments[1]+":", err)
	return cliResult{exitCode: 2}, true
}

func richOutputArguments(plan executionPlan, richOutput bool) []string {
	arguments := plan.arguments
	if !richOutput {
		return arguments
	}
	if len(plan.request.checks.enabled) > 0 {
		arguments = slices.Insert(arguments, 1, "-enable-checks="+strings.Join(sortedKeys(plan.request.checks.enabled), ","))
	}
	if len(plan.disabledChecks) > 0 {
		arguments = slices.Insert(arguments, 1, "-disable-checks="+strings.Join(sortedKeys(plan.disabledChecks), ","))
	}
	return arguments
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
