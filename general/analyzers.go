package general

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/general/contracts"
	"github.com/kojah/gohawk/general/ownership"
	"github.com/kojah/gohawk/general/reliability"
	testingchecks "github.com/kojah/gohawk/general/testing"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of Go policy analyzers.
type AnalyzerGroup struct {
	Name      string
	Doc       string
	DocPath   string
	Analyzers []*analysis.Analyzer
}

// AnalyzerProfile controls whether an analyzer runs without explicit selection.
type AnalyzerProfile string

const (
	AnalyzerProfileDefault AnalyzerProfile = "default"
	AnalyzerProfileOptIn   AnalyzerProfile = "opt-in"
)

// AnalyzerTag identifies a reason that a check's findings matter.
type AnalyzerTag string

const (
	AnalyzerTagCorrectness AnalyzerTag = "correctness"
	AnalyzerTagReliability AnalyzerTag = "reliability"
	AnalyzerTagPolicy      AnalyzerTag = "policy"
)

// TagInfo describes one check tag.
type TagInfo struct {
	ID          AnalyzerTag
	Description string
}

var tagCatalog = []TagInfo{
	{ID: AnalyzerTagCorrectness, Description: "Strong evidence that the program can behave incorrectly."},
	{ID: AnalyzerTagReliability, Description: "Code that may work but is vulnerable to meaningful lifecycle, concurrency, or operational failures."},
	{ID: AnalyzerTagPolicy, Description: "A project convention on which reasonable teams may differ."},
}

// TagCatalog returns every check tag in stable presentation order.
func TagCatalog() []TagInfo {
	return slices.Clone(tagCatalog)
}

// AnalyzerInfo describes capabilities that are not represented by analysis.Analyzer.
type AnalyzerInfo struct {
	Profile      AnalyzerProfile
	Checks       []AnalyzerCheckInfo
	SuggestedFix bool
}

// EnabledByDefault reports whether the analyzer belongs to the default profile.
func (info AnalyzerInfo) EnabledByDefault() bool {
	return info.Profile == AnalyzerProfileDefault
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	metadata := make(map[string]AnalyzerInfo)
	for _, group := range AnalyzerGroups() {
		for _, analyzer := range group.Analyzers {
			checks := cloneChecks(analyzerChecks[analyzer.Name])
			metadata[analyzer.Name] = AnalyzerInfo{
				Profile: AnalyzerProfileDefault,
				Checks:  checks,
			}
		}
	}
	for _, name := range []string{"apishape", "blockingtest", "closedomain", "determinism", "globalstate", "taintpolicy", "testpolicy", "wirepolicy"} {
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
			Name:      "contracts",
			Doc:       "API and data contracts",
			DocPath:   "api-and-data-contracts",
			Analyzers: contracts.Analyzers(),
		},
		{
			Name:      "ownership",
			Doc:       "ownership and lifecycle",
			DocPath:   "ownership-and-lifecycle",
			Analyzers: ownership.Analyzers(),
		},
		{
			Name:      "reliability",
			Doc:       "reliability and safety",
			DocPath:   "reliability-and-safety",
			Analyzers: reliability.Analyzers(),
		},
		{
			Name:      "testing",
			Doc:       "testing",
			DocPath:   "testing",
			Analyzers: testingchecks.Analyzers(),
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
	checks := make(map[string]bool, len(analyzerChecks[analyzer.Name]))
	for _, check := range analyzerChecks[analyzer.Name] {
		checks[string(check.ID)] = true
	}
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if !checks[diagnostic.Category] {
				panic(fmt.Sprintf("analyzer %q reported unknown check %q", analyzer.Name, diagnostic.Category))
			}
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
