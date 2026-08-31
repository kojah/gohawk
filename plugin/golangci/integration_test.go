//go:build integration

package golangci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const golangCILintVersion = "v2.13.2"

func TestCustomGolangCILint(t *testing.T) {
	repository := repositoryRoot(t)
	workspace := t.TempDir()
	writeTestFile(t, workspace, ".custom-gcl.yml", fmt.Sprintf(`version: %s
name: custom-gcl
destination: .
plugins:
  - module: github.com/kojah/gohawk
    import: github.com/kojah/gohawk/plugin/golangci
    path: %q
`, golangCILintVersion, repository))
	writeTestFile(t, workspace, "go.mod", `module example.com/gohawk-golangci-integration

go 1.25.0
`)
	writeTestFile(t, workspace, "main.go", `package fixture

import "sync"

var values = map[string]string{}

func remember(key, value string) {
	values[key] = value
}

func deferredUnlockInLoop(mu *sync.Mutex, values []int) {
	for range values {
		mu.Lock()
		defer mu.Unlock()
	}
}
`)
	// Keep the test in the production package. golangci-lint analyzes this as an
	// augmented test variant rather than also running the ordinary package, so
	// production files must remain visible to gohawk in the combined pass.
	writeTestFile(t, workspace, "main_test.go", `package fixture

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	go func(ctx context.Context) { <-ctx.Done() }(context.Background())
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"+golangCILintVersion, "custom")
	build.Dir = workspace
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build custom golangci-lint: %v\n%s", err, output)
	}

	binary := filepath.Join(workspace, "custom-gcl")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	tests := []struct {
		name    string
		config  string
		want    []string
		exclude []string
	}{
		{
			name: "ordinary run",
			config: pluginConfig(`          enable:
            - globalstate
          disable:
            - lockorder`),
			want: []string{
				"globalstate: mutable package state values",
				"deferinloop: deferred cleanup runs after the loop",
			},
			exclude: []string{"lockorder:", "contextpolicy: test-owned goroutine"},
		},
		{
			name: "individual checks",
			config: pluginConfig(`          enable:
            - globalstate
          disable:
            - lockorder
          enable-checks:
            - contextpolicy/test-context
          disable-checks:
            - deferinloop/cleanup-lifetime`),
			want: []string{
				"globalstate: mutable package state values",
				"contextpolicy: test-owned goroutine",
			},
			exclude: []string{"lockorder:", "deferinloop:"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeTestFile(t, workspace, ".golangci.yml", test.config)
			run := exec.CommandContext(ctx, binary, "run")
			run.Dir = workspace
			run.Env = append(os.Environ(), "GOWORK=off")
			outputBytes, err := run.CombinedOutput()
			output := string(outputBytes)
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
				t.Fatalf("run custom golangci-lint error = %v, want finding exit code 1\n%s", err, output)
			}
			for _, expected := range test.want {
				if !strings.Contains(output, expected) {
					t.Errorf("output does not contain %q:\n%s", expected, output)
				}
			}
			for _, excluded := range test.exclude {
				if strings.Contains(output, excluded) {
					t.Errorf("output unexpectedly contains %q:\n%s", excluded, output)
				}
			}
		})
	}
}

func pluginConfig(settings string) string {
	return `version: "2"
linters:
  default: none
  enable:
    - gohawk
  settings:
    custom:
      gohawk:
        type: module
        description: gohawk end-to-end test
        settings:
` + settings + "\n"
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func writeTestFile(t *testing.T, directory, name, content string) {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
