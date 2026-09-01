package cli

import (
	"flag"
	"fmt"
	"maps"
	"strconv"
	"strings"

	analysisTrace "github.com/kojah/gohawk/analysisutil/trace"
	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
)

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
func buildExecutionPlan(arguments []string, analyzers []*analysis.Analyzer, groups []gohawk.AnalyzerGroup, metadata map[string]gohawk.AnalyzerInfo, allowAnalyzerFlags bool) (executionPlan, error) {
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
}

type analyzerCheckSelection struct {
	arguments        []string
	normallySelected map[string]bool
	enableAll        bool
}

func withAnalyzerSelection(arguments []string, analyzers []*analysis.Analyzer, groups []gohawk.AnalyzerGroup, metadata map[string]gohawk.AnalyzerInfo, allowAnalyzerFlags bool) ([]string, error) {
	result, err := withAnalyzerCheckSelection(arguments, analyzers, groups, metadata, nil, allowAnalyzerFlags)
	return result.arguments, err
}

func withAnalyzerCheckSelection(arguments []string, analyzers []*analysis.Analyzer, groups []gohawk.AnalyzerGroup, metadata map[string]gohawk.AnalyzerInfo, checkOwners map[string]bool, allowAnalyzerFlags bool) (analyzerCheckSelection, error) {
	request, err := parseSelectionRequest(arguments, analyzers, groups, metadata, allowAnalyzerFlags)
	if err != nil {
		return analyzerCheckSelection{}, err
	}
	if checkOwners != nil {
		request.owners = checkOwners
	}
	return resolveAnalyzerSelection(request, analyzers, groups, metadata), nil
}

func parseSelectionRequest(arguments []string, analyzers []*analysis.Analyzer, groups []gohawk.AnalyzerGroup, metadata map[string]gohawk.AnalyzerInfo, allowAnalyzerFlags bool) (selectionRequest, error) {
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

func resolveAnalyzerSelection(request selectionRequest, analyzers []*analysis.Analyzer, groups []gohawk.AnalyzerGroup, metadata map[string]gohawk.AnalyzerInfo) analyzerCheckSelection {
	if len(request.arguments) > 1 && request.arguments[1] == "help" {
		return analyzerCheckSelection{arguments: request.arguments}
	}
	nameSelection := request.analyzers
	groupSelection := request.groups
	remaining := request.arguments
	enableAll := request.enableAll
	explicit := request.explicit
	checkOwners := request.owners
	hasExplicitEnabled := false
	for _, enabled := range explicit {
		hasExplicitEnabled = hasExplicitEnabled || enabled
	}
	if len(checkOwners) == 0 && len(nameSelection.enabled) == 0 && len(nameSelection.disabled) == 0 && len(groupSelection.enabled) == 0 && len(groupSelection.disabled) == 0 && (enableAll || hasExplicitEnabled) {
		normallySelected := make(map[string]bool)
		if enableAll {
			for _, analyzer := range analyzers {
				normallySelected[analyzer.Name] = true
			}
		}
		for name, enabled := range explicit {
			normallySelected[name] = enabled
		}
		return analyzerCheckSelection{arguments: remaining, normallySelected: normallySelected, enableAll: enableAll}
	}
	selected := make(map[string]bool)
	switch {
	case enableAll:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = true
		}
	case len(groupSelection.enabled) > 0:
		for _, group := range groups {
			if !groupSelection.enabled[group.Name] {
				continue
			}
			for _, analyzer := range group.Analyzers {
				selected[analyzer.Name] = true
			}
		}
	case len(groupSelection.disabled) > 0 || len(nameSelection.disabled) > 0:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = metadata[analyzer.Name].EnabledByDefault()
		}
	case len(nameSelection.enabled) > 0:
		// A positive analyzer list establishes its own selection base.
	case hasExplicitEnabled:
		// Naming an analyzer explicitly selects only named analyzers, preserving
		// the multichecker convention when no group selector establishes a base.
	case len(checkOwners) > 0:
		// An explicit check list establishes its own selection base. Its owning
		// analyzers are added after ordinary analyzer selection is resolved.
	default:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = metadata[analyzer.Name].EnabledByDefault()
		}
	}
	for _, group := range groups {
		if !groupSelection.disabled[group.Name] {
			continue
		}
		for _, analyzer := range group.Analyzers {
			selected[analyzer.Name] = false
		}
	}
	for name := range nameSelection.disabled {
		selected[name] = false
	}
	for name := range nameSelection.enabled {
		selected[name] = true
	}
	for name, enabled := range explicit {
		selected[name] = enabled
	}
	normallySelected := maps.Clone(selected)
	for owner := range checkOwners {
		selected[owner] = true
	}
	for name := range nameSelection.disabled {
		selected[name] = false
	}
	for name, enabled := range explicit {
		if !enabled {
			selected[name] = false
		}
	}

	enabledFlags := make([]string, 0, len(selected))
	for _, analyzer := range analyzers {
		if selected[analyzer.Name] {
			enabledFlags = append(enabledFlags, "-"+analyzer.Name+"=true")
		}
	}
	result := make([]string, 0, len(remaining)+len(enabledFlags))
	result = append(result, remaining[0])
	result = append(result, enabledFlags...)
	return analyzerCheckSelection{
		arguments: append(result, remaining[1:]...), normallySelected: normallySelected, enableAll: enableAll,
	}
}

type analyzerNameSelection struct {
	enabled  map[string]bool
	disabled map[string]bool
}

type checkSelection struct {
	enabled  map[string]bool
	disabled map[string]bool
}

func requestedChecks(arguments []string, metadata map[string]gohawk.AnalyzerInfo) (checkSelection, []string, error) {
	available := make(map[string]bool)
	for _, info := range metadata {
		for _, check := range info.Checks {
			available[string(check.ID)] = true
		}
	}
	requested := checkSelection{enabled: make(map[string]bool), disabled: make(map[string]bool)}
	remaining := make([]string, 0, len(arguments))
	if len(arguments) > 0 {
		remaining = append(remaining, arguments[0])
	}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if value == argument || name != "enable-checks" && name != "disable-checks" {
			remaining = append(remaining, argument)
			continue
		}
		if !hasValue {
			trailing := arguments[index+1:]
			if len(trailing) == 0 {
				return checkSelection{}, nil, fmt.Errorf("-%s requires a comma-separated value", name)
			}
			raw = trailing[0]
			index++
		}
		if raw == "" {
			return checkSelection{}, nil, fmt.Errorf("-%s requires at least one check", name)
		}
		target, action := requested.enabled, "enabled"
		if name == "disable-checks" {
			target, action = requested.disabled, "disabled"
		}
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				return checkSelection{}, nil, fmt.Errorf("invalid empty check in %q", raw)
			}
			if !available[candidate] {
				return checkSelection{}, nil, fmt.Errorf("unknown check %q (run 'gohawk list -checks' to see stable check IDs)", candidate)
			}
			if target[candidate] {
				return checkSelection{}, nil, fmt.Errorf("check %q is %s more than once", candidate, action)
			}
			target[candidate] = true
		}
	}
	return requested, remaining, nil
}

func requestedDisabledChecks(arguments []string, metadata map[string]gohawk.AnalyzerInfo) (map[string]bool, []string, error) {
	requested, remaining, err := requestedChecks(arguments, metadata)
	return requested.disabled, remaining, err
}

func checkOwners(checks map[string]bool, metadata map[string]gohawk.AnalyzerInfo) map[string]bool {
	owners := make(map[string]bool)
	for analyzer, info := range metadata {
		for _, check := range info.Checks {
			if checks[string(check.ID)] {
				owners[analyzer] = true
			}
		}
	}
	return owners
}

func effectiveDisabledChecks(metadata map[string]gohawk.AnalyzerInfo, normallySelected map[string]bool, requested checkSelection, enableAll bool) map[string]bool {
	disabled := maps.Clone(requested.disabled)
	for analyzer, info := range metadata {
		for _, check := range info.Checks {
			id := string(check.ID)
			if requested.enabled[id] {
				continue
			}
			if !normallySelected[analyzer] {
				disabled[id] = true
				continue
			}
			if !enableAll && !check.EnabledByDefault() {
				disabled[id] = true
			}
		}
	}
	return disabled
}

func withDisabledChecks(analyzers []*analysis.Analyzer, metadata map[string]gohawk.AnalyzerInfo, disabled map[string]bool) []*analysis.Analyzer {
	result := make([]*analysis.Analyzer, 0, len(analyzers))
	for _, analyzer := range analyzers {
		analyzerDisabled := make(map[string]bool)
		for _, check := range metadata[analyzer.Name].Checks {
			if disabled[string(check.ID)] {
				analyzerDisabled[string(check.ID)] = true
			}
		}
		wrapped := *analyzer
		run := analyzer.Run
		allDisabled := len(analyzerDisabled) == len(metadata[analyzer.Name].Checks)
		wrapped.Run = func(pass *analysis.Pass) (any, error) {
			if allDisabled {
				return nil, nil
			}
			report := pass.Report
			pass.Report = func(diagnostic analysis.Diagnostic) {
				if analyzerDisabled[diagnostic.Category] {
					analysisTrace.EmitDiagnostic(pass, analyzer.Name, "decision", "check-disabled", analysisTrace.OutcomeAccepted, diagnostic)
					return
				}
				analysisTrace.EmitDiagnostic(pass, analyzer.Name, "decision", "diagnostic-reported", analysisTrace.OutcomeRejected, diagnostic)
				report(diagnostic)
			}
			defer func() { pass.Report = report }()
			return run(pass)
		}
		result = append(result, &wrapped)
	}
	return result
}

func requestedAnalyzers(arguments []string, available map[string]bool) (analyzerNameSelection, []string, error) {
	requested := analyzerNameSelection{enabled: make(map[string]bool), disabled: make(map[string]bool)}
	remaining := make([]string, 0, len(arguments))
	if len(arguments) > 0 {
		remaining = append(remaining, arguments[0])
	}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if value == argument || name != "enable" && name != "disable" {
			remaining = append(remaining, argument)
			continue
		}
		target, action := requested.enabled, "enabled"
		if name == "disable" {
			target, action = requested.disabled, "disabled"
		}
		if !hasValue {
			index++
			if index >= len(arguments) {
				return analyzerNameSelection{}, nil, fmt.Errorf("-%s requires a comma-separated value", name)
			}
			raw = arguments[index]
		}
		if raw == "" {
			return analyzerNameSelection{}, nil, fmt.Errorf("-%s requires at least one analyzer", name)
		}
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				return analyzerNameSelection{}, nil, fmt.Errorf("invalid empty analyzer in %q", raw)
			}
			if !available[candidate] {
				return analyzerNameSelection{}, nil, fmt.Errorf("unknown analyzer %q (run 'gohawk list' to see available analyzers)", candidate)
			}
			if target[candidate] {
				return analyzerNameSelection{}, nil, fmt.Errorf("analyzer %q is %s more than once", candidate, action)
			}
			target[candidate] = true
		}
	}
	for name := range requested.enabled {
		if requested.disabled[name] {
			return analyzerNameSelection{}, nil, fmt.Errorf("analyzer %q cannot be both enabled and disabled", name)
		}
	}
	return requested, remaining, nil
}

type analyzerGroupSelection struct {
	enabled  map[string]bool
	disabled map[string]bool
}

func requestedAnalyzerGroups(arguments []string, groups []gohawk.AnalyzerGroup) (analyzerGroupSelection, []string, error) {
	available := make(map[string]bool, len(groups))
	var choices []string
	for _, group := range groups {
		available[group.Name] = true
		choices = append(choices, group.Name)
	}
	requested := analyzerGroupSelection{enabled: make(map[string]bool), disabled: make(map[string]bool)}
	remaining := make([]string, 0, len(arguments))
	if len(arguments) > 0 {
		remaining = append(remaining, arguments[0])
	}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if value == argument || name != "enable-groups" && name != "disable-groups" {
			remaining = append(remaining, argument)
			continue
		}
		target, action := requested.enabled, "enabled"
		if name == "disable-groups" {
			target, action = requested.disabled, "disabled"
		}
		if !hasValue {
			index++
			if index >= len(arguments) {
				return analyzerGroupSelection{}, nil, fmt.Errorf("-%s requires a comma-separated value", name)
			}
			raw = arguments[index]
		}
		if raw == "" {
			return analyzerGroupSelection{}, nil, fmt.Errorf("-%s requires at least one group", name)
		}
		for _, candidate := range strings.Split(raw, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				return analyzerGroupSelection{}, nil, fmt.Errorf("invalid empty group in %q", raw)
			}
			if !available[candidate] {
				return analyzerGroupSelection{}, nil, fmt.Errorf("unknown analyzer group %q (choose from %s)", candidate, strings.Join(choices, ", "))
			}
			if target[candidate] {
				return analyzerGroupSelection{}, nil, fmt.Errorf("analyzer group %q is %s more than once", candidate, action)
			}
			target[candidate] = true
		}
	}
	for name := range requested.enabled {
		if requested.disabled[name] {
			return analyzerGroupSelection{}, nil, fmt.Errorf("analyzer group %q cannot be both enabled and disabled", name)
		}
	}
	return requested, remaining, nil
}

func enableAllRequested(arguments []string) bool {
	for _, argument := range arguments[1:] {
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if name != "enable-all" {
			continue
		}
		if !hasValue {
			return true
		}
		enabled, err := strconv.ParseBool(raw)
		return err == nil && enabled
	}
	return false
}

func analyzerSelection(argument string, names map[string]bool) (string, bool, bool) {
	value := strings.TrimLeft(argument, "-")
	if value == argument {
		return "", false, false
	}
	name, raw, hasValue := strings.Cut(value, "=")
	if !names[name] {
		return "", false, false
	}
	if !hasValue {
		return name, true, true
	}
	enabled, err := strconv.ParseBool(raw)
	return name, enabled, err == nil
}
