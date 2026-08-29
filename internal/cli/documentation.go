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
