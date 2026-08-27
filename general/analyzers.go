package general

import (
	"fmt"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

type commaSeparatedChoice struct {
	value   *string
	allowed map[string]bool
}

type choiceValue struct {
	value   *string
	allowed map[string]bool
}

func newChoiceValue(value *string, allowed ...string) *choiceValue {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &choiceValue{value: value, allowed: choices}
}

func (choice *choiceValue) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *choiceValue) Set(value string) error {
	if !choice.allowed[value] {
		return fmt.Errorf("unknown value %q", value)
	}
	*choice.value = value
	return nil
}

func newCommaSeparatedChoice(value *string, allowed ...string) *commaSeparatedChoice {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &commaSeparatedChoice{value: value, allowed: choices}
}

func (choice *commaSeparatedChoice) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *commaSeparatedChoice) Set(value string) error {
	for item := range commaSeparatedSet(value) {
		if !choice.allowed[item] {
			return fmt.Errorf("unknown value %q", item)
		}
	}
	*choice.value = value
	return nil
}

func commaSeparatedSet(value string) map[string]bool {
	result := make(map[string]bool)
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = true
		}
	}
	return result
}

// AnalyzerGroup is a related set of Go policy analyzers.
type AnalyzerGroup struct {
	Name      string
	Doc       string
	Analyzers []*analysis.Analyzer
}

// AnalyzerInfo describes capabilities that are not represented by analysis.Analyzer.
type AnalyzerInfo struct {
	SuggestedFix bool
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	metadata := make(map[string]AnalyzerInfo)
	for _, group := range AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			metadata[analyzer.Name] = AnalyzerInfo{}
		}
	}
	for _, name := range []string{"cancellationownership", "testpolicy", "wirepolicy"} {
		metadata[name] = AnalyzerInfo{SuggestedFix: true}
	}
	return metadata
}

// AnalyzerGroups returns framework-neutral Go policy analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	groups := []AnalyzerGroup{
		{
			Name: "contracts",
			Doc:  "API and data contracts",
			Analyzers: []*analysis.Analyzer{
				apiShapeAnalyzer(),
				contextPolicyAnalyzer(),
				closedDomainAnalyzer(),
				wirePolicyAnalyzer(),
			},
		},
		{
			Name: "ownership",
			Doc:  "ownership and lifecycle",
			Analyzers: []*analysis.Analyzer{
				cancellationOwnershipAnalyzer(),
				channelPolicyAnalyzer(),
				deferInLoopAnalyzer(),
				goroutineOwnershipAnalyzer(),
				processOwnershipAnalyzer(),
				resourceLifetimeAnalyzer(),
			},
		},
		{
			Name: "reliability",
			Doc:  "reliability and safety",
			Analyzers: []*analysis.Analyzer{
				concurrentCaptureAnalyzer(),
				determinismAnalyzer(),
				errorOwnershipAnalyzer(),
				globalStateAnalyzer(),
				lockOrderAnalyzer(),
				taintPolicyAnalyzer(),
			},
		},
		{
			Name: "testing",
			Doc:  "testing",
			Analyzers: []*analysis.Analyzer{
				blockingTestAnalyzer(),
				testPolicyAnalyzer(),
			},
		},
	}
	for groupIndex := range groups {
		for analyzerIndex, analyzer := range groups[groupIndex].Analyzers {
			groups[groupIndex].Analyzers[analyzerIndex] = withSuppressions(analyzer)
		}
	}
	return groups
}

func withSuppressions(analyzer *analysis.Analyzer) *analysis.Analyzer {
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if !analysisutil.DiagnosticSuppressed(pass, diagnostic.Pos, analyzer.Name) {
				report(diagnostic)
			}
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return analyzer
}

// Analyzers returns all framework-neutral Go policy analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	groups := AnalyzerGroups()
	names := []string{
		"apishape",
		"contextpolicy",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"blockingtest",
		"goroutineownership",
		"errorownership",
		"channelpolicy",
		"processownership",
		"closedomain",
		"taintpolicy",
		"lockorder",
		"resourcelifetime",
		"deferinloop",
		"determinism",
		"concurrentcapture",
		"cancellationownership",
	}
	analyzers := make([]*analysis.Analyzer, 0, len(names))
	for _, name := range names {
	findAnalyzer:
		for _, group := range groups {
			for _, analyzer := range group.Analyzers {
				if analyzer.Name == name {
					analyzers = append(analyzers, analyzer)
					break findAnalyzer
				}
			}
		}
	}
	return analyzers
}
