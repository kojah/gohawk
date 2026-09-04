package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:cyclop,funlen,gocognit // Independent end-to-end scenarios intentionally remain in one integration suite.
func TestCLIIntegration(t *testing.T) {
	binary := buildTestBinary(t)

	t.Run("standalone output and selection", func(t *testing.T) {
		t.Parallel()
		module := writeTestModule(t)

		output, exitCode := runCommand(t, module, binary, "./...")
		if exitCode != 3 {
			t.Fatalf("default run: exit code = %d, want 3\n%s", exitCode, output)
		}
		for _, value := range []string{"warning[oncepolicy]", "-->", "sample.go:", "^", "sync.OnceFunc wrapper is discarded"} {
			if !strings.Contains(output, value) {
				t.Fatalf("rich diagnostic does not contain %q:\n%s", value, output)
			}
		}
		if strings.Contains(output, "persisted or wire struct literal") {
			t.Fatalf("default run included opt-in determinism:\n%s", output)
		}

		output, exitCode = runCommand(t, module, binary, "-json", "-enable=determinism", "./...")
		if exitCode != 0 {
			t.Fatalf("selected JSON run: exit code = %d, want 0\n%s", exitCode, output)
		}
		var diagnostics map[string]map[string][]json.RawMessage
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode JSON output: %v\n%s", err, output)
		}
		count := 0
		for _, analyzers := range diagnostics {
			count += len(analyzers["determinism"])
			if len(analyzers["oncepolicy"]) != 0 {
				t.Fatalf("selected analyzer unexpectedly ran defaults:\n%s", output)
			}
		}
		if count != 1 {
			t.Fatalf("determinism JSON diagnostic count = %d, want 1\n%s", count, output)
		}

		// Analyzer flags must reach the go analysis driver so its action cache
		// cannot reuse the preceding single-analyzer result for enable-all.
		output, exitCode = runCommand(t, module, binary, "-json", "-enable-all", "./...")
		if exitCode != 0 {
			t.Fatalf("enable-all JSON run: exit code = %d, want 0\n%s", exitCode, output)
		}
		diagnostics = nil
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode enable-all JSON output: %v\n%s", err, output)
		}
		var oncePolicy, wirePolicy int
		for _, analyzers := range diagnostics {
			oncePolicy += len(analyzers["oncepolicy"])
			wirePolicy += len(analyzers["determinism"])
		}
		if oncePolicy == 0 || wirePolicy == 0 {
			t.Fatalf("enable-all JSON diagnostics omit oncepolicy or determinism:\n%s", output)
		}
	})

	t.Run("canonical production and test files", func(t *testing.T) {
		t.Parallel()
		module := writeEvalOrderTestModule(t)
		output, exitCode := runCommand(t, module, binary, "-json", "-enable=evalorder", "-gohawk-include-tests", "./...")
		if exitCode != 0 {
			t.Fatalf("evalorder JSON run: exit code = %d, want 0\n%s", exitCode, output)
		}
		var diagnostics map[string]map[string][]struct {
			Posn string `json:"posn"`
		}
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode evalorder JSON output: %v\n%s", err, output)
		}
		var productionCount, testCount int
		for _, analyzers := range diagnostics {
			for _, diagnostic := range analyzers["evalorder"] {
				switch {
				case strings.Contains(diagnostic.Posn, "sample_test.go:"):
					testCount++
				case strings.Contains(diagnostic.Posn, "sample.go:"):
					productionCount++
				default:
					t.Fatalf("unexpected evalorder diagnostic at %q\n%s", diagnostic.Posn, output)
				}
			}
		}
		// With -gohawk-include-tests both the production and the test-file
		// operand-mutation are reported, each once.
		if productionCount != 1 || testCount != 1 {
			t.Fatalf("evalorder counts = production %d, test %d; want 1 each\n%s", productionCount, testCount, output)
		}
	})

	t.Run("test files skipped by default", func(t *testing.T) {
		t.Parallel()
		module := writeEvalOrderTestModule(t)
		output, exitCode := runCommand(t, module, binary, "-json", "-enable=evalorder", "./...")
		if exitCode != 0 {
			t.Fatalf("evalorder JSON run: exit code = %d, want 0\n%s", exitCode, output)
		}
		if strings.Contains(output, "sample_test.go:") {
			t.Fatalf("test-file diagnostic reported without -gohawk-include-tests\n%s", output)
		}
		if !strings.Contains(output, "sample.go:") {
			t.Fatalf("production diagnostic missing\n%s", output)
		}
	})

	for _, fixture := range []struct {
		name    string
		version string
		source  string
	}{
		{name: "Go 1.26", version: "1.26.0", source: `package sample

func identity[T any](value T) T { return value }

func answer() int { return identity(42) }
`},
		// Generic methods require Go 1.27. This proves the analyzer process
		// understands the newer syntax, rather than only loading its go.mod.
		{name: "Go 1.27", version: "1.27.0", source: `package sample

type identity struct{}

func (identity) value[T any](input T) T { return input }

func answer() int { return identity{}.value(42) }
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			module := writeLanguageVersionModule(t, fixture.version, fixture.source)
			output, exitCode := runCommand(t, module, binary, "./...")
			if exitCode != 0 || output != "" {
				t.Fatalf("analyze Go %s module: exit code = %d, output = %q", fixture.version, exitCode, output)
			}
		})
	}

	t.Run("target Go version", func(t *testing.T) {
		t.Parallel()
		legacyModule := writeContextTestModule(t, "1.23.0")
		output, exitCode := runCommand(t, legacyModule, binary, "-enable-checks=channelcapacity/rationale", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.23 module: exit code = %d, output = %q", exitCode, output)
		}

		modernModule := writeContextTestModule(t, "1.24.0")
		output, exitCode = runCommand(t, modernModule, binary, "-enable-checks=channelcapacity/rationale", "./...")
		if exitCode != 3 || !strings.Contains(output, "test-owned goroutine uses a never-cancelled context") {
			t.Fatalf("Go 1.24 module: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer option through vettool", func(t *testing.T) {
		t.Parallel()
		module := writeGoroutineTestModule(t)
		output, exitCode := runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 1 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("vettool analyzer option: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("check filtering", func(t *testing.T) {
		t.Parallel()
		module := writeCheckFilterModule(t)
		output, exitCode := runCommand(t, module, binary, "-enable=lockorder", "-disable-checks=lockorder/missing-release", "./...")
		if exitCode != 3 || strings.Contains(output, "context.Context must be first parameter") ||
			!strings.Contains(output, "do not pass nil context.Context") {
			t.Fatalf("filtered checks: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("invalid analyzer option", func(t *testing.T) {
		t.Parallel()
		module := writeTestModule(t)
		output, exitCode := runCommand(t, module, binary, "-enable=borrowedstorage", "-borrowedstorage.sinks=database", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "database"`) {
			t.Fatalf("invalid option: exit code = %d, want 2\n%s", exitCode, output)
		}
	})

	t.Run("cancellation trace and fix", func(t *testing.T) {
		t.Parallel()
		module := writeCancellationFixModule(t)
		const diagnostic = "cancel function from context.WithCancel is not called on every return path"
		tracePath := filepath.Join(t.TempDir(), "evidence.jsonl")

		output, exitCode := runCommand(
			t,
			module,
			binary,
			"-json",
			"-enable=cancellationownership",
			"-gohawk-trace=cancellationownership",
			"-gohawk-trace-file="+tracePath,
			"./...",
		)
		if exitCode != 0 || !json.Valid([]byte(output)) {
			t.Fatalf("traced JSON run: exit code = %d\n%s", exitCode, output)
		}
		assertCancellationTrace(t, tracePath)

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=cancellationownership", "./...")
		if exitCode != 1 || !strings.Contains(output, diagnostic) {
			t.Fatalf("vettool cancellation diagnostic: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("cancellation fix: exit code = %d\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, "defer cancel()")
		output, exitCode = runCommand(t, module, "go", "test", "./...")
		if exitCode != 0 {
			t.Fatalf("test fixed module: exit code = %d\n%s", exitCode, output)
		}
	})
}

func assertCancellationTrace(t *testing.T, tracePath string) {
	t.Helper()
	traceOutput, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read evidence trace: %v", err)
	}
	found := map[string]bool{}
	for line := range bytes.SplitSeq(bytes.TrimSpace(traceOutput), []byte("\n")) {
		var event struct {
			Analyzer string `json:"analyzer"`
			Phase    string `json:"phase"`
			Reason   string `json:"reason"`
			Outcome  string `json:"outcome"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode evidence trace line %q: %v", line, err)
		}
		if event.Analyzer == "cancellationownership" {
			found[event.Phase+"/"+event.Reason+"/"+event.Outcome] = true
		}
	}
	for _, want := range []string{
		"candidate/diagnostic-candidate/observed",
		"decision/ambiguous-cancellation-use/unknown",
		"decision/unowned-return/rejected",
		"decision/diagnostic-reported/rejected",
		"fix/suggested-fix-available/accepted",
	} {
		if !found[want] {
			t.Fatalf("trace does not contain %s:\n%s", want, traceOutput)
		}
	}
}
