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
		"internal/lifecycle",
		"internal/resourcemodel",
		"internal/syncmodel",
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
	case "syntax", "ssaflow", "heapmodel", "lifecycle", "resourcemodel", "syncmodel", "passes", "summaries", "check", "analyzers", "trace":
		return component
	default:
		return "other"
	}
}

func forbiddenLayerDependency(from, to string) bool {
	if to == "syncmodel" {
		return slices.Contains([]string{"syntax", "ssaflow", "heapmodel", "lifecycle", "resourcemodel", "passes", "check"}, from)
	}
	switch from {
	case "syntax":
		return slices.Contains([]string{"ssaflow", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers"}, to)
	case "ssaflow":
		return slices.Contains([]string{"heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "heapmodel":
		return slices.Contains([]string{"lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "lifecycle":
		return slices.Contains([]string{"resourcemodel", "passes", "summaries", "check", "analyzers", "trace"}, to)
	case "resourcemodel":
		return slices.Contains([]string{"passes", "summaries", "check", "analyzers", "trace"}, to)
	case "passes":
		return slices.Contains([]string{"summaries", "check", "analyzers"}, to)
	case "summaries":
		return to == "check" || to == "analyzers"
	case "syncmodel":
		return slices.Contains([]string{"summaries", "check", "analyzers", "trace"}, to)
	case "check":
		return slices.Contains([]string{"ssaflow", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "analyzers"}, to)
	default:
		return false
	}
}

func TestSemanticModelDependencyBoundaries(t *testing.T) {
	for _, test := range []struct {
		from, to string
		forbid   bool
	}{
		{"lifecycle", "heapmodel", false},
		{"heapmodel", "lifecycle", true},
		{"syncmodel", "passes", false},
		{"passes", "syncmodel", true},
		{"syncmodel", "ssaflow", false},
		{"lifecycle", "syncmodel", true},
		{"syncmodel", "analyzers", true},
		{"syncmodel", "summaries", true},
	} {
		t.Run(test.from+"/"+test.to, func(t *testing.T) {
			if internalLayer(test.from) != test.from || internalLayer(test.to) != test.to {
				t.Fatal("semantic model layer was not recognized")
			}
			if got := forbiddenLayerDependency(test.from, test.to); got != test.forbid {
				t.Fatalf("forbidden dependency = %v, want %v", got, test.forbid)
			}
		})
	}
}

func analyzerImplementationDependency(from, to, sourceDirectory, importedPath string) bool {
	if from != "analyzers" || to != "analyzers" {
		return false
	}
	sourcePackage := strings.TrimPrefix(sourceDirectory, "internal/")
	return sourcePackage != importedPath
}
