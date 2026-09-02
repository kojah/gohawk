//go:build exhaustive

package cli

import (
	"strings"
	"testing"
)

//nolint:cyclop,funlen,gocognit // Independent subprocess scenarios form one opt-in selection catalog.
func runExhaustiveSelectionScenarios(t *testing.T, binary, module string) {
	t.Helper()

	t.Run("version", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-V")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		if !strings.HasPrefix(output, "gohawk ") || strings.Contains(output, "buildID=") {
			t.Fatalf("human version output = %q", output)
		}

		output, exitCode = runCommand(t, module, binary, "-V=full")
		if exitCode != 0 || !strings.Contains(output, "version devel") || !strings.Contains(output, "buildID=") {
			t.Fatalf("vettool version protocol: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("grouped help", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "help")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, summary := range []string{
			"contracts (API and data contracts): apishape*, contextpolicy, closedomain*, wirepolicy*",
			"ownership (ownership and lifecycle): borrowedstorage*, cancellationownership, channelcapacity*, channelownership*, channelsafety, deferinloop, " +
				"exitpolicy, goroutineownership, producerlifecycle, processownership, resourcelifetime",
			"reliability (reliability and safety): concurrentcapture, determinism*, errorownership*, errorclassification, " +
				"inlineerror, evalorder, globalstate*, lockorder, oncepolicy, syncmapatomicity, taintpolicy*",
			"testing (testing): testlifecycle*, testpolicy*",
		} {
			if !strings.Contains(output, summary) {
				t.Fatalf("help does not contain %q:\n%s", summary, output)
			}
		}
	})

	t.Run("list analyzers", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "list")
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"apishape", "extended", "core runs by default", "oncepolicy"} {
			if !strings.Contains(output, value) {
				t.Fatalf("list output does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "list", "-defaults")
		if exitCode != 0 || !strings.Contains(output, "oncepolicy") || strings.Contains(output, "wirepolicy") {
			t.Fatalf("default list: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "list", "-opt-in")
		if exitCode != 0 || !strings.Contains(output, "wirepolicy") || strings.Contains(output, "oncepolicy") {
			t.Fatalf("opt-in list: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer and check documentation", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "doc", "contextpolicy")
		if exitCode != 0 {
			t.Fatalf("analyzer documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"contextpolicy/nil-context"} {
			if !strings.Contains(output, value) {
				t.Fatalf("analyzer documentation does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "doc", "contextpolicy/nil-context")
		if exitCode != 0 {
			t.Fatalf("check documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"Reports definitely nil context.Context arguments.", "Analyzer: contextpolicy"} {
			if !strings.Contains(output, value) {
				t.Fatalf("check documentation does not contain %q:\n%s", value, output)
			}
		}
	})

	t.Run("ordinary run", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("default analyzer did not run:\n%s", output)
		}
		for _, value := range []string{"warning[oncepolicy]", "-->", "sample.go:", "^"} {
			if !strings.Contains(output, value) {
				t.Fatalf("rich diagnostic does not contain %q:\n%s", value, output)
			}
		}
		for _, diagnostic := range []string{"persisted or wire struct literal", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("opt-in analyzer unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("one binary supports Go 1.26 and Go 1.27 modules", func(t *testing.T) {
		for _, fixture := range []struct {
			version string
			source  string
		}{
			{version: "1.26.0", source: `package sample

func identity[T any](value T) T { return value }

func answer() int { return identity(42) }
`},
			// Generic methods require Go 1.27. Keeping one here proves that the
			// analyzer process itself understands the newer language version, not
			// merely that its go command can load a module declaring it.
			{version: "1.27.0", source: `package sample

type identity struct{}

func (identity) value[T any](input T) T { return input }

func answer() int { return identity{}.value(42) }
`},
		} {
			t.Run("go"+fixture.version, func(t *testing.T) {
				compatibilityModule := writeLanguageVersionModule(t, fixture.version, fixture.source)
				output, exitCode := runCommand(t, compatibilityModule, binary, "./...")
				if exitCode != 0 || output != "" {
					t.Fatalf("analyze Go %s module: exit code = %d, output = %q", fixture.version, exitCode, output)
				}
			})
		}
	})

	t.Run("selected analyzer", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable=wirepolicy", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("output does not contain wirepolicy diagnostic:\n%s", output)
		}
		if strings.Contains(output, "mutable package state") {
			t.Fatalf("selected analyzer unexpectedly ran globalstate:\n%s", output)
		}
	})

	t.Run("selected analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-groups=contracts", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("contracts group did not run opt-in wirepolicy:\n%s", output)
		}
		for _, diagnostic := range []string{"sync.OnceFunc wrapper is discarded", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("contracts group unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabled analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "-disable-groups=reliability", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "persisted or wire struct literal must use field keys") {
			t.Fatalf("enable-all minus reliability did not run wirepolicy:\n%s", output)
		}
		for _, diagnostic := range []string{"sync.OnceFunc wrapper is discarded", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("disabled reliability group unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("all analyzers", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		for _, diagnostic := range []string{
			"sync.OnceFunc wrapper is discarded",
			"persisted or wire struct literal",
			"mutable package state cache",
		} {
			if !strings.Contains(output, diagnostic) {
				t.Fatalf("all-analyzer output does not contain %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabling opt-in analyzer keeps ordinary set", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=globalstate", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "sync.OnceFunc wrapper is discarded") {
			t.Fatalf("default analyzers did not run:\n%s", output)
		}
		for _, diagnostic := range []string{"persisted or wire struct literal", "mutable package state"} {
			if strings.Contains(output, diagnostic) {
				t.Fatalf("opt-in analyzer unexpectedly reported %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabled default analyzer", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=oncepolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("exit code = %d, output = %q", exitCode, output)
		}
	})

	t.Run("target Go version", func(t *testing.T) {
		legacyModule := writeContextTestModule(t, "1.23.0")
		output, exitCode := runCommand(t, legacyModule, binary, "-enable=contextpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.23 module: exit code = %d, output = %q", exitCode, output)
		}

		modernModule := writeContextTestModule(t, "1.24.0")
		output, exitCode = runCommand(t, modernModule, binary, "-enable=contextpolicy", "./...")
		if exitCode != 0 || output != "" {
			t.Fatalf("Go 1.24 default checks: exit code = %d, output = %q", exitCode, output)
		}

		output, exitCode = runCommand(t, modernModule, binary, "-enable-checks=testlifecycle/context-root", "./...")
		if exitCode != 3 || !strings.Contains(output, "test-owned goroutine uses a never-cancelled context") {
			t.Fatalf("Go 1.24 opt-in check: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, modernModule, binary, "-enable-all", "./...")
		if exitCode != 3 || !strings.Contains(output, "test-owned goroutine uses a never-cancelled context") {
			t.Fatalf("Go 1.24 enable-all: exit code = %d\n%s", exitCode, output)
		}
	})
}
