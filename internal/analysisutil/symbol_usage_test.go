package analysisutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAnalyzersUseSymbolIdentity(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate symbol usage test")
	}
	root := filepath.Join(filepath.Dir(currentFile), "..", "analyzers")

	// These uses need package metadata rather than one exact declaration. Keep
	// the expected counts explicit so every new escape prompts architecture review.
	allowed := map[string]int{
		"ownership/exitpolicy/analyzer.go":            1, // Package path is diagnostic display metadata.
		"reliability/errorclassification/analyzer.go": 1, // Text-preserving strings transforms are a package family.
		"reliability/globalstate/contracts.go":        1, // Framework contracts qualify arbitrary named types.
		"reliability/taintpolicy/analyzer.go":         1, // User-configured sanitizers need qualified call metadata.
	}
	rawIdentityPatterns := []string{
		"CallPackage(",
		".Pkg().Path()",
		".Pkg.Pkg.Path()",
		"Imported().Path()",
		"*types.Builtin",
		"BuiltinClose",
	}
	found := make(map[string]int, len(allowed))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(source)
		escapes := 0
		for _, pattern := range rawIdentityPatterns {
			escapes += strings.Count(text, pattern)
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		relative = filepath.ToSlash(relative)
		found[relative] = escapes
		if escapes != allowed[relative] {
			t.Errorf("%s has %d raw package-identity escapes, want %d; use Symbol or update the reviewed allowance", relative, escapes, allowed[relative])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range allowed {
		if found[path] != want {
			t.Errorf("%s allowance = %d, found %d", path, want, found[path])
		}
	}
}
