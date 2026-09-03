// Package cli implements the gohawk command.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"slices"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/unitchecker"
)

const filteredFlagsChild = "GOHAWK_FILTERED_FLAGS_CHILD"

type cliRuntime struct {
	output        io.Writer
	errorsOutput  io.Writer
	getenv        func(string) string
	filteredFlags func([]string, []*analysis.Analyzer, io.Writer, io.Writer) int
	execute       processExecutor
	executable    func() (string, error)
}

// renderMode is how a delegated analysis run presents go vet's JSON output.
type renderMode int

const (
	renderRich renderMode = iota // gohawk's own formatted diagnostics
	renderJSON                   // the raw go vet JSON, passed through
	renderFix                    // apply or preview suggested fixes
)

// analysisInvocation is the analysis work runCLI resolved but did not run. A
// delegate invocation runs the analyzers through `go vet -vettool`, which
// analyzes one package at a time with bounded memory; a non-delegate
// invocation is a vet-tool handshake (a .cfg unit, a flag or version query)
// that the unitchecker driver answers in this process.
type analysisInvocation struct {
	arguments    []string
	analyzers    []*analysis.Analyzer
	delegate     bool
	render       renderMode
	diff         bool
	contextLines int
}

type cliResult struct {
	exitCode   int
	invocation *analysisInvocation
}

// Main runs the gohawk command and returns its exit code.
//
// gohawk does not analyze a whole program in one process: a standalone run
// delegates to `go vet -vettool=<self>`, so dependencies are type-checked from
// export data and each package's SSA is built and freed one at a time. That
// keeps memory bounded on projects with large dependencies, where loading the
// entire closure at once would exhaust it. The rich output, check selection,
// and suggested fixes are preserved by post-processing go vet's JSON here. When
// go vet invokes this same binary as its tool, the invocation carries a .cfg
// file and is answered by unitchecker instead.
func Main() int {
	runtime := cliRuntime{
		output:        os.Stdout,
		errorsOutput:  os.Stderr,
		getenv:        os.Getenv,
		filteredFlags: printFilteredFlags,
		execute:       executeProcess,
		executable:    os.Executable,
	}
	result := runCLI(os.Args, runtime)
	if result.invocation == nil {
		return result.exitCode
	}
	if result.invocation.delegate {
		return runViaGoVet(result.invocation, runtime)
	}
	// A vet-tool handshake: unitchecker owns os.Exit, so this returns only in
	// tests that stub the process boundary.
	registerSelectionFlags()
	analyzers, arguments := unitAnalyzers(result.invocation.arguments, result.invocation.analyzers)
	os.Args = arguments
	unitchecker.Main(analyzers...)
	return 0
}

// unitAnalyzers narrows the analyzers run on a dependency unit to the fact
// producers. go vet analyzes a dependency package only for the facts it
// exports and marks that unit VetxOnly; unitchecker keeps every analyzer that
// transitively requires a fact producer, which pulls in resourcelifetime and
// processownership and runs them in full only to discard their diagnostics.
// The one thing a dependency contributes upward is facts, so running the
// fact-producer closure and nothing else leaves every diagnostic on the
// analyzed source identical while skipping that work. On a Kubernetes tree it
// removes several CPU-minutes per run.
func unitAnalyzers(arguments []string, analyzers []*analysis.Analyzer) ([]*analysis.Analyzer, []string) {
	if !factsOnlyUnit(arguments) {
		return analyzers, arguments
	}
	keep := factProducerClosure(analyzers)
	// unitchecker runs whatever it is given plus their prerequisites, so the
	// kept set is the closure itself, which includes the fact producers that
	// were only reachable through the dropped consumers.
	kept := make([]*analysis.Analyzer, 0, len(keep))
	for analyzer := range keep {
		kept = append(kept, analyzer)
	}
	slices.SortFunc(kept, func(left, right *analysis.Analyzer) int {
		return strings.Compare(left.Name, right.Name)
	})
	dropped := make(map[string]bool, len(analyzers))
	for _, analyzer := range analyzers {
		if !keep[analyzer] {
			dropped[analyzer.Name] = true
		}
	}
	// unitchecker registers one Boolean flag per analyzer it is given, and go
	// vet forwards gohawk's per-analyzer selection flags to every unit, so the
	// flag of a dropped analyzer would be undefined. Drop those flags too.
	filtered := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if name, ok := flagName(argument); ok && dropped[name] {
			continue
		}
		filtered = append(filtered, argument)
	}
	return kept, filtered
}

// factProducerClosure returns the analyzers that must run on a facts-only
// unit: every analyzer reachable through Requires that exports a fact type,
// together with the prerequisites those producers need. lifecyclefacts is
// itself reachable only as a prerequisite of its consumers, so the walk must
// follow Requires rather than inspect the selected list alone.
func factProducerClosure(selected []*analysis.Analyzer) map[*analysis.Analyzer]bool {
	reachable := map[*analysis.Analyzer]bool{}
	var visit func(*analysis.Analyzer)
	visit = func(analyzer *analysis.Analyzer) {
		if reachable[analyzer] {
			return
		}
		reachable[analyzer] = true
		for _, required := range analyzer.Requires {
			visit(required)
		}
	}
	for _, analyzer := range selected {
		visit(analyzer)
	}
	keep := map[*analysis.Analyzer]bool{}
	var require func(*analysis.Analyzer)
	require = func(analyzer *analysis.Analyzer) {
		if keep[analyzer] {
			return
		}
		keep[analyzer] = true
		for _, prerequisite := range analyzer.Requires {
			require(prerequisite)
		}
	}
	for analyzer := range reachable {
		if len(analyzer.FactTypes) > 0 {
			require(analyzer)
		}
	}
	return keep
}

// factsOnlyUnit reports whether the vet-tool arguments name a .cfg unit that
// is analyzed only for the facts it exports and never for user-facing
// diagnostics. That is true for an external dependency go vet marked
// VetxOnly. It is deliberately NOT true for a first-party package, because go
// vet analyzes a first-party package both as a VetxOnly dependency of its
// siblings and as a root that produces diagnostics; dropping the diagnostic
// analyzers on its VetxOnly pass would lose the finding its root pass reports.
// First-party source lives under the working tree; a dependency's source
// lives under the module cache or GOROOT.
func factsOnlyUnit(arguments []string) bool {
	for _, argument := range arguments[1:] {
		if !strings.HasSuffix(argument, ".cfg") {
			continue
		}
		data, err := os.ReadFile(argument) //nolint:gosec // go vet supplies the path of the unit it is asking about.
		if err != nil {
			return false
		}
		var unit struct {
			VetxOnly bool     `json:"VetxOnly"`
			GoFiles  []string `json:"GoFiles"`
		}
		if json.Unmarshal(data, &unit) != nil || !unit.VetxOnly {
			return false
		}
		return externalDependencySources(unit.GoFiles)
	}
	return false
}

// externalDependencySources reports whether every source file lives outside
// the working tree, under the module cache or GOROOT. A single file under the
// working tree marks the package first-party, so it is not filtered.
func externalDependencySources(files []string) bool {
	if len(files) == 0 {
		return false
	}
	working, err := os.Getwd()
	if err != nil {
		return false
	}
	for _, file := range files {
		if strings.HasPrefix(file, working+string(os.PathSeparator)) {
			return false
		}
	}
	return true
}

func runCLI(arguments []string, runtime cliRuntime) cliResult {
	if humanVersionRequested(arguments) {
		printHumanVersion(runtime.output)
		return cliResult{}
	}
	originalArguments := slices.Clone(arguments)
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
	// Validate selection in this process so a bad name is reported once, before
	// any delegation, rather than once per package by the vet-tool children.
	plan, err := buildExecutionPlan(arguments, analyzers, gohawk.AnalyzerGroups(), metadata, false)
	if err != nil {
		writeLine(runtime.errorsOutput, "gohawk:", err)
		return cliResult{exitCode: 2}
	}
	if vetToolHandshake(originalArguments) {
		return cliResult{invocation: &analysisInvocation{arguments: plan.arguments, analyzers: plan.analyzers}}
	}
	// go vet relays an analyzer option flag to every package, so a bad value
	// would be reported once per package with a usage banner. Validate it here
	// so a typo fails once, cleanly, before any delegation.
	if err := validateAnalyzerOptionFlags(analyzers, originalArguments); err != nil {
		writeLine(runtime.errorsOutput, "gohawk:", err)
		return cliResult{exitCode: 2}
	}
	return cliResult{invocation: &analysisInvocation{
		delegate:     true,
		arguments:    forwardedArguments(originalArguments),
		render:       renderModeFor(originalArguments),
		diff:         hasFlag(originalArguments, "diff"),
		contextLines: requestedContext(originalArguments),
	}}
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
	case "facts":
		err = printFacts(arguments[2:], runtime.output, runtime.errorsOutput)
	default:
		return cliResult{}, false
	}
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return cliResult{}, true
	}
	writeLine(runtime.errorsOutput, "gohawk "+arguments[1]+":", err)
	return cliResult{exitCode: 2}, true
}

// vetToolHandshake reports whether arguments are a request go vet makes of its
// tool rather than a request to analyze packages: a .cfg unit to analyze, a
// machine-readable flag list, or a version query. These are answered by
// unitchecker in this process; everything else delegates to go vet.
func vetToolHandshake(arguments []string) bool {
	for _, argument := range arguments[1:] {
		if strings.HasSuffix(argument, ".cfg") {
			return true
		}
		if name, ok := flagName(argument); ok && (name == "flags" || name == "V") {
			return true
		}
	}
	return false
}

func renderModeFor(arguments []string) renderMode {
	switch {
	case hasFlag(arguments, "fix") || hasFlag(arguments, "diff"):
		return renderFix
	case hasFlag(arguments, "json"):
		return renderJSON
	default:
		return renderRich
	}
}

// forwardedArguments are the flags and package patterns handed to go vet. The
// flags gohawk interprets itself and does not want go vet to see are dropped:
// -json, -fix, and -diff select how this process post-processes the JSON, and
// -c is the context width gohawk applies while rendering. Selection, analyzer,
// and trace flags are forwarded unchanged; go vet relays them to each vet-tool
// invocation, which resolves them.
func forwardedArguments(arguments []string) []string {
	forwarded := make([]string, 0, len(arguments))
	skipValue := false
	for _, argument := range arguments[1:] {
		if skipValue {
			skipValue = false
			continue
		}
		if name, ok := flagName(argument); ok {
			switch name {
			case "json", "fix", "diff":
				continue
			case "c":
				skipValue = !strings.Contains(argument, "=") // a bare -c takes a separate value
				continue
			}
		}
		forwarded = append(forwarded, argument)
	}
	return forwarded
}

// flagName returns the flag name of a -flag or --flag=value argument.
func flagName(argument string) (string, bool) {
	if len(argument) < 2 || argument[0] != '-' {
		return "", false
	}
	name := strings.TrimLeft(argument, "-")
	name, _, _ = strings.Cut(name, "=")
	return name, name != ""
}

func hasFlag(arguments []string, name string) bool {
	for _, argument := range arguments {
		if candidate, ok := flagName(argument); ok && candidate == name {
			return true
		}
	}
	return false
}

// validateAnalyzerOptionFlags parses the analyzer option flags (named
// analyzer.flag) the user passed, using each analyzer's own flag value, so an
// invalid value is rejected with the same message the analysis driver would
// give. Other flags are ignored here; go vet validates them against the tool's
// advertised flag set.
func validateAnalyzerOptionFlags(analyzers []*analysis.Analyzer, arguments []string) error {
	set := flag.NewFlagSet("gohawk", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	known := map[string]bool{}
	for _, analyzer := range analyzers {
		analyzer.Flags.VisitAll(func(option *flag.Flag) {
			name := analyzer.Name + "." + option.Name
			set.Var(option.Value, name, option.Usage)
			known[name] = true
		})
	}
	var relevant []string
	for _, argument := range arguments[1:] {
		if name, ok := flagName(argument); ok && known[name] {
			relevant = append(relevant, argument)
		}
	}
	return set.Parse(relevant)
}
