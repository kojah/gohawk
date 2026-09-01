package architecture

import (
	"strings"
	"testing"
)

func TestAnalyzersUseSymbolIdentity(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)

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
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers") {
		text := string(source.source)
		escapes := 0
		for _, pattern := range rawIdentityPatterns {
			escapes += strings.Count(text, pattern)
		}
		relative := strings.TrimPrefix(source.repositoryPath, "internal/analyzers/")
		found[relative] = escapes
		if escapes != allowed[relative] {
			t.Errorf("%s has %d raw package-identity escapes, want %d; use Symbol or update the reviewed allowance", relative, escapes, allowed[relative])
		}
	}
	for path, want := range allowed {
		if found[path] != want {
			t.Errorf("%s allowance = %d, found %d", path, want, found[path])
		}
	}
}
