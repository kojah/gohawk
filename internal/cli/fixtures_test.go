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
	return writeSampleModule(t, "module example.com/cancellationfix\n\ngo 1.26.0\n", `package cancellationfix

import "context"

func work(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	_, _ = ctx, cancel
}

type cancelOwner interface {
	Store(context.CancelFunc)
}

func handOff(parent context.Context, owner cancelOwner) {
	_, cancel := context.WithCancel(parent)
	owner.Store(cancel)
}
`, `package cancellationfix

import "testing"

func TestWork(t *testing.T) {
	_ = t
}
`)
}

func writeContextTestModule(t *testing.T, goVersion string) string {
	t.Helper()
	return writeSampleModule(t, "module example.com/contexttest\n\ngo "+goVersion+"\n", "package sample\n", `package sample

import (
	"context"
	"testing"
)

func TestBackground(t *testing.T) {
	_ = t
	go func(ctx context.Context) { <-ctx.Done() }(context.Background())
}
`)
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
	return writeSampleModule(t, "module example.com/evalordertest\n\ngo 1.25.0\n", `package sample

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
`, `package sample

func staleTestOnly(value int) (int, error) {
	return value, replace(&value)
}

func orderedTestOnly(value int) (int, error) {
	err := replace(&value)
	return value, err
}
`)
}

func writeCheckFilterModule(t *testing.T) string {
	t.Helper()
	return writeSampleModule(t, "module example.com/checkfilter\n\ngo 1.25.0\n", `package sample

import "context"

func misplaced(value string, ctx context.Context) {}

func accept(ctx context.Context) {}

func call() {
	accept(nil)
}
`, "")
}

// writeSampleModule writes a module whose go.mod line is goMod, with source in
// sample/sample.go and, when testSource is not empty, sample/sample_test.go.
func writeSampleModule(t *testing.T, goMod, source, testSource string) string {
	t.Helper()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "go.mod"), goMod)
	writeTestFile(t, filepath.Join(directory, "sample", "sample.go"), source)
	if testSource != "" {
		writeTestFile(t, filepath.Join(directory, "sample", "sample_test.go"), testSource)
	}
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
