package cli

import (
	"flag"
	"fmt"
	"maps"
	"strings"

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
	// ceiling is the most permissive tier the run admits without naming a
	// check; it defaults to core.
	ceiling gohawk.CheckTier
}

// executionPlan is the fully resolved analysis invocation. Keeping selection
// policy out of runCLI gives the vet-tool handshake and the delegated run one
// model to execute rather than independently interpreting flags.
type executionPlan struct {
	arguments      []string
	analyzers      []*analysis.Analyzer
	request        selectionRequest
	disabledChecks map[string]bool
}

// buildExecutionPlan resolves selection flags into the analyzers to run and the
// arguments the vet-tool driver should see.
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
	disabledChecks := effectiveDisabledChecks(metadata, selection, request.checks)
	return executionPlan{
		arguments:      selection.arguments,
		analyzers:      withDisabledChecks(analyzers, metadata, disabledChecks),
		request:        request,
		disabledChecks: disabledChecks,
	}, nil
}

func registerSelectionFlags() {
	flag.Bool("enable-all", false, "enable every analyzer and check at every tier")
	flag.String("tier", string(gohawk.CheckTierCore), "most permissive tier to run without naming a check: core or experimental")
	flag.String("enable", "", "enable comma-separated analyzers")
	flag.String("disable", "", "disable comma-separated analyzers")
	flag.String("enable-checks", "", "enable comma-separated checks by stable ID")
	flag.String("disable-checks", "", "disable comma-separated checks by stable ID")
	flag.String("enable-groups", "", "enable comma-separated analyzer groups")
	flag.String("disable-groups", "", "disable comma-separated analyzer groups")
	analysisTrace.RegisterFlags(flag.CommandLine)
	check.RegisterFlags(flag.CommandLine)
}

type analyzerCheckSelection struct {
	arguments        []string
	normallySelected map[string]bool
	enableAll        bool
	ceiling          gohawk.CheckTier
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
	ceiling, remaining, err := requestedTier(remaining)
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
		if option, ok := removedAnalyzerOption(argument, names); ok && !allowAnalyzerFlags {
			return selectionRequest{}, fmt.Errorf(
				"analyzer option %q was removed; analyzers have no options, so select checks with -enable-checks or -disable-checks", option)
		}
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
		ceiling:   ceiling,
	}, nil
}

// removedAnalyzerOption returns the flag name when argument sets an analyzer
// option, such as -goroutineownership.mode=join. Analyzers once took options;
// naming the flag in an error is clearer than go vet's "flag provided but not
// defined".
func removedAnalyzerOption(argument string, analyzers map[string]bool) (string, bool) {
	name, ok := flagName(argument)
	if !ok {
		return "", false
	}
	analyzer, _, found := strings.Cut(name, ".")
	return name, found && analyzers[analyzer]
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
		return analyzerCheckSelection{
			arguments: remaining, normallySelected: normallySelected, enableAll: enableAll, ceiling: request.ceiling,
		}
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
		arguments: append(result, remaining[1:]...), normallySelected: normallySelected, enableAll: enableAll, ceiling: request.ceiling,
	}
}
