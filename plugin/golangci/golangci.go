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
	Enable    []string `json:"enable"`
	Disable   []string `json:"disable"`
	EnableAll bool     `json:"enable-all"`
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
	for _, name := range settings.Disable {
		if _, ok := metadata[name]; !ok {
			return nil, fmt.Errorf("unknown analyzer in disable: %q", name)
		}
		enabled[name] = false
	}

	selected := make([]*analysis.Analyzer, 0, len(all))
	for _, analyzer := range all {
		if enabled[analyzer.Name] {
			selected = append(selected, analyzer)
		}
	}
	return selected, nil
}
