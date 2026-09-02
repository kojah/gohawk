package cli

import (
	"maps"

	gohawk "github.com/kojah/gohawk/analyzers"
	"golang.org/x/tools/go/analysis"
)

func anyEnabled(selection map[string]bool) bool {
	for _, enabled := range selection {
		if enabled {
			return true
		}
	}
	return false
}

func nativeSelectionSuffices(request selectionRequest, hasExplicitEnabled bool) bool {
	return len(request.owners) == 0 &&
		len(request.analyzers.enabled) == 0 && len(request.analyzers.disabled) == 0 &&
		len(request.groups.enabled) == 0 && len(request.groups.disabled) == 0 &&
		!request.enableAll && hasExplicitEnabled
}

func nativeAnalyzerSelection(analyzers []*analysis.Analyzer, explicit map[string]bool, enableAll bool) map[string]bool {
	selected := make(map[string]bool)
	if enableAll {
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = true
		}
	}
	maps.Copy(selected, explicit)
	return selected
}

func baseAnalyzerSelection(
	analyzers []*analysis.Analyzer,
	groups []gohawk.AnalyzerGroup,
	metadata map[string]gohawk.AnalyzerInfo,
	request selectionRequest,
	hasExplicitEnabled bool,
) map[string]bool {
	selected := make(map[string]bool)
	switch {
	case request.enableAll:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = true
		}
	case len(request.groups.enabled) > 0:
		for _, group := range groups {
			if !request.groups.enabled[group.Name] {
				continue
			}
			for _, analyzer := range group.Analyzers {
				selected[analyzer.Name] = true
			}
		}
	case len(request.groups.disabled) > 0 || len(request.analyzers.disabled) > 0:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = metadata[analyzer.Name].EnabledAt(request.ceiling)
		}
	case len(request.analyzers.enabled) > 0:
		// A positive analyzer list establishes its own selection base.
	case hasExplicitEnabled:
		// Naming an analyzer explicitly selects only named analyzers, preserving
		// the analysis-driver convention when no group selector establishes a base.
	case len(request.owners) > 0:
		// An explicit check list establishes its own selection base. Its owning
		// analyzers are added after ordinary analyzer selection is resolved.
	default:
		for _, analyzer := range analyzers {
			selected[analyzer.Name] = metadata[analyzer.Name].EnabledAt(request.ceiling)
		}
	}
	return selected
}

func applyAnalyzerSelection(
	selected map[string]bool,
	groups []gohawk.AnalyzerGroup,
	names analyzerNameSelection,
	groupSelection analyzerGroupSelection,
	explicit map[string]bool,
) {
	for _, group := range groups {
		if !groupSelection.disabled[group.Name] {
			continue
		}
		for _, analyzer := range group.Analyzers {
			selected[analyzer.Name] = false
		}
	}
	for name := range names.disabled {
		selected[name] = false
	}
	for name := range names.enabled {
		selected[name] = true
	}
	maps.Copy(selected, explicit)
}

func applyCheckOwners(selected, checkOwners, disabledNames, explicit map[string]bool) {
	for owner := range checkOwners {
		selected[owner] = true
	}
	for name := range disabledNames {
		selected[name] = false
	}
	for name, enabled := range explicit {
		if !enabled {
			selected[name] = false
		}
	}
}

func enabledAnalyzerFlags(analyzers []*analysis.Analyzer, selected map[string]bool) []string {
	flags := make([]string, 0, len(selected))
	for _, analyzer := range analyzers {
		if selected[analyzer.Name] {
			flags = append(flags, "-"+analyzer.Name+"=true")
		}
	}
	return flags
}
