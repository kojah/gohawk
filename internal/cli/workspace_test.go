package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWorkspaceHint(t *testing.T) {
	tests := []struct {
		name     string
		goStderr string
		want     string
	}{
		{
			name:     "workspace refusal",
			goStderr: "pattern ./...: directory prefix . does not contain modules listed in go.work or their selected dependencies\n",
			want:     workspaceHintLine,
		},
		{
			name:     "parent pattern refusal",
			goStderr: "pattern ../...: directory prefix .. does not contain modules listed in go.work or their selected dependencies\n",
			want:     workspaceHintLine,
		},
		{name: "module mode refusal", goStderr: "pattern ./...: directory prefix . does not contain main module or its selected dependencies\n"},
		{name: "build failure", goStderr: "# example.com/p\np.go:3:2: undefined: x\n"},
		{name: "empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workspaceHint([]byte(test.goStderr)); got != test.want {
				t.Fatalf("workspaceHint(%q) = %q, want %q", test.goStderr, got, test.want)
			}
		})
	}
}

// writeWorkspaceFixture lays out a two-module go.work workspace with one
// finding per module, so a run shows which modules it covered.
func writeWorkspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.work"), "go 1.25.0\n\nuse (\n\t./alpha\n\t./beta\n)\n")
	writeTestFile(t, filepath.Join(root, "alpha", "go.mod"), "module example.com/alpha\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(root, "alpha", "sample", "sample.go"), `package sample

import "sync"

func sharedTotal(items []int) int {
	total := 0
	var wg sync.WaitGroup
	for range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			total++
		}()
	}
	wg.Wait()
	return total
}
`)
	writeTestFile(t, filepath.Join(root, "beta", "go.mod"), "module example.com/beta\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(root, "beta", "sample", "sample.go"), `package sample

import "sync"

type counter struct {
	mu sync.RWMutex
	n  int
}

func (c *counter) bump() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.n++
}
`)
	return root
}

func TestCLIWorkspaceIntegration(t *testing.T) {
	binary := buildTestBinary(t)
	root := writeWorkspaceFixture(t)
	workspace := []string{"GOWORK=" + filepath.Join(root, "go.work")}
	packagesReported := func(t *testing.T, output string) []string {
		t.Helper()
		var diagnostics map[string]map[string][]json.RawMessage
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode JSON output: %v\n%s", err, output)
		}
		return slices.Sorted(func(yield func(string) bool) {
			for name := range diagnostics {
				if !yield(name) {
					return
				}
			}
		})
	}
	t.Run("./... at the workspace root fails with the hint", func(t *testing.T) {
		output, exitCode := runCommandWithEnvironment(t, root, workspace, binary, "./...")
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1\n%s", exitCode, output)
		}
		for _, want := range []string{goWorkRefusal, workspaceHintLine} {
			if !strings.Contains(output, want) {
				t.Errorf("output does not contain %q:\n%s", want, output)
			}
		}
	})
	t.Run("work analyzes every module", func(t *testing.T) {
		output, exitCode := runCommandWithEnvironment(t, root, workspace, binary, "-json", "work")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if got := packagesReported(t, output); !slices.Equal(got, []string{"example.com/alpha/sample", "example.com/beta/sample"}) {
			t.Fatalf("packages reported = %v, want both modules\n%s", got, output)
		}
	})
	t.Run("./... inside a module analyzes that module", func(t *testing.T) {
		output, exitCode := runCommandWithEnvironment(t, filepath.Join(root, "beta"), workspace, binary, "-json", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if got := packagesReported(t, output); !slices.Equal(got, []string{"example.com/beta/sample"}) {
			t.Fatalf("packages reported = %v, want beta only\n%s", got, output)
		}
	})
	t.Run("outside workspace mode go's error stands alone", func(t *testing.T) {
		output, exitCode := runCommand(t, root, binary, "./...")
		if exitCode != 1 || !strings.Contains(output, "main module") {
			t.Fatalf("exit code = %d, want 1 with go's module-mode error\n%s", exitCode, output)
		}
		if strings.Contains(output, "gohawk work") {
			t.Fatalf("hint printed outside workspace mode:\n%s", output)
		}
	})
}
