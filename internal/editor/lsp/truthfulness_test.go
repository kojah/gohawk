package lsp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Each scenario here once showed the user something untrue: a real finding
// hidden behind a clean status, a finding for text that no longer exists, or
// an unknown state reported as known. They were found by adversarial testing
// against the command-line result for the same content.

func inPackage(source, name string) string {
	return strings.Replace(source, "package leak", "package "+name, 1)
}

// go vet caches analysis output by content. Returning to earlier text replays
// output that names the replacement file of the earlier run.
func TestReturningToEarlierTextShowsItsFindingAgain(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": fixedSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(path, fixedSource)
	client.awaitStatus(stateIdle)

	client.change(path, leakSource, 2)
	client.awaitFindings(path, 1)
	client.change(path, fixedSource, 3)
	client.awaitFindings(path, 0)
	client.change(path, leakSource, 4)
	finding := client.awaitFindings(path, 1)[0]
	if finding.Range.Start.Line != 8 {
		t.Errorf("replayed finding is on line %d, want 8", finding.Range.Start.Line)
	}
}

func TestWorkspaceOpenedThroughASymbolicLink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs a privilege on Windows")
	}
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	link := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
	path := filepath.Join(link, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: link})
	client.open(path, leakSource)
	// The finding must arrive on the URI the editor opened.
	client.awaitFindings(path, 1)
}

func TestAPackageThatDoesNotCompileStaysNamedWhileOthersAreChecked(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{
		"leak/leak.go":   leakSource,
		"clean/clean.go": inPackage(fixedSource, "clean"),
		"other/other.go": inPackage(fixedSource, "other"),
	})
	leak, clean, other := filepath.Join(root, "leak", "leak.go"), filepath.Join(root, "clean", "clean.go"), filepath.Join(root, "other", "other.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(leak, leakSource)
	client.awaitFindings(leak, 1)
	client.open(clean, inPackage(fixedSource, "clean"))
	client.awaitStatus(stateFindings)

	// Break both a package with a finding and a package without one.
	client.change(leak, strings.Replace(leakSource, `"io"`, `"ioo"`, 1), 2)
	client.awaitStatus(stateStale)
	client.change(clean, strings.Replace(inPackage(fixedSource, "clean"), `"io"`, `"ioo"`, 1), 2)
	client.awaitStatus(stateStale)

	// Checking a third, healthy package must not make the state look known.
	client.open(other, inPackage(fixedSource, "other"))
	deadline := time.After(scenarioTimeout)
	for {
		select {
		case status := <-client.statuses:
			if status.State == stateIdle || status.State == stateFindings {
				t.Fatalf("status became %q (%s) while two packages do not compile", status.State, status.Text)
			}
			if status.State == stateStale && strings.Contains(status.Text, "2 packages") && status.Findings == 1 {
				return
			}
		case <-deadline:
			t.Fatal("status never settled on two packages failing to build")
		}
	}
}

func TestBufferOpenedWithTextThatDiffersFromDisk(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root})
	// The editor restores unsaved work: the buffer is fixed, the file leaks.
	client.open(path, fixedSource)
	client.awaitStatus(stateIdle)
	client.expectQuiet(path, 300*time.Millisecond)
}

func TestClosingAnUnsavedBufferRevertsToTheFile(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(path, leakSource)
	client.awaitFindings(path, 1)
	client.change(path, fixedSource, 2)
	client.awaitFindings(path, 0)
	// Discarding the buffer leaves the leaking file as the truth.
	client.close(path)
	client.awaitFindings(path, 1)
}

func TestWorkspaceRunLeavesANestedModuleAlone(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{
		"a/a.go":          inPackage(leakSource, "a"),
		"nested/go.mod":   "module example.com/nested\n\ngo 1.25.0\n",
		"nested/n/n.go":   inPackage(leakSource, "n"),
		"nested/n/doc.go": "package n\n",
	})
	outer, nested := filepath.Join(root, "a", "a.go"), filepath.Join(root, "nested", "n", "n.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(nested, inPackage(leakSource, "n"))
	client.awaitFindings(nested, 1)
	client.open(outer, inPackage(leakSource, "a"))
	client.awaitFindings(outer, 1)

	client.command(commandCheckWorkspace, uriFromPath(outer))
	// Wait for the whole-module run to start, then for it to finish.
	running := true
	deadline := time.After(scenarioTimeout)
	for started := false; running; {
		select {
		case status := <-client.statuses:
			checking := strings.Contains(status.Tooltip, "every package in the module")
			started = started || checking
			if started && !checking {
				running = false
				if status.Findings != 2 {
					t.Errorf("after the outer module run: %d findings (%s), want the nested module's finding kept", status.Findings, status.Text)
				}
			}
		case <-deadline:
			t.Fatal("the whole-module run never finished")
		}
	}
}

func TestDeletedFilesLoseTheirFindings(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource, "leak/doc.go": "package leak\n"})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(filepath.Join(root, "leak", "doc.go"), "package leak\n")
	client.awaitFindings(path, 1)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	client.notify(methodDidChangeWatchedFiles, watchedFilesParams{Changes: []fileEvent{{URI: uriFromPath(path), Change: fileDeleted}}})
	client.awaitFindings(path, 0)

	// A file that appears on disk without being opened is found too.
	added := filepath.Join(root, "leak", "added.go")
	writeFile(t, added, leakSource)
	client.notify(methodDidChangeWatchedFiles, watchedFilesParams{Changes: []fileEvent{{URI: uriFromPath(added), Change: 1}}})
	client.awaitFindings(added, 1)
}

// When one importer stops compiling, the multi-package run cannot say which
// packages it analyzed. Each importer is then checked on its own.
func TestOneBrokenImporterDoesNotFreezeTheOthers(t *testing.T) {
	t.Parallel()
	const helper = "package lib\n\nimport \"os\"\n\nfunc Open(path string) (*os.File, error) { return os.Open(path) }\n"
	const user = `package NAME

import "example.com/editor/lib"

func Use(path string) error {
	f, err := lib.Open(path)
	if err != nil {
		return err
	}
	_ = f
	return nil
}
`
	root := writeModule(t, map[string]string{
		"lib/lib.go":     helper,
		"good/good.go":   strings.Replace(user, "NAME", "good", 1),
		"bad/bad.go":     "package bad\n\nimport _ \"example.com/editor/lib\"\n\nfunc Broken() { undefined() }\n",
		"leak/leak.go":   leakSource,
		"leak/import.go": "package leak\n\nimport _ \"example.com/editor/lib\"\n",
	})
	client := startEditor(t, editorOptions{workspace: root})
	client.open(filepath.Join(root, "lib", "lib.go"), helper)
	// leak imports lib and compiles, so its finding must still arrive even
	// though bad, another importer, does not compile.
	client.awaitFindings(filepath.Join(root, "leak", "leak.go"), 1)
	status := client.awaitStatus(stateStale)
	for !strings.Contains(status.Text, "build failed in bad") {
		status = client.awaitStatus(stateStale)
	}
}

func TestImportersInASiblingWorkspaceModuleAreChecked(t *testing.T) {
	// Not parallel: the scenario needs the go command's workspace mode, which
	// the environment can switch off. CI runs with GOWORK=off, and a -mod
	// flag in GOFLAGS is rejected in workspace mode. The server passes its
	// environment on to the go command, as it must for a real user.
	t.Setenv("GOWORK", "")
	t.Setenv("GOFLAGS", "")
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const library = "package lib\n\nfunc Value() int { return 1 }\n"
	writeFile(t, filepath.Join(base, "go.work"), "go 1.25.0\n\nuse (\n\t./lib\n\t./app\n)\n")
	writeFile(t, filepath.Join(base, "lib", "go.mod"), "module example.com/lib\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(base, "lib", "lib.go"), library)
	writeFile(t, filepath.Join(base, "app", "go.mod"), "module example.com/app\n\ngo 1.25.0\n\nrequire example.com/lib v0.0.0\n")
	application := strings.Replace(leakSource, "package leak", "package app\n\nimport _ \"example.com/lib\"", 1)
	writeFile(t, filepath.Join(base, "app", "app.go"), application)

	client := startEditor(t, editorOptions{workspace: base})
	client.open(filepath.Join(base, "lib", "lib.go"), library)
	// app lives in another module of the workspace and was never opened.
	client.awaitFindings(filepath.Join(base, "app", "app.go"), 1)
}

const unwaitedCommandSource = `package launch

import "os/exec"

func Run(rules []byte) error {
	cmd := exec.Command("nft", "-f", "-")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := input.Write(rules); err != nil {
		return err
	}
	return cmd.Wait()
}
`

func TestTogglingExperimentalChecksTakesEffectAtOnce(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"launch/launch.go": unwaitedCommandSource})
	path := filepath.Join(root, "launch", "launch.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"scope": scopePackage}})
	client.open(path, unwaitedCommandSource)
	client.awaitStatus(stateIdle)

	configure := func(experimental bool) {
		client.notify(methodDidChangeConfiguration, map[string]any{
			"settings": map[string]any{"gohawk": map[string]any{"experimental": experimental, "debounce": 50, "scope": scopePackage}},
		})
	}
	configure(true)
	finding := client.awaitFindings(path, 1)[0]
	if finding.Code != "processownership/missing-wait" || finding.Severity != severityInformation {
		t.Errorf("finding = %s severity %d, want the experimental check as information", finding.Code, finding.Severity)
	}
	configure(false)
	client.awaitFindings(path, 0)
}

// With checks on save, findings describe the saved file. Typing must neither
// cancel the check of the saved text nor analyze the unsaved buffer.
func TestChecksOnSaveDescribeTheSavedFile(t *testing.T) {
	t.Parallel()
	root := writeModule(t, map[string]string{"leak/leak.go": leakSource})
	path := filepath.Join(root, "leak", "leak.go")
	client := startEditor(t, editorOptions{workspace: root, settings: map[string]any{"run": runOnSave, "scope": scopePackage}})
	client.open(path, leakSource)
	client.awaitFindings(path, 1)

	// The fix is typed but not saved: the finding stays.
	client.change(path, fixedSource, 2)
	client.expectQuiet(path, 400*time.Millisecond)

	writeFile(t, path, fixedSource)
	client.save(path)
	// A keystroke right after the save must not strand the check.
	client.change(path, fixedSource+"\n", 3)
	client.awaitFindings(path, 0)
	client.awaitStatus(stateIdle)
}
