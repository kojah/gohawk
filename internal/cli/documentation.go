package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
)

type advertisedFlag struct {
	Name  string `json:"Name"`
	Bool  bool   `json:"Bool"`
	Usage string `json:"Usage"`
}

type analyzerListOptions struct {
	defaultsOnly bool
	optInOnly    bool
	showChecks   bool
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
		writeLine(errorsOutput, "gohawk: inspect analyzer flags:", err)
		return 1
	}
	var flags []advertisedFlag
	if err := json.Unmarshal(result.stdout, &flags); err != nil {
		writeLine(errorsOutput, "gohawk: decode analyzer flags:", err)
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
		writeLine(errorsOutput, "gohawk: encode analyzer flags:", err)
		return 1
	}
	_, _ = output.Write(encoded)
	return 0
}

func printAnalyzerList(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	defaultsOnly := flags.Bool("defaults", false, "show only entries included in an ordinary run")
	optInOnly := flags.Bool("opt-in", false, "show only entries requiring explicit selection")
	showChecks := flags.Bool("checks", false, "show stable check IDs instead of analyzer names")
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk list [-checks] [-defaults | -opt-in]")
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
	options := analyzerListOptions{
		defaultsOnly: *defaultsOnly,
		optInOnly:    *optInOnly,
		showChecks:   *showChecks,
	}

	metadata := gohawk.AnalyzerMetadata()
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if options.showChecks {
		writeLine(table, "CHECK\tKIND\tGROUP")
	} else {
		writeLine(table, "ANALYZER\tGROUP")
	}
	shownOptIn := writeAnalyzerListRows(table, metadata, options)
	if err := table.Flush(); err != nil {
		return err
	}
	if shownOptIn {
		writeLine(output, "\n* opt-in; requires explicit selection")
	}
	return nil
}

func writeAnalyzerListRows(
	output io.Writer,
	metadata map[string]gohawk.AnalyzerInfo,
	options analyzerListOptions,
) bool {
	shownOptIn := false
	for _, group := range gohawk.AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			info := metadata[analyzer.Name]
			isDefault := info.EnabledByDefault()
			if !options.showChecks && listEntryFiltered(isDefault, options) {
				continue
			}
			if options.showChecks {
				shownOptIn = writeCheckListRows(output, group.Name, info, options) || shownOptIn
				continue
			}
			writeFormattedf(output, "%s\t%s\n", optInName(analyzer.Name, !isDefault), group.Name)
			shownOptIn = shownOptIn || !isDefault
		}
	}
	return shownOptIn
}

func writeCheckListRows(output io.Writer, groupName string, info gohawk.AnalyzerInfo, options analyzerListOptions) bool {
	shownOptIn := false
	for _, check := range info.Checks {
		isDefault := info.EnabledByDefault() && check.EnabledByDefault()
		if listEntryFiltered(isDefault, options) {
			continue
		}
		writeFormattedf(output, "%s\t%s\t%s\n", optInName(string(check.ID), !isDefault), check.Kind, groupName)
		shownOptIn = shownOptIn || !isDefault
	}
	return shownOptIn
}

func listEntryFiltered(isDefault bool, options analyzerListOptions) bool {
	return options.defaultsOnly && !isDefault || options.optInOnly && isDefault
}

func optInName(name string, optIn bool) string {
	if optIn {
		return name + "*"
	}
	return name
}

const analyzerDocumentationBaseURL = "https://gohawk.dev/analyzers/"

func printDocumentation(arguments []string, output, errorsOutput io.Writer) error {
	flags := flag.NewFlagSet("doc", flag.ContinueOnError)
	flags.SetOutput(errorsOutput)
	flags.Usage = func() {
		writeLine(errorsOutput, "usage: gohawk doc ANALYZER|CHECK")
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
	writeLine(output, analyzer.Name)
	writeFormattedf(output, "  %s\n\n", analyzer.Doc)
	if info.OptIn {
		writeLine(output, "Opt-in: yes")
	}
	writeFormattedf(output, "Group: %s (%s)\n", group.Name, group.Doc)
	writeFormattedf(output, "Suggested fixes: %s\n", yesNo(info.SuggestedFix))
	writeFormattedf(output, "Documentation: %s\n", analyzerDocumentationURL(group, analyzer.Name))
	writeLine(output, "\nChecks:")
	for _, check := range info.Checks {
		writeFormattedf(output, "  %s\n", optInName(string(check.ID), info.OptIn || check.OptIn))
		writeFormattedf(output, "    Kind: %s\n", check.Kind)
		writeFormattedf(output, "    %s\n", check.Doc)
	}
	if info.OptIn || slices.ContainsFunc(info.Checks, func(check gohawk.AnalyzerCheckInfo) bool { return check.OptIn }) {
		writeLine(output, "  * opt-in; requires explicit selection")
	}
	printAnalyzerOptions(output, analyzer)
}

func printCheckDocumentation(
	output io.Writer,
	group gohawk.AnalyzerGroup,
	analyzer *analysis.Analyzer,
	info gohawk.AnalyzerInfo,
	check gohawk.AnalyzerCheckInfo,
) {
	writeLine(output, check.ID)
	writeFormattedf(output, "  %s\n\n", check.Doc)
	writeFormattedf(output, "Analyzer: %s\n", analyzer.Name)
	writeFormattedf(output, "Kind: %s\n", check.Kind)
	if info.OptIn || check.OptIn {
		writeLine(output, "Opt-in: yes")
	}
	writeFormattedf(output, "Group: %s (%s)\n", group.Name, group.Doc)
	writeFormattedf(output, "Documentation: %s\n", analyzerDocumentationURL(group, analyzer.Name))
}

func printAnalyzerOptions(output io.Writer, analyzer *analysis.Analyzer) {
	var options []*flag.Flag
	analyzer.Flags.VisitAll(func(option *flag.Flag) {
		options = append(options, option)
	})
	if len(options) == 0 {
		return
	}
	writeLine(output, "\nOptions:")
	for _, option := range options {
		writeFormattedf(output, "  -%s.%s (default %s)\n", analyzer.Name, option.Name, option.DefValue)
		writeFormattedf(output, "    %s\n", option.Usage)
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

func humanVersionRequested(arguments []string) bool {
	return len(arguments) == 2 && (arguments[1] == "-V" || arguments[1] == "--version")
}

func printHumanVersion(output io.Writer) {
	info, ok := debug.ReadBuildInfo()
	version := humanVersion(info, ok)
	writeLine(output, "gohawk", version)
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
	writeLine(output, "usage: gohawk [selection flags] [analysis flags] package...")
	writeLine(output, "\nAnalyzer selection:")
	writeLine(output, "  -enable=NAME1,NAME2          run only named analyzers")
	writeLine(output, "  -disable=NAME1,NAME2         remove analyzers from the ordinary run")
	writeLine(output, "  -enable-checks=CHECK1,CHECK2 run only named checks, or add them to selected analyzers")
	writeLine(output, "  -disable-checks=CHECK1,CHECK2 suppress checks by stable ID")
	writeLine(output, "  -enable-groups=GROUP1,GROUP2 run every analyzer in named groups")
	writeLine(output, "  -disable-groups=GROUP1,GROUP2 remove groups from the selected set")
	writeLine(output, "  -enable-all                  run every analyzer and check")
	writeLine(output, "\nCommon analysis flags: -json, -fix, -diff, -c=N, -V")
	writeLine(output, "Test files: -gohawk-include-tests reports findings in _test.go files for analyzers that do not target tests")
	writeLine(output, "Evidence tracing: -gohawk-trace=ANALYZER[,CHECK...] [-gohawk-trace-file=PATH]")
	writeLine(output, "Run 'gohawk doc ANALYZER|CHECK' for metadata and documentation.")
	writeLine(output, "Run 'gohawk help ANALYZER' for an analyzer's configuration flags.")
	writeLine(output, "\nAnalyzer groups:")
	metadata := gohawk.AnalyzerMetadata()
	for _, group := range gohawk.AnalyzerGroups() {
		names := make([]string, 0, len(group.Analyzers))
		for _, analyzer := range group.Analyzers {
			names = append(names, optInName(analyzer.Name, !metadata[analyzer.Name].EnabledByDefault()))
		}
		writeFormattedf(output, "  %s (%s): %s\n", group.Name, group.Doc, strings.Join(names, ", "))
	}
	writeLine(output, "\n* opt-in; requires explicit selection")
	writeLine(output, "Run 'gohawk list' for the full catalog.")
	writeLine(output)
}
