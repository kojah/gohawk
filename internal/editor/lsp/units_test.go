package lsp

import (
	"bytes"
	"path/filepath"
	"reflect"
	"runtime"
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
	buffer := &overlay{texts: map[string]string{path: "x\ns := \"é😀\"; call()\n"}, original: map[string]string{}}
	convert := newConverter(func(string) CheckInfo { return CheckInfo{Experimental: true} }, buffer)
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

func TestOverlayMapsReportedPathsBack(t *testing.T) {
	t.Parallel()
	real := filepath.Join(t.TempDir(), "pkg", "a.go")
	written, err := writeOverlay(t.TempDir(), map[string]string{real: "package pkg\n"})
	if err != nil {
		t.Fatal(err)
	}
	defer written.remove()
	var temporary string
	for path := range written.original {
		temporary = path
	}
	if got := written.realPath(temporary); got != real {
		t.Errorf("realPath(%q) = %q, want %q", temporary, got, real)
	}
	if got := written.realPath("/elsewhere/b.go"); got != "/elsewhere/b.go" {
		t.Errorf("an unrelated path was rewritten to %q", got)
	}
	if none, err := writeOverlay(t.TempDir(), nil); none != nil || err != nil {
		t.Errorf("no unsaved buffers produced an overlay: %v, %v", none, err)
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
	touched := store.replace([]string{filepath.Dir(a)}, nil)
	if !reflect.DeepEqual(touched, []string{a}) {
		t.Errorf("touched = %v, want only %s", touched, a)
	}
	if len(store.forFile(a)) != 0 || len(store.forFile(b)) != 1 || store.total() != 1 {
		t.Errorf("after replace: a=%d b=%d total=%d", len(store.forFile(a)), len(store.forFile(b)), store.total())
	}
	// A finding outside the covered packages is not installed.
	store.replace([]string{filepath.Dir(a)}, []fileDiagnostic{finding(filepath.FromSlash("/m/c/c.go"))})
	if store.total() != 1 {
		t.Errorf("an uncovered finding was stored; total = %d", store.total())
	}
	store.markStale([]string{filepath.Dir(a), filepath.Dir(b)})
	if !store.stale[filepath.Dir(b)] || store.stale[filepath.Dir(a)] {
		t.Errorf("stale = %v, want only the package that has findings", store.stale)
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
