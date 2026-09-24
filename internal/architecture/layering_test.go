package architecture

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const internalImportPrefix = "github.com/kojah/gohawk/internal/"

func TestInternalPackagesRespectDependencyDirection(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	for _, source := range inventory.productionGoFiles(
		t,
		"internal/syntax",
		"internal/ssaflow",
		"internal/heapmodel",
		"internal/ssainfer",
		"internal/resourcemodel",
		"internal/passes",
		"internal/summaries",
		"internal/check",
		"internal/analyzers",
	) {
		from := internalLayer(strings.TrimPrefix(path.Dir(source.repositoryPath), "internal/"))
		for _, imported := range source.file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("%s: parse import %s: %v", source.repositoryPath, imported.Path.Value, err)
			}
			if !strings.HasPrefix(importPath, internalImportPrefix) {
				continue
			}
			toPath := strings.TrimPrefix(importPath, internalImportPrefix)
			to := internalLayer(toPath)
			if forbiddenLayerDependency(from, to) || analyzerImplementationDependency(from, to, path.Dir(source.repositoryPath), toPath) {
				line := source.fileSet.Position(imported.Pos()).Line
				t.Errorf("%s:%d imports %s; %s must not depend on %s", source.repositoryPath, line, importPath, from, to)
			}
		}
	}
}

func internalLayer(packagePath string) string {
	component, _, _ := strings.Cut(packagePath, "/")
	switch component {
	case "syntax", "ssaflow", "heapmodel", "ssainfer", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace":
		return component
	default:
		return "other"
	}
}

func forbiddenLayerDependency(from, to string) bool {
	switch from {
	case "syntax":
		return slices.Contains([]string{"ssaflow", "heapmodel", "ssainfer", "resourcemodel", "passes", "summaries", "check", "analyzers"}, to)
	case "ssaflow":
		return slices.Contains([]string{"heapmodel", "ssainfer", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "heapmodel":
		return slices.Contains([]string{"ssainfer", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "ssainfer":
		return slices.Contains([]string{"resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "resourcemodel":
		return slices.Contains([]string{"passes", "summaries", "check", "analyzers", "trace"}, to)
	case "passes":
		return slices.Contains([]string{"summaries", "check", "analyzers"}, to)
	case "summaries":
		return to == "check" || to == "analyzers"
	case "check":
		return slices.Contains([]string{"ssaflow", "heapmodel", "ssainfer", "resourcemodel", "passes", "summaries", "analyzers"}, to)
	default:
		return false
	}
}

func analyzerImplementationDependency(from, to, sourceDirectory, importedPath string) bool {
	if from != "analyzers" || to != "analyzers" {
		return false
	}
	sourcePackage := strings.TrimPrefix(sourceDirectory, "internal/")
	return sourcePackage != importedPath
}
