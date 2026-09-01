// Package golangci registers gohawk as a golangci-lint module plugin.
//
// Import this package when building a custom golangci-lint binary. Importing
// the regular gohawk analyzer packages does not register the plugin.
package golangci

import (
	"fmt"
	"slices"

	"github.com/golangci/plugin-module-register/register"
	"github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/analysisutil"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("gohawk", New)
}

// New constructs the gohawk golangci-lint plugin.
func New(settings any) (register.LinterPlugin, error) {
	decoded, err := register.DecodeSettings[pluginSettings](settings)
	if err != nil {
		return nil, err
	}

	selected, err := selectAnalyzers(decoded)
	if err != nil {
		return nil, err
	}

	return plugin{analyzers: selected}, nil
}

type pluginSettings struct {
	Enable        []string `json:"enable"`
	Disable       []string `json:"disable"`
	EnableChecks  []string `json:"enable-checks"`
	DisableChecks []string `json:"disable-checks"`
	EnableAll     bool     `json:"enable-all"`
}

type plugin struct {
	analyzers []*analysis.Analyzer
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return slices.Clone(p.analyzers), nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func selectAnalyzers(settings pluginSettings) ([]*analysis.Analyzer, error) {
	all := analyzers.Analyzers()
	metadata := analyzers.AnalyzerMetadata()
	enabled := make(map[string]bool, len(all))
	for _, analyzer := range all {
		enabled[analyzer.Name] = settings.EnableAll || metadata[analyzer.Name].EnabledByDefault()
	}

	for _, name := range settings.Enable {
		if _, ok := metadata[name]; !ok {
			return nil, fmt.Errorf("unknown analyzer in enable: %q", name)
		}
		enabled[name] = true
	}

	checkOwners := make(map[string]string)
	for analyzer, info := range metadata {
		for _, check := range info.Checks {
			checkOwners[string(check.ID)] = analyzer
		}
	}
	enabledChecks, err := requestedChecks("enable-checks", settings.EnableChecks, checkOwners)
	if err != nil {
		return nil, err
	}
	disabledChecks, err := requestedChecks("disable-checks", settings.DisableChecks, checkOwners)
	if err != nil {
		return nil, err
	}
	for check := range enabledChecks {
		enabled[checkOwners[check]] = true
	}

	// Analyzer disables take precedence over check enables, matching the CLI:
	// explicitly suppressing an analyzer must not let one of its checks add it
	// back to the golangci-lint execution plan.
	for _, name := range settings.Disable {
		if _, ok := metadata[name]; !ok {
			return nil, fmt.Errorf("unknown analyzer in disable: %q", name)
		}
		enabled[name] = false
	}

	selected := make([]*analysis.Analyzer, 0, len(all))
	for _, analyzer := range all {
		if !enabled[analyzer.Name] {
			continue
		}
		disabled := make(map[string]bool)
		for _, check := range metadata[analyzer.Name].Checks {
			id := string(check.ID)
			if disabledChecks[id] || !settings.EnableAll && !check.EnabledByDefault() && !enabledChecks[id] {
				disabled[id] = true
			}
		}
		configured := withDisabledChecks(analyzer, disabled, len(metadata[analyzer.Name].Checks))
		selected = append(selected, analysisutil.IncludeProductionFilesInTestVariants(configured))
	}
	return selected, nil
}

func requestedChecks(setting string, values []string, owners map[string]string) (map[string]bool, error) {
	requested := make(map[string]bool, len(values))
	for _, id := range values {
		if _, ok := owners[id]; !ok {
			return nil, fmt.Errorf("unknown check in %s: %q", setting, id)
		}
		if requested[id] {
			return nil, fmt.Errorf("check appears more than once in %s: %q", setting, id)
		}
		requested[id] = true
	}
	return requested, nil
}

func withDisabledChecks(analyzer *analysis.Analyzer, disabled map[string]bool, checkCount int) *analysis.Analyzer {
	wrapper := *analyzer
	run := analyzer.Run
	allDisabled := len(disabled) == checkCount
	wrapper.Run = func(pass *analysis.Pass) (any, error) {
		if allDisabled {
			return nil, nil
		}
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if disabled[diagnostic.Category] {
				analysisTrace.EmitDiagnostic(pass, analyzer.Name, "decision", "check-disabled", analysisTrace.OutcomeAccepted, diagnostic)
				return
			}
			analysisTrace.EmitDiagnostic(pass, analyzer.Name, "decision", "diagnostic-reported", analysisTrace.OutcomeRejected, diagnostic)
			report(diagnostic)
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return &wrapper
}
