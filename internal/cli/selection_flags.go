package cli

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

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
		if err := addSelectedNames(raw, available, target, selectedNameKind{
			kind: "check", action: action, hint: "run 'gohawk list -checks' to see stable check IDs",
		}); err != nil {
			return checkSelection{}, nil, err
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

// effectiveDisabledChecks decides which checks of the selected analyzers
// stay silent. A check runs when it was named, when every tier is enabled,
// when its tier is within the ceiling, or when its analyzer was asked for by
// name and the check is no more than extended.
func effectiveDisabledChecks(
	metadata map[string]gohawk.AnalyzerInfo,
	selection analyzerCheckSelection,
	requested checkSelection,
) map[string]bool {
	disabled := maps.Clone(requested.disabled)
	for analyzer, info := range metadata {
		for _, check := range info.Checks {
			id := string(check.ID)
			if requested.enabled[id] {
				continue
			}
			if !selection.normallySelected[analyzer] {
				disabled[id] = true
				continue
			}
			admitted := selection.enableAll || check.EnabledAt(selection.ceiling) ||
				selection.named[analyzer] && check.EnabledAt(gohawk.CheckTierExtended)
			if !admitted {
				disabled[id] = true
			}
		}
	}
	return disabled
}

// requestedTier consumes a -tier flag and returns the ceiling it names,
// defaulting to core.
func requestedTier(arguments []string) (gohawk.CheckTier, []string, error) {
	ceiling := gohawk.CheckTierCore
	remaining := make([]string, 0, len(arguments))
	if len(arguments) > 0 {
		remaining = append(remaining, arguments[0])
	}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if value == argument || name != "tier" {
			remaining = append(remaining, argument)
			continue
		}
		if !hasValue {
			index++
			if index >= len(arguments) {
				return "", nil, errors.New("-tier requires a value: core, extended, or experimental")
			}
			raw = arguments[index]
		}
		parsed, err := gohawk.ParseCheckTier(raw)
		if err != nil {
			return "", nil, err
		}
		ceiling = parsed
	}
	return ceiling, remaining, nil
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
					analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
						Analyzer: analyzer.Name, Phase: "decision", Reason: "check-disabled", Outcome: analysisTrace.OutcomeAccepted, Diagnostic: diagnostic,
					})
					return
				}
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: "decision", Reason: "diagnostic-reported", Outcome: analysisTrace.OutcomeRejected, Diagnostic: diagnostic,
				})
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
		if err := addSelectedNames(raw, available, target, selectedNameKind{
			kind: "analyzer", action: action, hint: "run 'gohawk list' to see available analyzers",
		}); err != nil {
			return analyzerNameSelection{}, nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(requested.enabled)) {
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
		for candidate := range strings.SplitSeq(raw, ",") {
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
	for _, name := range slices.Sorted(maps.Keys(requested.enabled)) {
		if requested.disabled[name] {
			return analyzerGroupSelection{}, nil, fmt.Errorf("analyzer group %q cannot be both enabled and disabled", name)
		}
	}
	return requested, remaining, nil
}

// selectedNameKind words the errors for one kind of comma-separated
// selection list.
type selectedNameKind struct {
	kind   string
	action string
	hint   string
}

// addSelectedNames adds the comma-separated names in raw to target, rejecting
// an empty, unknown, or repeated entry.
func addSelectedNames(raw string, available, target map[string]bool, words selectedNameKind) error {
	for candidate := range strings.SplitSeq(raw, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return fmt.Errorf("invalid empty %s in %q", words.kind, raw)
		}
		if !available[candidate] {
			return fmt.Errorf("unknown %s %q (%s)", words.kind, candidate, words.hint)
		}
		if target[candidate] {
			return fmt.Errorf("%s %q is %s more than once", words.kind, candidate, words.action)
		}
		target[candidate] = true
	}
	return nil
}

func enableAllRequested(arguments []string) bool {
	return booleanFlagRequested(arguments, "enable-all")
}

// booleanFlagRequested reports whether a bare -name or an explicit -name=true
// appears among arguments, stopping at the first occurrence.
func booleanFlagRequested(arguments []string, flag string) bool {
	for _, argument := range arguments[1:] {
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if name != flag {
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
