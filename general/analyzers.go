package general

import (
	"fmt"
	"slices"
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

// AnalyzerProfile controls whether an analyzer runs without explicit selection.
type AnalyzerProfile string

const (
	AnalyzerProfileDefault AnalyzerProfile = "default"
	AnalyzerProfileOptIn   AnalyzerProfile = "opt-in"
)

// AnalyzerTag describes the reason an analyzer's findings matter. Tags are
// composable because one analyzer may report several related kinds of finding.
type AnalyzerTag string

const (
	AnalyzerTagCorrectness AnalyzerTag = "correctness"
	AnalyzerTagReliability AnalyzerTag = "reliability"
	AnalyzerTagPolicy      AnalyzerTag = "policy"
)

// AnalyzerInfo describes capabilities that are not represented by analysis.Analyzer.
type AnalyzerInfo struct {
	Profile      AnalyzerProfile
	Tags         []AnalyzerTag
	SuggestedFix bool
}

// EnabledByDefault reports whether the analyzer belongs to the default profile.
func (info AnalyzerInfo) EnabledByDefault() bool {
	return info.Profile == AnalyzerProfileDefault
}

var analyzerTags = map[string][]AnalyzerTag{
	"apishape":              {AnalyzerTagPolicy},
	"contextpolicy":         {AnalyzerTagCorrectness, AnalyzerTagReliability, AnalyzerTagPolicy},
	"closedomain":           {AnalyzerTagReliability, AnalyzerTagPolicy},
	"wirepolicy":            {AnalyzerTagReliability, AnalyzerTagPolicy},
	"cancellationownership": {AnalyzerTagCorrectness},
	"channelpolicy":         {AnalyzerTagCorrectness, AnalyzerTagReliability, AnalyzerTagPolicy},
	"deferinloop":           {AnalyzerTagReliability},
	"exitpolicy":            {AnalyzerTagCorrectness},
	"goroutineownership":    {AnalyzerTagReliability},
	"processownership":      {AnalyzerTagCorrectness},
	"resourcelifetime":      {AnalyzerTagCorrectness},
	"concurrentcapture":     {AnalyzerTagCorrectness},
	"determinism":           {AnalyzerTagReliability},
	"errorownership":        {AnalyzerTagCorrectness, AnalyzerTagReliability},
	"evalorder":             {AnalyzerTagCorrectness},
	"globalstate":           {AnalyzerTagReliability, AnalyzerTagPolicy},
	"lockorder":             {AnalyzerTagCorrectness},
	"oncepolicy":            {AnalyzerTagCorrectness},
	"syncmapatomicity":      {AnalyzerTagCorrectness},
	"taintpolicy":           {AnalyzerTagCorrectness, AnalyzerTagReliability},
	"blockingtest":          {AnalyzerTagReliability},
	"testpolicy":            {AnalyzerTagPolicy},
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	metadata := make(map[string]AnalyzerInfo)
	for _, group := range AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			metadata[analyzer.Name] = AnalyzerInfo{
				Profile: AnalyzerProfileDefault,
				Tags:    slices.Clone(analyzerTags[analyzer.Name]),
			}
		}
	}
	for _, name := range []string{"apishape", "closedomain", "globalstate", "taintpolicy", "testpolicy", "wirepolicy"} {
		info := metadata[name]
		info.Profile = AnalyzerProfileOptIn
		metadata[name] = info
	}
	for _, name := range []string{"cancellationownership", "testpolicy", "wirepolicy"} {
		info := metadata[name]
		info.SuggestedFix = true
		metadata[name] = info
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
				exitPolicyAnalyzer(),
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
				evalOrderAnalyzer(),
				globalStateAnalyzer(),
				lockOrderAnalyzer(),
				oncePolicyAnalyzer(),
				syncMapAtomicityAnalyzer(),
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
		"exitpolicy",
		"determinism",
		"concurrentcapture",
		"evalorder",
		"oncepolicy",
		"syncmapatomicity",
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

// DefaultAnalyzers returns the analyzers enabled when no analyzer selection
// flags are provided.
func DefaultAnalyzers() []*analysis.Analyzer {
	metadata := AnalyzerMetadata()
	return slices.DeleteFunc(Analyzers(), func(analyzer *analysis.Analyzer) bool {
		return !metadata[analyzer.Name].EnabledByDefault()
	})
}
