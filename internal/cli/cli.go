// Package cli implements the gohawk command.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

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
// the go/analysis driver protocol.
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
}

type advertisedFlag struct {
	Name  string
	Bool  bool
	Usage string
}

func flagsRequested(arguments []string) bool {
	for _, argument := range arguments[1:] {
		value := strings.TrimLeft(argument, "-")
		name, raw, hasValue := strings.Cut(value, "=")
		if name != "flags" {
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

func printFilteredFlags(arguments []string, analyzers []*analysis.Analyzer, output, errorsOutput io.Writer) int {
	return printFilteredFlagsUsing(arguments, analyzers, output, errorsOutput, executeProcess)
}

func printFilteredFlagsUsing(arguments []string, analyzers []*analysis.Analyzer, output, errorsOutput io.Writer, execute processExecutor) int {
	result, err := execute(arguments[0], arguments[1:], []string{filteredFlagsChild + "=1"})
	if err != nil {
		_, _ = errorsOutput.Write(result.stderr)
		if result.exitCode >= 0 {
			return result.exitCode
		}
		fmt.Fprintln(errorsOutput, "gohawk: inspect analyzer flags:", err)
		return 1
	}
	var flags []advertisedFlag
	if err := json.Unmarshal(result.stdout, &flags); err != nil {
		fmt.Fprintln(errorsOutput, "gohawk: decode analyzer flags:", err)
		return 1
	}
	hidden := make(map[string]bool, len(analyzers))
	for _, analyzer := range analyzers {
		hidden[analyzer.Name] = true
	}
	flags = slices.DeleteFunc(flags, func(candidate advertisedFlag) bool {
		return hidden[candidate.Name]
	})
	encoded, err := json.MarshalIndent(flags, "", "\t")
	if err != nil {
		fmt.Fprintln(errorsOutput, "gohawk: encode analyzer flags:", err)
		return 1
	}
	_, _ = output.Write(encoded)
	return 0
}

func printAnalyzerList(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	defaultsOnly := flags.Bool("defaults", false, "show only analyzers enabled by default")
	optInOnly := flags.Bool("opt-in", false, "show only opt-in analyzers")
	showChecks := flags.Bool("checks", false, "show stable check IDs instead of analyzer names")
	flags.Usage = func() {
		fmt.Fprintln(errorsOutput, "usage: gohawk list [-checks] [-defaults | -opt-in]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *defaultsOnly && *optInOnly {
		return errors.New("-defaults and -opt-in cannot be used together")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	metadata := gohawk.AnalyzerMetadata()
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if *showChecks {
		fmt.Fprintln(table, "CHECK\tPROFILE\tGROUP\tTAGS")
	} else {
		fmt.Fprintln(table, "ANALYZER\tPROFILE\tGROUP")
	}
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			info := metadata[analyzer.Name]
			isDefault := info.EnabledByDefault()
			if !*showChecks && ((*defaultsOnly && !isDefault) || (*optInOnly && isDefault)) {
				continue
			}
			profile := "default"
			if !isDefault {
				profile = "opt-in"
			}
			if *showChecks {
				for _, check := range info.Checks {
					checkDefault := check.EnabledByDefault()
					if (*defaultsOnly && !checkDefault) || (*optInOnly && checkDefault) {
						continue
					}
					fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", check.ID, check.Profile, group.Name, joinTags(check.Tags))
				}
			} else {
				fmt.Fprintf(table, "%s\t%s\t%s\n", analyzer.Name, profile, group.Name)
			}
		}
	}
	return table.Flush()
}

func joinTags(tags []gohawk.AnalyzerTag) string {
	values := make([]string, len(tags))
	for index, tag := range tags {
		values[index] = string(tag)
	}
	return strings.Join(values, ",")
}

const analyzerDocumentationBaseURL = "https://kojah.github.io/gohawk/analyzers/"

func printDocumentation(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("doc", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	flags.Usage = func() {
		fmt.Fprintln(errorsOutput, "usage: gohawk doc ANALYZER|CHECK")
	}
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("expected one analyzer name or check ID")
	}

	target := flags.Arg(0)
	metadata := gohawk.AnalyzerMetadata()
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			info := metadata[analyzer.Name]
			if target == analyzer.Name {
				printAnalyzerDocumentation(output, group, analyzer, info)
				return nil
			}
			for _, check := range info.Checks {
				if target == string(check.ID) {
					printCheckDocumentation(output, group, analyzer, info, check)
					return nil
				}
			}
		}
	}
	return fmt.Errorf("unknown analyzer or check %q", target)
}

func printAnalyzerDocumentation(output io.Writer, group gohawk.AnalyzerGroup, analyzer *analysis.Analyzer, info gohawk.AnalyzerInfo) {
	fmt.Fprintln(output, analyzer.Name)
	fmt.Fprintf(output, "  %s\n\n", analyzer.Doc)
	fmt.Fprintf(output, "Profile: %s\n", info.Profile)
	fmt.Fprintf(output, "Group: %s (%s)\n", group.Name, group.Doc)
	fmt.Fprintf(output, "Suggested fixes: %s\n", yesNo(info.SuggestedFix))
	fmt.Fprintf(output, "Documentation: %s\n", analyzerDocumentationURL(group, analyzer.Name))
	fmt.Fprintln(output, "\nChecks:")
	for _, check := range info.Checks {
		fmt.Fprintf(output, "  %s\n", check.ID)
		fmt.Fprintf(output, "    %s\n", check.Doc)
		fmt.Fprintf(output, "    Profile: %s\n", check.Profile)
		fmt.Fprintf(output, "    Tags: %s\n", joinTags(check.Tags))
	}
	printAnalyzerOptions(output, analyzer)
}

func printCheckDocumentation(output io.Writer, group gohawk.AnalyzerGroup, analyzer *analysis.Analyzer, info gohawk.AnalyzerInfo, check gohawk.AnalyzerCheckInfo) {
	fmt.Fprintln(output, check.ID)
	fmt.Fprintf(output, "  %s\n\n", check.Doc)
	fmt.Fprintf(output, "Analyzer: %s\n", analyzer.Name)
	fmt.Fprintf(output, "Profile: %s\n", check.Profile)
	fmt.Fprintf(output, "Analyzer profile: %s\n", info.Profile)
	fmt.Fprintf(output, "Group: %s (%s)\n", group.Name, group.Doc)
	fmt.Fprintf(output, "Documentation: %s\n", analyzerDocumentationURL(group, analyzer.Name))
	fmt.Fprintln(output, "\nTags:")
	tagDescriptions := make(map[gohawk.AnalyzerTag]string)
	for _, tag := range gohawk.TagCatalog() {
		tagDescriptions[tag.ID] = tag.Description
	}
	for _, tag := range check.Tags {
		fmt.Fprintf(output, "  %s — %s\n", tag, tagDescriptions[tag])
	}
}

func printAnalyzerOptions(output io.Writer, analyzer *analysis.Analyzer) {
	var options []*flag.Flag
	analyzer.Flags.VisitAll(func(option *flag.Flag) {
		options = append(options, option)
	})
	if len(options) == 0 {
		return
	}
	fmt.Fprintln(output, "\nOptions:")
	for _, option := range options {
		fmt.Fprintf(output, "  -%s.%s (default %s)\n", analyzer.Name, option.Name, option.DefValue)
		fmt.Fprintf(output, "    %s\n", option.Usage)
	}
}

func analyzerDocumentationURL(group gohawk.AnalyzerGroup, analyzer string) string {
	return analyzerDocumentationBaseURL + group.DocPath + "/" + analyzer + "/"
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
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
			index++
			if index >= len(arguments) {
				return checkSelection{}, nil, fmt.Errorf("-%s requires a comma-separated value", name)
			}
			raw = arguments[index]
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
	if len(disabled) == 0 {
		return analyzers
	}
	result := make([]*analysis.Analyzer, 0, len(analyzers))
	for _, analyzer := range analyzers {
		analyzerDisabled := make(map[string]bool)
		for _, check := range metadata[analyzer.Name].Checks {
			if disabled[string(check.ID)] {
				analyzerDisabled[string(check.ID)] = true
			}
		}
		if len(analyzerDisabled) == 0 {
			result = append(result, analyzer)
			continue
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
				if !analyzerDisabled[diagnostic.Category] {
					report(diagnostic)
				}
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

func humanVersionRequested(arguments []string) bool {
	return len(arguments) == 2 && (arguments[1] == "-V" || arguments[1] == "--version")
}

func printHumanVersion(output io.Writer) {
	info, ok := debug.ReadBuildInfo()
	version := humanVersion(info, ok)
	fmt.Fprintln(output, "gohawk", version)
}

func humanVersion(info *debug.BuildInfo, ok bool) string {
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

func generalHelpRequested(arguments []string) bool {
	if len(arguments) < 2 {
		return false
	}
	switch arguments[1] {
	case "help", "-h", "--help", "-help":
		return len(arguments) == 2
	default:
		return false
	}
}

func printGeneralHelp(output io.Writer) {
	fmt.Fprintln(output, "usage: gohawk [selection flags] [analysis flags] package...")
	fmt.Fprintln(output, "\nAnalyzer selection:")
	fmt.Fprintln(output, "  -enable=NAME1,NAME2          run only named analyzers")
	fmt.Fprintln(output, "  -disable=NAME1,NAME2         remove analyzers from the default profile")
	fmt.Fprintln(output, "  -enable-checks=CHECK1,CHECK2 run only named checks, or add them to selected analyzers")
	fmt.Fprintln(output, "  -disable-checks=CHECK1,CHECK2 suppress checks by stable ID")
	fmt.Fprintln(output, "  -enable-groups=GROUP1,GROUP2 run every analyzer in named groups")
	fmt.Fprintln(output, "  -disable-groups=GROUP1,GROUP2 remove groups from the selected set")
	fmt.Fprintln(output, "  -enable-all                  run every analyzer and check")
	fmt.Fprintln(output, "\nCommon analysis flags: -json, -fix, -diff, -c=N, -V")
	fmt.Fprintln(output, "Run 'gohawk doc ANALYZER|CHECK' for metadata and documentation.")
	fmt.Fprintln(output, "Run 'gohawk help ANALYZER' for an analyzer's configuration flags.")
	fmt.Fprintln(output, "\nAnalyzer groups:")
	metadata := gohawk.AnalyzerMetadata()
	for _, group := range gohawk.AnalyzerGroups() {
		names := make([]string, 0, len(group.Analyzers))
		for _, analyzer := range group.Analyzers {
			name := analyzer.Name
			if !metadata[analyzer.Name].EnabledByDefault() {
				name += " (opt-in)"
			}
			names = append(names, name)
		}
		fmt.Fprintf(output, "  %s (%s): %s\n", group.Name, group.Doc, strings.Join(names, ", "))
	}
	fmt.Fprintln(output, "\nRun 'gohawk list' for the full catalog and default status.")
	fmt.Fprintln(output)
}
