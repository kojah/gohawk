package architecture

import (
	"os/exec"
	"testing"
)

// Exact regeneration supplements symbol coverage: signatures, comments,
// methods, links, and newly added pass packages must not silently go stale.
// The narrow mode neither runs analyzer examples nor writes the working tree.
func TestSharedHelperReferencesStayCurrent(t *testing.T) {
	inventory := newRepositorySourceInventory(t)
	command := exec.CommandContext(t.Context(), "go", "run", "./tools/gendocs", "-helpers-check")
	command.Dir = inventory.root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("shared helper references: %v\n%s", err, output)
	}
}
