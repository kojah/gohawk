package general

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/analysisutil"
	"github.com/kojah/gohawk/general/contracts"
	"github.com/kojah/gohawk/general/ownership"
	"github.com/kojah/gohawk/general/reliability"
	testingchecks "github.com/kojah/gohawk/general/testing"
	"github.com/kojah/gohawk/internal/analyzerbase"

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

// TagCatalog returns every check tag in stable presentation order.
func TagCatalog() []TagInfo {
	tags := analyzerbase.Tags()
	result := make([]TagInfo, len(tags))
	for index, tag := range tags {
		result[index] = TagInfo{ID: AnalyzerTag(tag.ID), Description: tag.Description}
	}
	return result
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

func newCatalog() *analyzerbase.Catalog {
	catalog, err := analyzerbase.NewCatalog([]analyzerbase.GroupSpec{
		{ID: "contracts", Doc: "API and data contracts", DocPath: "api-and-data-contracts", Analyzers: contracts.Specs()},
		{ID: "ownership", Doc: "ownership and lifecycle", DocPath: "ownership-and-lifecycle", Analyzers: ownership.Specs()},
		{ID: "reliability", Doc: "reliability and safety", DocPath: "reliability-and-safety", Analyzers: reliability.Specs()},
		{ID: "testing", Doc: "testing", DocPath: "testing", Analyzers: testingchecks.Specs()},
	}, []analyzerbase.AnalyzerID{
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
	})
	if err != nil {
		panic(fmt.Sprintf("invalid analyzer catalog: %v", err))
	}
	return catalog
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	metadata := make(map[string]AnalyzerInfo)
	for _, spec := range newCatalog().Analyzers() {
		metadata[spec.Analyzer.Name] = publicAnalyzerInfo(spec)
	}
	return metadata
}

// AnalyzerGroups returns framework-neutral Go policy analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	groups := newCatalog().Groups()
	result := make([]AnalyzerGroup, len(groups))
	for groupIndex, group := range groups {
		result[groupIndex] = AnalyzerGroup{Name: string(group.ID), Doc: group.Doc, DocPath: group.DocPath}
		for _, spec := range group.Analyzers {
			result[groupIndex].Analyzers = append(result[groupIndex].Analyzers, withSuppressions(spec.Analyzer, spec.Checks))
		}
	}
	return result
}

func withSuppressions(analyzer *analysis.Analyzer, declared []analyzerbase.CheckInfo) *analysis.Analyzer {
	checks := make(map[string]bool, len(declared))
	for _, check := range declared {
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
	specs := newCatalog().Analyzers()
	analyzers := make([]*analysis.Analyzer, 0, len(specs))
	for _, spec := range specs {
		analyzers = append(analyzers, withSuppressions(spec.Analyzer, spec.Checks))
	}
	return analyzers
}

// DefaultAnalyzers returns the analyzers enabled when no analyzer selection
// flags are provided.
func DefaultAnalyzers() []*analysis.Analyzer {
	specs := newCatalog().Analyzers()
	analyzers := make([]*analysis.Analyzer, 0, len(specs))
	for _, spec := range specs {
		if spec.EnabledByDefault() {
			analyzers = append(analyzers, withSuppressions(spec.Analyzer, spec.Checks))
		}
	}
	return analyzers
}

func publicAnalyzerInfo(spec analyzerbase.AnalyzerSpec) AnalyzerInfo {
	checks := make([]AnalyzerCheckInfo, len(spec.Checks))
	for index, check := range spec.Checks {
		tags := make([]AnalyzerTag, len(check.Tags))
		for tagIndex, tag := range check.Tags {
			tags[tagIndex] = AnalyzerTag(tag)
		}
		checks[index] = AnalyzerCheckInfo{ID: AnalyzerCheck(check.ID), Doc: check.Doc, Profile: CheckProfile(check.Profile), Tags: tags}
	}
	return AnalyzerInfo{Profile: AnalyzerProfile(spec.Profile), Checks: slices.Clone(checks), SuggestedFix: spec.SuggestedFix}
}
