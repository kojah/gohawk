package cli

import (
	"errors"
	"fmt"
	"strings"

	gohawk "github.com/kojah/gohawk/analyzers"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// analyzeForDump loads the packages with their syntax and runs analyzers
// over them in this process, one action at a time, so a view can read the
// results, diagnostics, and timings the ordinary run would produce.
func analyzeForDump(patterns []string, includeTests bool, analyzers []*analysis.Analyzer) (*checker.Graph, error) {
	if len(patterns) == 0 {
		return nil, errors.New("at least one package pattern is required")
	}
	config := &packages.Config{Mode: packages.LoadAllSyntax, Tests: includeTests}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(loaded) > 0 {
		return nil, errors.New("packages have load errors")
	}
	return checker.Analyze(analyzers, loaded, &checker.Options{Sequential: true})
}

// catalogAnalyzers returns the named catalog analyzers, or all of them when
// names is empty.
func catalogAnalyzers(names string) ([]*analysis.Analyzer, error) {
	all := gohawk.Analyzers()
	if names == "" {
		return all, nil
	}
	byName := make(map[string]*analysis.Analyzer, len(all))
	for _, analyzer := range all {
		byName[analyzer.Name] = analyzer
	}
	var selected []*analysis.Analyzer
	for name := range strings.SplitSeq(names, ",") {
		analyzer, ok := byName[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("unknown analyzer %q", name)
		}
		selected = append(selected, analyzer)
	}
	return selected, nil
}
