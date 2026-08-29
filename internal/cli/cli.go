// Package cli implements the gohawk command.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

// Main runs the gohawk command and exits with its result.
func Main() {
	normalizeVersionFlag(os.Args)
	originalArguments := append([]string(nil), os.Args...)
	if len(os.Args) > 1 && os.Args[1] == "list" {
		if err := printAnalyzerList(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintln(os.Stderr, "gohawk list:", err)
			os.Exit(2)
		}
		return
	}
	flag.Bool("enable-all", false, "enable every analyzer, including opt-in analyzers")
	if generalHelpRequested(os.Args) {
		printAnalyzerGroups()
	}
	analyzers := gohawk.Analyzers()
	os.Args = withDefaultAnalyzerSelection(os.Args, analyzers, gohawk.AnalyzerMetadata())
	if useRichOutput(originalArguments) {
		os.Exit(runWithRichOutput(os.Args, os.Stderr))
	}
	multichecker.Main(analyzers...)
}

func printAnalyzerList(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	defaultsOnly := flags.Bool("defaults", false, "show only analyzers enabled by default")
	optInOnly := flags.Bool("opt-in", false, "show only opt-in analyzers")
	flags.Usage = func() {
		fmt.Fprintln(errorsOutput, "usage: gohawk list [-defaults | -opt-in]")
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
	fmt.Fprintln(table, "ANALYZER\tPROFILE\tTAGS\tCATEGORY")
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			info := metadata[analyzer.Name]
			isDefault := info.EnabledByDefault()
			if (*defaultsOnly && !isDefault) || (*optInOnly && isDefault) {
				continue
			}
			profile := "default"
			if !isDefault {
				profile = "opt-in"
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", analyzer.Name, profile, joinTags(info.Tags), group.Doc)
		}
	}
	return table.Flush()
}

func withDefaultAnalyzerSelection(arguments []string, analyzers []*analysis.Analyzer, metadata map[string]gohawk.AnalyzerInfo) []string {
	if (len(arguments) > 1 && arguments[1] == "help") || enableAllRequested(arguments) {
		return arguments
	}
	names := make(map[string]bool, len(analyzers))
	for _, analyzer := range analyzers {
		names[analyzer.Name] = true
	}
	disabled := make(map[string]bool)
	for _, argument := range arguments[1:] {
		name, enabled, ok := analyzerSelection(argument, names)
		if !ok {
			continue
		}
		if enabled {
			return arguments
		}
		disabled[name] = true
	}
	defaults := make([]string, 0, len(analyzers))
	for _, analyzer := range analyzers {
		if metadata[analyzer.Name].EnabledByDefault() && !disabled[analyzer.Name] {
			defaults = append(defaults, "-"+analyzer.Name+"=true")
		}
	}
	result := make([]string, 0, len(arguments)+len(defaults))
	result = append(result, arguments[0])
	result = append(result, defaults...)
	return append(result, arguments[1:]...)
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

func normalizeVersionFlag(arguments []string) {
	for index, argument := range arguments {
		if argument == "-V" {
			arguments[index] = "-V=full"
		}
	}
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

func printAnalyzerGroups() {
	fmt.Fprintln(os.Stderr, "gohawk analyzer groups (select checks with individual -NAME flags):")
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
		fmt.Fprintf(os.Stderr, "  %s (%s): %s\n", group.Name, group.Doc, strings.Join(names, ", "))
	}
	fmt.Fprintln(os.Stderr, "\nRun 'gohawk list' for the full catalog and default status.")
	fmt.Fprintln(os.Stderr)
}

func joinTags(tags []gohawk.AnalyzerTag) string {
	values := make([]string, len(tags))
	for index, tag := range tags {
		values[index] = string(tag)
	}
	return strings.Join(values, ",")
}
