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
	writeTestFile(t, workspace, ".golangci.yml", `version: "2"
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
          enable:
            - globalstate
          disable:
            - lockorder
`)
	writeTestFile(t, workspace, "go.mod", `module example.com/gohawk-golangci-integration

go 1.25.0
`)
	writeTestFile(t, workspace, "main.go", `package fixture

import "sync"

var values = map[string]string{}

func deferredUnlockInLoop(mu *sync.Mutex, values []int) {
	for range values {
		mu.Lock()
		defer mu.Unlock()
	}
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
	run := exec.CommandContext(ctx, binary, "run")
	run.Dir = workspace
	run.Env = append(os.Environ(), "GOWORK=off")
	outputBytes, err := run.CombinedOutput()
	output := string(outputBytes)
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
		t.Fatalf("run custom golangci-lint error = %v, want finding exit code 1\n%s", err, output)
	}
	for _, expected := range []string{
		"globalstate: mutable package state values",
		"deferinloop: deferred cleanup runs after the loop",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output does not contain %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "lockorder:") {
		t.Errorf("disabled lockorder analyzer reported a finding:\n%s", output)
	}
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
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
