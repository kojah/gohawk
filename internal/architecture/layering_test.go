package architecture

import (
	"path"
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
		"internal/passes",
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
	case "syntax", "ssaflow", "passes", "check", "analyzers", "trace":
		return component
	default:
		return "other"
	}
}

func forbiddenLayerDependency(from, to string) bool {
	switch from {
	case "syntax":
		return to == "ssaflow" || to == "passes" || to == "check" || to == "analyzers"
	case "ssaflow":
		return to == "passes" || to == "check" || to == "analyzers" || to == "trace"
	case "passes":
		return to == "check" || to == "analyzers"
	case "check":
		return to == "ssaflow" || to == "passes" || to == "analyzers"
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
