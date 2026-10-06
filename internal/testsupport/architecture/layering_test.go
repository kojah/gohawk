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
		"internal/engine/proof",
		"internal/engine/syntax",
		"internal/engine/ssaflow",
		"internal/engine/heapmodel",
		"internal/engine/lifecycle",
		"internal/engine/resourcemodel",
		"internal/analysis/passes",
		"internal/analysis/summaries",
		"internal/reporting/check",
		"internal/analysis/analyzers",
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
	for _, container := range []string{"engine/", "analysis/", "reporting/", "testsupport/"} {
		if nested, ok := strings.CutPrefix(packagePath, container); ok {
			packagePath = nested
			break
		}
	}
	if nested, ok := strings.CutPrefix(packagePath, "ssaflow/"); ok {
		component, _, _ := strings.Cut(nested, "/")
		switch component {
		case "cfg", "calls", "path":
			return component
		}
	}
	component, _, _ := strings.Cut(packagePath, "/")
	switch component {
	case "proof", "syntax", "ssaflow", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace":
		return component
	default:
		return "other"
	}
}

func forbiddenLayerDependency(from, to string) bool {
	switch from {
	case "cfg":
		return to != "proof" && to != "cfg"
	case "calls", "path":
		return from == "calls" && to == "path" || slices.Contains([]string{
			"heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace",
		}, to)
	case "proof":
		return slices.Contains([]string{
			"syntax", "ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel",
			"passes", "summaries", "check", "analyzers", "trace",
		}, to)
	case "syntax":
		return slices.Contains([]string{
			"ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers",
		}, to)
	case "ssaflow":
		return slices.Contains([]string{
			"calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "check", "analyzers", "trace",
		}, to)
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
	case "check":
		return slices.Contains([]string{
			"ssaflow", "cfg", "calls", "path", "heapmodel", "lifecycle", "resourcemodel", "passes", "summaries", "analyzers",
		}, to)
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

type layerDependencyCase struct {
	from, to string
	forbid   bool
}

func TestSSADependencyBoundaries(t *testing.T) {
	for _, test := range []layerDependencyCase{
		{"ssaflow", "ssaflow/cfg", false},
		{"ssaflow/cfg", "proof", false},
		{"ssaflow/cfg", "ssaflow", true},
		{"ssaflow/cfg", "lifecycle", true},
		{"syntax", "ssaflow/cfg", true},
		{"proof", "ssaflow/cfg", true},
		{"ssaflow/calls", "ssaflow", false},
		{"ssaflow/path", "ssaflow/calls", false},
		{"ssaflow/calls", "ssaflow/path", true},
		{"ssaflow", "ssaflow/calls", true},
		{"ssaflow", "ssaflow/path", true},
		{"ssaflow/cfg", "ssaflow/path", true},
		{"ssaflow/calls", "lifecycle", true},
		{"ssaflow/path", "heapmodel", true},
		{"ssaflow/cfg/cache", "ssaflow/path", true},
		{"check", "ssaflow/cfg", true},
		{"check", "ssaflow/calls", true},
		{"check", "ssaflow/path", true},
		{"engine/ssaflow", "engine/ssaflow/calls", true},
		{"engine/ssaflow/calls", "engine/ssaflow/path", true},
		{"engine/ssaflow/path", "engine/ssaflow/calls", false},
		{"reporting/check", "engine/ssaflow/path", true},
		{"analysis/passes/lifecyclefacts", "analysis/analyzers/resources/resourcelifetime", true},
		{"analysis/summaries", "analysis/passes/lifecyclefacts", false},
	} {
		t.Run(test.from+"/"+test.to, func(t *testing.T) {
			if got := forbiddenLayerDependency(internalLayer(test.from), internalLayer(test.to)); got != test.forbid {
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
