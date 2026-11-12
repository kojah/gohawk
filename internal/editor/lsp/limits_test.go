package lsp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Limits on a run: what counts as progress, when an idle run is stopped, and
// how far a widely imported package is followed.

func TestGoCommandOutputIsSplitIntoProgressAndMessages(t *testing.T) {
	t.Parallel()
	progress := newRunProgress()
	writer := &progressWriter{progress: progress, rest: cappedBuffer{limit: 1 << 10}}
	// Written in pieces that do not end on line boundaries.
	for _, chunk := range []string{
		"internal/goarch\nexample.com/m/",
		"leak\nexample.com/m/leak [example.com/m/leak.test]\n# example.com/m/bad\n",
		"bad/bad.go:5:19: undefined: undefined\ngo: downloading example.com/x v1.0.0\n\n",
	} {
		if written, err := writer.Write([]byte(chunk)); written != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v", written, err)
		}
	}
	if got := progress.packages.Load(); got != 3 {
		t.Errorf("packages counted = %d, want 3", got)
	}
	kept := writer.rest.buffer.String()
	for _, want := range []string{"# example.com/m/bad", "bad/bad.go:5:19: undefined: undefined", "go: downloading"} {
		if !strings.Contains(kept, want) {
			t.Errorf("message %q was not kept; kept %q", want, kept)
		}
	}
	if strings.Contains(kept, "internal/goarch") {
		t.Errorf("a package name was kept as a message: %q", kept)
	}
}

// helperSleepVariable makes the test binary stand in for a command that
// makes no progress.
const helperSleepVariable = "GOHAWK_TEST_SLEEP"

func TestMain(m *testing.M) {
	if os.Getenv(helperSleepVariable) != "" {
		time.Sleep(time.Minute)
		return
	}
	runTests(m)
}

func TestARunWithoutProgressIsStoppedAndSaysSo(t *testing.T) {
	t.Setenv(helperSleepVariable, "1")
	runContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	guarded := newGuard(runLimits{idle: 300 * time.Millisecond, memoryLimit: defaultMemoryLimit}, cancel, nil)
	started := time.Now()
	_, err := guarded.runProgram(runContext, t.TempDir(), os.Args[0], nil)
	if err == nil {
		t.Fatal("the idle command ran to the end")
	}
	if elapsed := time.Since(started); elapsed > 20*time.Second {
		t.Errorf("the idle command was stopped only after %s", elapsed)
	}
	if got := interrupted(t.Context(), runContext, guarded, outcomeCompleted); got != outcomeTimedOut {
		t.Errorf("outcome = %s, want timed-out", got)
	}
}

func TestProgressKeepsARunAlivePastTheIdleLimit(t *testing.T) {
	t.Parallel()
	progress := newRunProgress()
	progress.last.Store(time.Now().Add(-time.Hour).UnixNano())
	if progress.idleFor() < time.Hour {
		t.Fatal("idle time does not follow the last sign of progress")
	}
	writer := &progressWriter{progress: progress, rest: cappedBuffer{limit: 16}}
	if _, err := writer.Write([]byte("example.com/m/next\n")); err != nil {
		t.Fatal(err)
	}
	if idle := progress.idleFor(); idle > time.Second {
		t.Errorf("a package line did not reset the idle time: %s", idle)
	}
}

func TestAWidelyImportedPackageIsFollowedOnlyIntoOpenImporters(t *testing.T) {
	t.Parallel()
	importers := []string{"/m/a", "/m/b", "/m/c", "/m/d"}
	open := []string{"/m/c", "/m/elsewhere"}
	if kept, skipped := withinImporterLimit(importers, open, 10); !reflect.DeepEqual(kept, importers) || skipped != 0 {
		t.Errorf("under the limit: kept %v, skipped %d", kept, skipped)
	}
	if kept, skipped := withinImporterLimit(importers, open, 0); len(kept) != 4 || skipped != 0 {
		t.Errorf("no limit: kept %v, skipped %d", kept, skipped)
	}
	kept, skipped := withinImporterLimit(importers, open, 3)
	if !reflect.DeepEqual(kept, []string{"/m/c"}) || skipped != 3 {
		t.Errorf("over the limit: kept %v, skipped %d; want only the open importer", kept, skipped)
	}
}

func TestProgressNoteAppearsOnlyOnceARunIsLong(t *testing.T) {
	t.Parallel()
	quick := newRunProgress()
	if note := progressNote(quick); note != "" {
		t.Errorf("a run that just started shows %q", note)
	}
	long := newRunProgress()
	long.started = time.Now().Add(-83 * time.Second)
	long.packages.Store(412)
	if note := progressNote(long); note != " 412 packages, 1m23s" {
		t.Errorf("progress note = %q", note)
	}
	long.packages.Store(1)
	if note := progressNote(long); note != " 1m23s" {
		t.Errorf("a single package shows %q, want only the time", note)
	}
}

// The edited package has more importers than the limit, so only the importer
// the user has open is re-checked, and the status says the others were not.
func TestImporterLimitIsReportedAndLiftedByAWorkspaceCheck(t *testing.T) {
	t.Parallel()
	const library = "package lib\n\nfunc Value() int { return 1 }\n"
	importer := func(name string) string {
		return strings.Replace(leakSource, "package leak", "package "+name+"\n\nimport _ \"example.com/editor/lib\"", 1)
	}
	root := writeModule(t, map[string]string{
		"lib/lib.go":     library,
		"open/open.go":   importer("open"),
		"far1/far1.go":   importer("far1"),
		"far2/far2.go":   importer("far2"),
		"alone/alone.go": "package alone\n",
	})
	opened, far := filepath.Join(root, "open", "open.go"), filepath.Join(root, "far1", "far1.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"importerLimit": 1, "run": runOnSave}})
	client.open(opened, importer("open"))
	client.awaitFindings(opened, 1)
	client.open(filepath.Join(root, "lib", "lib.go"), library)

	deadline := time.After(20 * time.Second)
	var seen []string
	for noted := false; !noted; {
		select {
		case status := <-client.statuses:
			seen = append(seen, status.Text)
			noted = strings.Contains(status.Text, "2 importers not re-checked")
		case <-deadline:
			t.Fatalf("the status never said importers were left unchecked; statuses: %q\nserver log:\n%s", seen, client.logs())
		}
	}
	client.expectQuiet(far, 300*time.Millisecond)

	client.command(commandCheckWorkspace, uriFromPath(opened))
	client.awaitFindings(far, 1)
	deadline = time.After(scenarioTimeout)
	for {
		select {
		case status := <-client.statuses:
			if !strings.Contains(status.Text, "not re-checked") && !strings.Contains(status.Text, "checking") {
				if status.Findings != 3 {
					t.Errorf("after the workspace check: %d findings (%s), want 3", status.Findings, status.Text)
				}
				return
			}
		case <-deadline:
			t.Fatal("the unchecked note never went away")
		}
	}
}
