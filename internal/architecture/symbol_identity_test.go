package architecture

import (
	"strings"
	"testing"
)

func TestAnalyzersUseSymbolIdentity(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)

	// All current analyzers can identify known declarations through Symbol.
	rawIdentityPatterns := []string{
		"CallPackage(",
		".Pkg().Path()",
		".Pkg.Pkg.Path()",
		"Imported().Path()",
		"*types.Builtin",
		"BuiltinClose",
	}
	for _, source := range inventory.productionGoFiles(t, "internal/analyzers") {
		text := string(source.source)
		escapes := 0
		for _, pattern := range rawIdentityPatterns {
			escapes += strings.Count(text, pattern)
		}
		relative := strings.TrimPrefix(source.repositoryPath, "internal/analyzers/")
		if escapes != 0 {
			t.Errorf("%s has %d raw package-identity escapes; use Symbol", relative, escapes)
		}
	}
}
