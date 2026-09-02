package cli

import (
	"flag"
	"fmt"
	"maps"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// selectionRequest is the user's analyzer/check policy before defaults and
// ownership relationships are resolved.
type selectionRequest struct {
	arguments []string
	analyzers analyzerNameSelection
	groups    analyzerGroupSelection
	checks    checkSelection
	enableAll bool
	explicit  map[string]bool
	owners    map[string]bool
}

// executionPlan is the fully resolved analysis invocation. Keeping selection
// policy out of runCLI gives the multichecker and rich-output paths one model
// to execute rather than independently interpreting flags.
type executionPlan struct {
	arguments      []string
	analyzers      []*analysis.Analyzer
	request        selectionRequest
	disabledChecks map[string]bool
}

// Main runs the gohawk command and exits with its result. The analyzer engine
// remains at this boundary because multichecker.Main owns os.Exit as part of
func buildExecutionPlan(
	arguments []string,
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
	allowAnalyzerFlags bool,
) (executionPlan, error) {
	request, err := parseSelectionRequest(arguments, analyzers, groups, metadata, allowAnalyzerFlags)
	if err != nil {
		return executionPlan{}, err
	}
	selection := resolveAnalyzerSelection(request, analyzers, groups, metadata)
	disabledChecks := effectiveDisabledChecks(metadata, selection.normallySelected, request.checks, selection.enableAll)
	return executionPlan{
		arguments:      selection.arguments,
		analyzers:      withDisabledChecks(analyzers, metadata, disabledChecks),
		request:        request,
		disabledChecks: disabledChecks,
	}, nil
}

func registerSelectionFlags() {
	flag.Bool("enable-all", false, "enable every analyzer and check, including opt-in entries")
	flag.String("enable", "", "enable comma-separated analyzers")
	flag.String("disable", "", "disable comma-separated analyzers")
	flag.String("enable-checks", "", "enable comma-separated checks by stable ID")
	flag.String("disable-checks", "", "disable comma-separated checks by stable ID")
	flag.String("enable-groups", "", "enable comma-separated analyzer groups, including opt-in analyzers")
	flag.String("disable-groups", "", "disable comma-separated analyzer groups")
	analysisTrace.RegisterFlags(flag.CommandLine)
	check.RegisterFlags(flag.CommandLine)
}

type analyzerCheckSelection struct {
	arguments        []string
	normallySelected map[string]bool
	enableAll        bool
}

func withAnalyzerSelection(
	arguments []string,
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
	allowAnalyzerFlags bool,
) ([]string, error) {
	result, err := withAnalyzerCheckSelection(arguments, analyzers, groups, metadata, nil, allowAnalyzerFlags)
	return result.arguments, err
}

func withAnalyzerCheckSelection(
	arguments []string,
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
	checkOwners map[string]bool,
	allowAnalyzerFlags bool,
) (analyzerCheckSelection, error) {
	request, err := parseSelectionRequest(arguments, analyzers, groups, metadata, allowAnalyzerFlags)
	if err != nil {
		return analyzerCheckSelection{}, err
	}
	if checkOwners != nil {
		request.owners = checkOwners
	}
	return resolveAnalyzerSelection(request, analyzers, groups, metadata), nil
}

func parseSelectionRequest(
	arguments []string,
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
	allowAnalyzerFlags bool,
) (selectionRequest, error) {
	if len(arguments) > 1 && arguments[1] == "help" {
		return selectionRequest{arguments: arguments}, nil
	}
	checks, remaining, err := requestedChecks(arguments, metadata)
	if err != nil {
		return selectionRequest{}, err
	}
	names := make(map[string]bool, len(analyzers))
	for _, analyzer := range analyzers {
		names[analyzer.Name] = true
	}
	nameSelection, remaining, err := requestedAnalyzers(remaining, names)
	if err != nil {
		return selectionRequest{}, err
	}
	groupSelection, remaining, err := requestedAnalyzerGroups(remaining, groups)
	if err != nil {
		return selectionRequest{}, err
	}
	enableAll := enableAllRequested(remaining)
	explicit := make(map[string]bool)
	for _, argument := range remaining[1:] {
		name, enabled, ok := analyzerSelection(argument, names)
		if !ok {
			continue
		}
		if !allowAnalyzerFlags {
			replacement := "-enable=" + name
			if !enabled {
				replacement = "-disable=" + name
			}
			return selectionRequest{}, fmt.Errorf("analyzer Boolean flag %q is no longer supported; use %s", argument, replacement)
		}
		explicit[name] = enabled
	}
	return selectionRequest{
		arguments: remaining,
		analyzers: nameSelection,
		groups:    groupSelection,
		checks:    checks,
		enableAll: enableAll,
		explicit:  explicit,
		owners:    checkOwners(checks.enabled, metadata),
	}, nil
}

func resolveAnalyzerSelection(
	request selectionRequest,
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
) analyzerCheckSelection {
	if len(request.arguments) > 1 && request.arguments[1] == "help" {
		return analyzerCheckSelection{arguments: request.arguments}
	}
	nameSelection := request.analyzers
	groupSelection := request.groups
	remaining := request.arguments
	enableAll := request.enableAll
	explicit := request.explicit
	checkOwners := request.owners
	hasExplicitEnabled := anyEnabled(explicit)
	if nativeSelectionSuffices(request, hasExplicitEnabled) {
		normallySelected := nativeAnalyzerSelection(analyzers, explicit, enableAll)
		return analyzerCheckSelection{arguments: remaining, normallySelected: normallySelected, enableAll: enableAll}
	}
	selected := baseAnalyzerSelection(analyzers, groups, metadata, request, hasExplicitEnabled)
	applyAnalyzerSelection(selected, groups, nameSelection, groupSelection, explicit)
	normallySelected := maps.Clone(selected)
	applyCheckOwners(selected, checkOwners, nameSelection.disabled, explicit)

	enabledFlags := enabledAnalyzerFlags(analyzers, selected)
	result := make([]string, 0, len(remaining)+len(enabledFlags))
	result = append(result, remaining[0])
	result = append(result, enabledFlags...)
	return analyzerCheckSelection{
		arguments: append(result, remaining[1:]...), normallySelected: normallySelected, enableAll: enableAll,
	}
}
