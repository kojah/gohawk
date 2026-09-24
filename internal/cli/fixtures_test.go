package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestHumanVersion(t *testing.T) {
	for _, test := range []struct {
		name          string
		linkedVersion string
		info          *debug.BuildInfo
		ok            bool
		want          string
	}{
		{name: "release workflow", linkedVersion: "v0.3.1", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true, want: "v0.3.1"},
		{name: "go install", info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1"}}, ok: true, want: "v0.1.1"},
		{name: "missing build info", info: nil, ok: false, want: "devel"},
		{name: "development module", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true, want: "devel"},
		{name: "empty module version", info: &debug.BuildInfo{}, ok: true, want: "devel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := humanVersion(test.linkedVersion, test.info, test.ok); got != test.want {
				t.Errorf("humanVersion(%q, %#v, %t) = %q, want %q", test.linkedVersion, test.info, test.ok, got, test.want)
			}
		})
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

func sendAfterClose(ch chan int) {
	close(ch)
	ch <- 1
}

func recursiveLock() {
	var mu sync.Mutex
	mu.Lock()
	// Reacquiring this lock supplies the lockorder diagnostic.
	mu.Lock()
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

func writeCancellationModule(t *testing.T) string {
	t.Helper()
	return writeSampleModule(t, "module example.com/cancellation\n\ngo 1.26.0\n", `package cancellation

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
`, `package cancellation

import "testing"

func TestWork(t *testing.T) {
	_ = t
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
	}()
}
`)
	return directory
}

func writeChannelSafetyTestModule(t *testing.T) string {
	t.Helper()
	return writeSampleModule(t, "module example.com/channelsafetytest\n\ngo 1.25.0\n", `package sample

func staleProduction(ch chan int) {
	close(ch)
	ch <- 1
}

func orderedProduction(ch chan int) {
	ch <- 1
	close(ch)
}
`, `package sample

func staleTestOnly(ch chan int) {
	close(ch)
	ch <- 1
}

func orderedTestOnly(ch chan int) {
	ch <- 1
	close(ch)
}
`)
}

func writeCheckFilterModule(t *testing.T) string {
	t.Helper()
	return writeSampleModule(t, "module example.com/checkfilter\n\ngo 1.25.0\n", `package sample

import "sync"

func lockTwice(mu *sync.Mutex) {
	lock := mu
	lock.Lock()
	mu.Lock()
}

func sometimesUnlock(mu *sync.Mutex, unlock bool) {
	mu.Lock()
	if unlock {
		mu.Unlock()
	}
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
