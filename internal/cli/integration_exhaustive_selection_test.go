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
			"resources (resources and lifecycle): cancellationownership, deferinloop, processownership, resourcelifetime",
			"concurrency (concurrency and synchronization): channelsafety, concurrentcapture, " +
				"goroutineownership, lockorder, producerlifecycle",
			"correctness (general correctness): nilargument",
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
		for _, value := range []string{"channelsafety", "core runs by default", "producerlifecycle"} {
			if !strings.Contains(output, value) {
				t.Fatalf("list output does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "list", "-defaults")
		if exitCode != 0 || !strings.Contains(output, "producerlifecycle") || !strings.Contains(output, "channelsafety") {
			t.Fatalf("default list: exit code = %d\n%s", exitCode, output)
		}

		output, exitCode = runCommand(t, module, binary, "list", "-opt-in")
		if exitCode != 0 || strings.Contains(output, "producerlifecycle") || strings.Contains(output, "channelsafety") {
			t.Fatalf("opt-in list: exit code = %d\n%s", exitCode, output)
		}
	})

	t.Run("analyzer and check documentation", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "doc", "lockorder")
		if exitCode != 0 {
			t.Fatalf("analyzer documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"lockorder/missing-release"} {
			if !strings.Contains(output, value) {
				t.Fatalf("analyzer documentation does not contain %q:\n%s", value, output)
			}
		}

		output, exitCode = runCommand(t, module, binary, "doc", "lockorder/missing-release")
		if exitCode != 0 {
			t.Fatalf("check documentation: exit code = %d, want 0\n%s", exitCode, output)
		}
		for _, value := range []string{"Reports return paths that leave an owned lock held.", "Analyzer: lockorder"} {
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
		if !strings.Contains(output, "send follows close of channel") {
			t.Fatalf("default analyzer did not run:\n%s", output)
		}
		for _, value := range []string{"warning[channelsafety]", "-->", "sample.go:", "^"} {
			if !strings.Contains(output, value) {
				t.Fatalf("rich diagnostic does not contain %q:\n%s", value, output)
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
		output, exitCode := runCommand(t, module, binary, "-enable=channelsafety", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "send follows close of channel") {
			t.Fatalf("output does not contain channelsafety diagnostic:\n%s", output)
		}
		if strings.Contains(output, "is acquired while already held") {
			t.Fatalf("selected analyzer unexpectedly ran lockorder:\n%s", output)
		}
	})

	t.Run("selected analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-groups=concurrency", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "send follows close of channel") {
			t.Fatalf("concurrency group did not run channelsafety:\n%s", output)
		}
		if !strings.Contains(output, "is acquired while already held") {
			t.Fatalf("concurrency group did not run lockorder:\n%s", output)
		}
	})

	t.Run("disabled analyzer group", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "-disable-groups=resources", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "send follows close of channel") {
			t.Fatalf("enable-all minus resources did not run channelsafety:\n%s", output)
		}
		if !strings.Contains(output, "is acquired while already held") {
			t.Fatalf("enable-all minus resources did not run lockorder:\n%s", output)
		}
	})

	t.Run("all analyzers", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-enable-all", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		for _, diagnostic := range []string{
			"is acquired while already held",
			"send follows close of channel",
		} {
			if !strings.Contains(output, diagnostic) {
				t.Fatalf("all-analyzer output does not contain %q:\n%s", diagnostic, output)
			}
		}
	})

	t.Run("disabling one default analyzer keeps the rest", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=channelsafety", "./...")
		if exitCode != 3 {
			t.Fatalf("exit code = %d, want 3\n%s", exitCode, output)
		}
		if !strings.Contains(output, "is acquired while already held") {
			t.Fatalf("default analyzers did not run:\n%s", output)
		}
		if strings.Contains(output, "send follows close of channel") {
			t.Fatalf("disabled channelsafety unexpectedly reported:\n%s", output)
		}
	})

	t.Run("disabled default analyzer", func(t *testing.T) {
		output, exitCode := runCommand(t, module, binary, "-disable=lockorder", "./...")
		if exitCode != 3 || !strings.Contains(output, "send follows close of channel") || strings.Contains(output, "is acquired while already held") {
			t.Fatalf("disabled analyzer run: exit code = %d\n%s", exitCode, output)
		}
	})
}
