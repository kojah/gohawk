package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func TestHumanVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1"}}
	if got := humanVersion(info, true); got != "v0.1.1" {
		t.Fatalf("humanVersion() = %q, want v0.1.1", got)
	}
	for _, test := range []struct {
		info *debug.BuildInfo
		ok   bool
	}{
		{info: nil, ok: false},
		{info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true},
		{info: &debug.BuildInfo{}, ok: true},
	} {
		if got := humanVersion(test.info, test.ok); got != "devel" {
			t.Errorf("humanVersion(%#v, %t) = %q, want devel", test.info, test.ok, got)
		}
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	name := "gohawk"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	output, exitCode := runCommand(t, repositoryRoot, "go", "build", "-o", binary, ".")
	if exitCode != 0 {
		t.Fatalf("build CLI: exit code = %d\n%s", exitCode, output)
	}
	return binary
}

func writeTestModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/gohawkcli\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package sample

import "sync"

type EventRow struct {
	ID   string `+"`json:\"id\"`"+`
	Kind string `+"`json:\"kind\"`"+`
}

var event = EventRow{"42", "created"}

var cache = map[string]string{}

func initialize() {
	cache["ready"] = "true"
	sync.OnceFunc(func() {})()
}
`)
	return directory
}

func writeLanguageVersionModule(t *testing.T, version, source string) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/languageversion\n\ngo "+version+"\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), source)
	return directory
}

func writeCancellationFixModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/cancellationfix\n\ngo 1.26.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package cancellationfix

import "context"

func work(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	_, _ = ctx, cancel
}
`)
	writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), `package cancellationfix

import "testing"

func TestWork(t *testing.T) {
	_ = t
}
`)
	return directory
}

func writeTestPolicyFixModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/testpolicyfix\n\ngo 1.26.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "helper_test.go"), `package sample

import "testing"

func requireUser(t *testing.T) {
	// Keep this setup comment.
	t.Log("require user")
}

func emptyHelper(t *testing.T) {}

func TestHelpers(t *testing.T) {
	requireUser(t)
	emptyHelper(t)
}
`)
	return directory
}

func writeContextTestModule(t *testing.T, goVersion string) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/contexttest\n\ngo "+goVersion+"\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), "package sample\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), `package sample

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	go func(ctx context.Context) { <-ctx.Done() }(context.Background())
}
`)
	return directory
}

func writeGoroutineTestModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/goroutinetest\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "worker", "worker.go"), `package worker

import "context"

func Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
	}()
}
`)
	return directory
}

func writeEvalOrderTestModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/evalordertest\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package sample

func replace(target *int) error {
	*target = 42
	return nil
}

func staleProduction(value int) (int, error) {
	return value, replace(&value)
}

func orderedProduction(value int) (int, error) {
	err := replace(&value)
	return value, err
}
`)
	writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), `package sample

func staleTestOnly(value int) (int, error) {
	return value, replace(&value)
}

func orderedTestOnly(value int) (int, error) {
	err := replace(&value)
	return value, err
}
`)
	return directory
}

func writeCheckFilterModule(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), "module example.com/checkfilter\n\ngo 1.25.0\n")
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), `package sample

import "context"

func misplaced(value string, ctx context.Context) {}

func accept(ctx context.Context) {}

func call() {
	accept(nil)
}
`)
	return directory
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func assertFixtureContains(t *testing.T, module, want string) {
	t.Helper()
	contents := moduleFileContents(t, module, filepath.Join("sample", "sample.go"))
	if !strings.Contains(contents, want) {
		t.Fatalf("fixture does not contain %q:\n%s", want, contents)
	}
}

func moduleFileContents(t *testing.T, module, relativePath string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(module, relativePath))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(contents)
}

func runCommand(t *testing.T, directory, name string, arguments ...string) (string, int) {
	t.Helper()
	command := exec.CommandContext(t.Context(), name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0
	}
	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(output), exitError.ExitCode()
	}
	t.Fatalf("run %s: %v", name, err)
	return "", -1
}
