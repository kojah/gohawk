package analyzers

import (
	"errors"
	"fmt"

	"github.com/kojah/gohawk/internal/catalog"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of Go policy analyzers.
type AnalyzerGroup struct {
	Name      string
	Doc       string
	DocPath   string
	Analyzers []*analysis.Analyzer
}

// AnalyzerInfo describes capabilities that are not represented by analysis.Analyzer.
type AnalyzerInfo struct {
	OptIn        bool
	Checks       []AnalyzerCheckInfo
	SuggestedFix bool
}

// EnabledByDefault reports whether the analyzer runs without explicit selection.
func (info AnalyzerInfo) EnabledByDefault() bool {
	return !info.OptIn
}

func newCatalog() (*catalog.Catalog, error) {
	return catalog.NewCatalog([]catalog.GroupSpec{
		{ID: "contracts", Doc: "API and data contracts", DocPath: "api-and-data-contracts", Analyzers: contractSpecs()},
		{ID: "ownership", Doc: "ownership and lifecycle", DocPath: "ownership-and-lifecycle", Analyzers: ownershipSpecs()},
		{ID: "reliability", Doc: "reliability and safety", DocPath: "reliability-and-safety", Analyzers: reliabilitySpecs()},
		{ID: "testing", Doc: "testing", DocPath: "testing", Analyzers: testingSpecs()},
	}, []catalog.AnalyzerID{
		"apishape",
		"contextpolicy",
		"testlifecycle",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"goroutineownership",
		"producerlifecycle",
		"errorownership",
		"errorclassification",
		"inlineerror",
		"channelcapacity",
		"channelownership",
		"channelsafety",
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
		"borrowedstorage",
	})
}

// AnalyzerMetadata returns documentation metadata keyed by analyzer name.
func AnalyzerMetadata() map[string]AnalyzerInfo {
	metadata := make(map[string]AnalyzerInfo)
	catalog, err := newCatalog()
	if err != nil {
		// The catalog is compiled-in and covered by validation tests. If a bad
		// declaration nevertheless ships, expose no analyzers rather than letting
		// an imported library terminate its host process.
		return metadata
	}
	for _, spec := range catalog.Analyzers() {
		metadata[spec.Analyzer.Name] = publicAnalyzerInfo(spec)
	}
	return metadata
}

// AnalyzerGroups returns framework-neutral Go policy analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	catalog, err := newCatalog()
	if err != nil {
		return nil
	}
	groups := catalog.Groups()
	result := make([]AnalyzerGroup, len(groups))
	for groupIndex, group := range groups {
		result[groupIndex] = AnalyzerGroup{Name: string(group.ID), Doc: group.Doc, DocPath: group.DocPath}
		for _, spec := range group.Analyzers {
			result[groupIndex].Analyzers = append(result[groupIndex].Analyzers, withSuppressions(group.ID, spec))
		}
	}
	return result
}

// testingGroup is the catalog group whose analyzers take tests as their
// subject; only they report inside _test.go files by default.
const testingGroup = "testing"

func withSuppressions(group catalog.GroupID, spec catalog.AnalyzerSpec) *analysis.Analyzer {
	return withCheckFilter(spec.Analyzer, spec.Checks, nil, group == testingGroup)
}

func withDefaultSuppressions(group catalog.GroupID, spec catalog.AnalyzerSpec) *analysis.Analyzer {
	disabled := make(map[string]bool)
	for _, declaredCheck := range spec.Checks {
		if !declaredCheck.EnabledByDefault() {
			disabled[string(declaredCheck.ID)] = true
		}
	}
	return withCheckFilter(spec.Analyzer, spec.Checks, disabled, group == testingGroup)
}

func withCheckFilter(analyzer *analysis.Analyzer, declared []catalog.CheckInfo, disabled map[string]bool, targetsTests bool) *analysis.Analyzer {
	checks := make(map[string]bool, len(declared))
	for _, declaredCheck := range declared {
		checks[string(declaredCheck.ID)] = true
	}
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		var reportErr error
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if !checks[diagnostic.Category] {
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: "decision", Reason: "unknown-check", Outcome: analysisTrace.OutcomeRejected, Diagnostic: diagnostic,
				})
				reportErr = errors.Join(reportErr, fmt.Errorf("analyzer %q reported unknown check %q", analyzer.Name, diagnostic.Category))
				return
			}
			if disabled[diagnostic.Category] {
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: "decision", Reason: "check-not-selected", Outcome: analysisTrace.OutcomeAccepted,
					Diagnostic: diagnostic,
				})
				return
			}
			if check.Suppressed(pass, diagnostic.Pos, analyzer.Name) {
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: "decision", Reason: "suppression-comment", Outcome: analysisTrace.OutcomeAccepted, Diagnostic: diagnostic,
				})
				return
			}
			if !targetsTests && !check.IncludeTests() && check.TestFilePosition(pass, diagnostic.Pos) {
				// Test files are skipped by default for every analyzer whose
				// subject is production code; see internal/check/testfiles.go.
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: "decision", Reason: "test-file-skipped", Outcome: analysisTrace.OutcomeAccepted, Diagnostic: diagnostic,
				})
				return
			}
			report(diagnostic)
		}
		defer func() { pass.Report = report }()
		result, err := run(pass)
		return result, errors.Join(err, reportErr)
	}
	return analyzer
}

// Analyzers returns all framework-neutral Go policy analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	catalog, err := newCatalog()
	if err != nil {
		return nil
	}
	specs := catalog.Analyzers()
	analyzers := make([]*analysis.Analyzer, 0, len(specs))
	for _, spec := range specs {
		analyzers = append(analyzers, withSuppressions(spec.Group, spec))
	}
	return analyzers
}

// DefaultAnalyzers returns the analyzers enabled when no explicit selection
// is provided.
func DefaultAnalyzers() []*analysis.Analyzer {
	catalog, err := newCatalog()
	if err != nil {
		return nil
	}
	specs := catalog.Analyzers()
	analyzers := make([]*analysis.Analyzer, 0, len(specs))
	for _, spec := range specs {
		if spec.EnabledByDefault() {
			analyzers = append(analyzers, withDefaultSuppressions(spec.Group, spec))
		}
	}
	return analyzers
}

func publicAnalyzerInfo(spec catalog.AnalyzerSpec) AnalyzerInfo {
	checks := make([]AnalyzerCheckInfo, len(spec.Checks))
	for index, check := range spec.Checks {
		checks[index] = AnalyzerCheckInfo{ID: AnalyzerCheck(check.ID), Doc: check.Doc, Kind: CheckKind(check.Kind), OptIn: check.OptIn}
	}
	return AnalyzerInfo{OptIn: spec.OptIn, Checks: checks, SuggestedFix: spec.SuggestedFix}
}
