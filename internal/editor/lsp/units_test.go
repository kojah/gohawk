package lsp

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/reporting/vetjson"
)

func TestParseByteSize(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input string
		want  int64
		ok    bool
	}{
		{"2GiB", 2 << 30, true},
		{"512MiB", 512 << 20, true},
		{"1024", 1024, true},
		{"64KiB", 64 << 10, true},
		{"", 0, false},
		{"0GiB", 0, false},
		{"-1MiB", 0, false},
		{"2GB", 0, false},
		{"lots", 0, false},
	} {
		got, ok := parseByteSize(test.input)
		if got != test.want || ok != test.ok {
			t.Errorf("parseByteSize(%q) = %d, %t; want %d, %t", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestSettingsFallBackToDefaults(t *testing.T) {
	t.Parallel()
	got := settings{Run: "always", Scope: "galaxy", MemoryLimit: "huge", MaxParallel: 0}.normalized()
	if got.Run != runOnPause || got.Scope != scopeThenImporters || got.MemoryLimit != defaultMemoryLimit || got.MaxParallel != defaultMaxParallel {
		t.Errorf("normalized settings = %+v", got)
	}
	if got.debounce() != defaultDebounce || got.timeout() != defaultTimeout {
		t.Errorf("default delays = %s, %s", got.debounce(), got.timeout())
	}
}

func TestReverseClosureFollowsImportsTransitively(t *testing.T) {
	t.Parallel()
	packages := []listedPackage{
		{Dir: "/m/base", ImportPath: "m/base"},
		{Dir: "/m/mid", ImportPath: "m/mid", Imports: []string{"m/base", "fmt"}},
		{Dir: "/m/top", ImportPath: "m/top", Imports: []string{"m/mid"}},
		{Dir: "/m/other", ImportPath: "m/other", Imports: []string{"fmt"}},
	}
	got := reverseClosure(packages, []string{"/m/base"})
	want := map[string]bool{"/m/mid": true, "/m/top": true}
	if len(got) != len(want) {
		t.Fatalf("importers = %v, want mid and top", got)
	}
	for _, directory := range got {
		if !want[filepath.ToSlash(directory)] {
			t.Errorf("unexpected importer %s", directory)
		}
	}
	if importers := reverseClosure(packages, []string{"/m/top"}); len(importers) != 0 {
		t.Errorf("a leaf package has importers: %v", importers)
	}
}

func TestPositionsCountUTF16UnitsOfTheAnalyzedText(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.go")
	// "é" is two bytes and one UTF-16 unit; "😀" is four bytes and two units.
	buffer := &overlay{texts: map[string]string{path: "x\ns := \"é😀\"; call()\n"}}
	convert := newConverter(func(string) CheckInfo { return CheckInfo{Experimental: true} }, buffer, nil)
	byteColumn := len(`s := "é😀"; `) + 1
	findings := convert.convert([]vetjson.Diagnostic{{
		Check: "a/b", Message: "m",
		Start: vetjson.Position{Filename: path, Line: 2, Column: byteColumn},
		End:   vetjson.Position{Filename: path, Line: 2, Column: byteColumn + len("call()")},
	}})
	if len(findings) != 1 {
		t.Fatalf("converted %d findings, want 1", len(findings))
	}
	got := findings[0].diagnostic
	wantStart := position{Line: 1, Character: len(`s := "`) + 1 + 2 + len(`"; `)}
	if got.Range.Start != wantStart || got.Range.End.Character != wantStart.Character+6 {
		t.Errorf("range = %+v, want start %+v and six columns", got.Range, wantStart)
	}
	if got.Severity != severityInformation {
		t.Errorf("experimental check severity = %d, want information", got.Severity)
	}
}

// go vet replays cached output that names a replacement file from an earlier
// run or an earlier server, so the path alone must lead back to the real file.
func TestOverlayPathsDecodeWithoutTheRunThatWroteThem(t *testing.T) {
	t.Parallel()
	long := filepath.Join(t.TempDir(), strings.Repeat("deep/", 60), "name with space é.go")
	for _, real := range []string{filepath.Join(t.TempDir(), "pkg", "a.go"), filepath.FromSlash(long)} {
		written, err := writeOverlay(t.TempDir(), map[string]string{real: "package pkg\n"})
		if err != nil {
			t.Fatal(err)
		}
		temporary := encodeOverlayPath(written.directory, real)
		if _, err := os.Stat(temporary); err != nil {
			t.Errorf("replacement for %q was not written: %v", real, err)
		}
		written.remove()
		// Decoding needs neither the overlay value nor the file.
		if got, ok := decodeOverlayPath(temporary); !ok || got != real {
			t.Errorf("decodeOverlayPath(%q) = %q, %t; want %q", temporary, got, ok, real)
		}
		if filepath.Base(temporary) != filepath.Base(real) {
			t.Errorf("replacement %q does not keep the base name", temporary)
		}
	}
	for _, ordinary := range []string{"/elsewhere/b.go", filepath.Join("/x", overlayMarker, "b.go"), filepath.Join("/x", overlayMarker, "!!!", "b.go")} {
		if got, ok := decodeOverlayPath(ordinary); ok {
			t.Errorf("an ordinary path %q decoded to %q", ordinary, got)
		}
	}
	if none, err := writeOverlay(t.TempDir(), nil); none != nil || err != nil {
		t.Errorf("no unsaved buffers produced an overlay: %v, %v", none, err)
	}
}

func TestResolvedPathsAreSpelledAsTheEditorSpellsThem(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs a privilege on Windows")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target, link := filepath.Join(base, "real"), filepath.Join(base, "link")
	if err := os.MkdirAll(filepath.Join(target, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	spelling := newPathSpelling([]string{link})
	want := filepath.Join(link, "pkg", "a.go")
	for _, reported := range []string{filepath.Join(target, "pkg", "a.go"), want} {
		if got := spelling.editorPath(reported); got != want {
			t.Errorf("editorPath(%q) = %q, want %q", reported, got, want)
		}
	}
	outside := filepath.Join(base, "other", "b.go")
	if got := spelling.editorPath(outside); got != outside {
		t.Errorf("a path outside the workspace was rewritten to %q", got)
	}
}

func TestStoreReplacesOnlyCoveredPackages(t *testing.T) {
	t.Parallel()
	store := newResultStore()
	a, b := filepath.FromSlash("/m/a/a.go"), filepath.FromSlash("/m/b/b.go")
	finding := func(path string) fileDiagnostic {
		return fileDiagnostic{path: path, diagnostic: diagnostic{Message: path}}
	}
	store.replace([]string{filepath.Dir(a), filepath.Dir(b)}, []fileDiagnostic{finding(a), finding(b)})

	// Re-checking a alone finds nothing: a is cleared and republished, b stays.
	touched, _ := store.replace([]string{filepath.Dir(a)}, nil)
	if !reflect.DeepEqual(touched, []string{a}) {
		t.Errorf("touched = %v, want only %s", touched, a)
	}
	if len(store.forFile(a)) != 0 || len(store.forFile(b)) != 1 || store.total() != 1 {
		t.Errorf("after replace: a=%d b=%d total=%d", len(store.forFile(a)), len(store.forFile(b)), store.total())
	}
	// A finding outside the covered packages is refused, and reported back.
	_, refused := store.replace([]string{filepath.Dir(a)}, []fileDiagnostic{finding(filepath.FromSlash("/m/c/c.go"))})
	if store.total() != 1 || len(refused) != 1 {
		t.Errorf("uncovered finding: total = %d, refused = %d", store.total(), len(refused))
	}
}

func TestStoreKeepsAPackageUnresolvedUntilItIsCheckedAgain(t *testing.T) {
	t.Parallel()
	store := newResultStore()
	a, b := filepath.FromSlash("/m/a"), filepath.FromSlash("/m/b")
	store.replace([]string{a}, []fileDiagnostic{{path: filepath.Join(a, "a.go")}})
	// b has no findings; it must still be remembered as not compiling.
	store.markUnresolved([]string{a, b}, outcomeBuildFailed)
	if broken, stale := store.unresolvedBy(outcomeBuildFailed); len(broken) != 2 || stale != 1 {
		t.Errorf("unresolved = %v with %d stale findings, want both packages and one finding", broken, stale)
	}
	// Checking an unrelated package changes nothing about a and b.
	store.replace([]string{filepath.FromSlash("/m/c")}, nil)
	if broken, _ := store.unresolvedBy(outcomeBuildFailed); len(broken) != 2 {
		t.Errorf("unresolved after an unrelated check = %v", broken)
	}
	store.replace([]string{b}, nil)
	if broken, _ := store.unresolvedBy(outcomeBuildFailed); !reflect.DeepEqual(broken, []string{a}) {
		t.Errorf("unresolved after b compiled again = %v, want only a", broken)
	}
	if touched := store.dropUnder(a); len(touched) != 1 || store.total() != 0 {
		t.Errorf("dropUnder: touched %v, total %d", touched, store.total())
	}
	if broken, _ := store.unresolvedBy(outcomeBuildFailed); len(broken) != 0 {
		t.Errorf("a deleted package is still unresolved: %v", broken)
	}
}

func TestTransportRoundTrip(t *testing.T) {
	t.Parallel()
	var stream bytes.Buffer
	writer := newConnection(&bytes.Buffer{}, &stream)
	if err := writer.notify(methodStatus, StatusParams{State: stateIdle, Text: "gohawk"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.notify(methodLogMessage, logMessageParams{Type: messageInfo, Message: "héllo"}); err != nil {
		t.Fatal(err)
	}
	reader := newConnection(&stream, &bytes.Buffer{})
	first, err := reader.read()
	if err != nil || first.Method != methodStatus {
		t.Fatalf("first message = %+v, %v", first, err)
	}
	second, err := reader.read()
	var log logMessageParams
	if err != nil || !decode(second.Params, &log) || log.Message != "héllo" {
		t.Fatalf("second message = %+v, %v", second, err)
	}
	if _, err := newConnection(bytes.NewBufferString("Content-Length: nope\r\n\r\n"), &bytes.Buffer{}).read(); err == nil {
		t.Error("a malformed header was accepted")
	}
}

func TestFileURIsRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "dir with space", "a.go")
	got, ok := pathFromURI(uriFromPath(path))
	if !ok || got != path {
		t.Errorf("round trip of %q = %q, %t", path, got, ok)
	}
	if _, ok := pathFromURI("untitled:Untitled-1"); ok {
		t.Error("an untitled buffer was given a file path")
	}
	if runtime.GOOS == "windows" {
		if got, ok := pathFromURI("file:///c%3A/work/a.go"); !ok || got != `C:\work\a.go` {
			t.Errorf("drive letter URI = %q, %t", got, ok)
		}
	}
}

func TestRelativePatterns(t *testing.T) {
	t.Parallel()
	root := filepath.FromSlash("/m")
	got := relativePatterns(root, []string{filepath.FromSlash("/m/b/c"), filepath.FromSlash("/m/a")})
	if !reflect.DeepEqual(got, []string{"./a", "./b/c"}) {
		t.Errorf("patterns = %v", got)
	}
}
