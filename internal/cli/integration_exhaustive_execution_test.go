//go:build exhaustive

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:cyclop,funlen,gocognit // Independent subprocess scenarios form one opt-in execution catalog.
func runExhaustiveExecutionScenarios(t *testing.T, binary, module string) {
	t.Helper()

	t.Run("analyzer configuration", func(t *testing.T) {
		goroutineModule := writeGoroutineTestModule(t)
		output, exitCode := runCommand(t, goroutineModule, binary, "-enable=goroutineownership", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("default policy: exit code = %d, output = %q", exitCode, output)
		}

		output, exitCode = runCommand(t, goroutineModule, binary, "-enable=goroutineownership", "-goroutineownership.mode=join", "./...")
		if exitCode != 3 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("join policy: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(
			t,
			goroutineModule,
			"go",
			"vet",
			"-vettool="+binary,
			"-enable=goroutineownership",
			"-goroutineownership.mode=join",
			"./...",
		)
		if exitCode != 1 || !strings.Contains(output, "goroutine is not joined") {
			t.Fatalf("vettool join policy: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("disabled check", func(t *testing.T) {
		checkModule := writeCheckFilterModule(t)
		output, exitCode := runCommand(t, checkModule, binary, "-enable=lockorder", "-disable-checks=lockorder/missing-release", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if strings.Contains(output, "is not released on this return path") {
			t.Fatalf("disabled check still reported:\n%s", output)
		}
		if !strings.Contains(output, "is acquired while already held") {
			t.Fatalf("enabled sibling check did not report:\n%s", output)
		}

		output, exitCode = runCommand(
			t,
			checkModule,
			"go",
			"vet",
			"-vettool="+binary,
			"-enable=lockorder",
			"-disable-checks=lockorder/missing-release",
			"./...",
		)
		if exitCode != 1 || strings.Contains(output, "is not released on this return path") ||
			!strings.Contains(output, "is acquired while already held") {
			t.Fatalf("vettool disabled check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, checkModule, binary,
			"-enable=lockorder",
			"-disable-checks=lockorder/missing-release,lockorder/recursive-acquire",
			"./...",
		)
		if exitCode != 0 || output != "" {
			t.Fatalf("all checks disabled: exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("enabled check", func(t *testing.T) {
		checkModule := writeCheckFilterModule(t)
		output, exitCode := runCommand(t, checkModule, binary, "-enable-checks=lockorder/recursive-acquire", "./...")
		if exitCode != 3 || !strings.Contains(output, "is acquired while already held") {
			t.Fatalf("exact check: exit code = %d\n%s", exitCode, output)
		}
		if strings.Contains(output, "is not released on this return path") {
			t.Fatalf("exact check ran default sibling:\n%s", output)
		}

		output, exitCode = runCommand(t, checkModule, "go", "vet", "-vettool="+binary, "-enable-checks=lockorder/recursive-acquire", "./...")
		if exitCode != 1 || !strings.Contains(output, "is acquired while already held") ||
			strings.Contains(output, "is not released on this return path") {
			t.Fatalf("vettool exact check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary,
			"-enable=oncepolicy", "-enable-checks=channelsafety/send-after-close", "./...",
		)
		if exitCode != 3 || !strings.Contains(output, "sync.OnceFunc wrapper is discarded") ||
			!strings.Contains(output, "send follows close of channel") {
			t.Fatalf("combined analyzer and check selection: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("invalid disabled check", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable-checks=lockorder/not-a-check", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown check "lockorder/not-a-check"`) {
			t.Fatalf("exit code = %d, want 2 with check error\n%s", exitCode, output)
		}
	})

	t.Run("analyzer flags advertised", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-flags")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, name := range []string{
			"disable",
			"disable-checks",
			"disable-groups",
			"enable",
			"enable-checks",
			"enable-groups",
			"goroutineownership.mode",
			"resourcelifetime.contracts",
			"resourcelifetime.require-reader-close",
		} {
			if !strings.Contains(output, `"Name": "`+name+`"`) {
				t.Fatalf("-flags output does not contain %q:\n%s", name, output)
			}
		}
		for _, name := range []string{"channelsafety", "oncepolicy", "lockorder"} {
			if strings.Contains(output, `"Name": "`+name+`"`) {
				t.Fatalf("-flags output still advertises analyzer Boolean %q:\n%s", name, output)
			}
		}
	})

	t.Run("legacy analyzer Boolean selection", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-channelsafety=false", "./...")
		if exitCode != 2 || !strings.Contains(output, "use -disable=channelsafety") {
			t.Fatalf("exit code = %d, want 2 with migration error\n%s", exitCode, output)
		}
	})

	t.Run("invalid analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-groups=unknown", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown analyzer group "unknown"`) {
			t.Fatalf("exit code = %d, want 2 with group error\n%s", exitCode, output)
		}
	})

	t.Run("invalid analyzer option", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=goroutineownership", "-goroutineownership.mode=database", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "database"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("invalid goroutine ownership mode", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=goroutineownership", "-goroutineownership.mode=strict", "./...")
		if exitCode != 2 || !strings.Contains(output, `unknown value "strict"`) {
			t.Fatalf("exit code = %d, want 2 with option error\n%s", exitCode, output)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-json", "-enable=channelsafety", "./...")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		var diagnostics map[string]map[string][]json.RawMessage
		if err := json.Unmarshal([]byte(output), &diagnostics); err != nil {
			t.Fatalf("decode JSON output: %v\n%s", err, output)
		}
		if len(diagnostics) == 0 {
			t.Fatal("JSON output contains no package diagnostics")
		}
		count := 0
		for _, analyzers := range diagnostics {
			count += len(analyzers["channelsafety"])
		}
		if count != 1 {
			t.Fatalf("channelsafety JSON diagnostic count = %d, want 1\n%s", count, output)
		}
	})

	t.Run("vet tool", func(t *testing.T) {
		output, exitCode := runCommand(t, module, "go", "vet", "-vettool="+binary, "./...")
		if exitCode != 1 {
			t.Fatalf("exit code = %d, want 1\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("output does not contain default diagnostic:\n%s", output)
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=channelsafety", "./...")
		if exitCode != 1 || !strings.Contains(output, "send follows close of channel") {
			t.Fatalf("vettool selected analyzer: exit code = %d\n%s", exitCode, output)
		}
		if strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("vettool selected analyzer unexpectedly ran defaults:\n%s", output)
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-disable=oncepolicy", "./...")
		if exitCode != 1 || !strings.Contains(output, "send follows close of channel") || strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("vettool disabled analyzer: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("cancellation ownership through CLI and vet tool", func(t *testing.T) {
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

		output, exitCode = runCommand(t, module, binary, "-enable=cancellationownership", "./...")
		if exitCode != 3 || !strings.Contains(output, diagnostic) {
			t.Fatalf("standalone cancellation diagnostic: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "-enable=cancellationownership", "./...")
		if exitCode != 1 || !strings.Contains(output, diagnostic) {
			t.Fatalf("vettool cancellation diagnostic: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "-fix", "-diff", "./...")
		if exitCode != 0 || !strings.Contains(output, "defer cancel()") {
			t.Fatalf("cancellation fix preview: exit code = %d\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, "_, _ = ctx, cancel")

		output, exitCode = runCommand(t, module, binary, "-fix", "./...")
		if exitCode != 0 {
			t.Fatalf("cancellation fix: exit code = %d\n%s", exitCode, output)
		}
		assertFixtureContains(t, module, "defer cancel()")

		output, exitCode = runCommand(t, module, binary, "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed standalone module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "vet", "-vettool="+binary, "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("fixed vettool module: exit code = %d, output = %q", exitCode, output)
		}
		output, exitCode = runCommand(t, module, "go", "test", "./...")
		if exitCode != 0 {
			t.Fatalf("test fixed module: exit code = %d\n%s", exitCode, output)
		}
	})
}
