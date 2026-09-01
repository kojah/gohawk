package trace

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestEmitFiltersAndEncodesJSONL(t *testing.T) {
	resetTrace(t)
	var output bytes.Buffer
	global.config.writer = &output
	global.config.selectors = map[string]bool{"resourcelifetime": true}
	global.config.source = "resource.go:7"
	global.config.function = "openFile"
	global.active.Store(true)

	files := token.NewFileSet()
	file := files.AddFile("resource.go", -1, 100)
	file.SetLines([]int{0, 10, 20, 30, 40, 50, 60})
	pass := &analysis.Pass{Fset: files}
	Emit(pass, Event{Analyzer: "goroutineownership", Phase: "decision", Reason: "ignored", Pos: file.Pos(60), Function: "openFile"})
	Emit(pass, Event{Analyzer: "resourcelifetime", Check: "resourcelifetime/missing-release", Phase: "decision", Reason: "unowned-return", Outcome: OutcomeRejected, Pos: file.Pos(60), Function: "openFile", Details: map[string]string{"resource": "*os.File"}})

	var got record
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &got); err != nil {
		t.Fatalf("decode trace: %v\n%s", err, output.String())
	}
	if got.Analyzer != "resourcelifetime" || got.Reason != "unowned-return" || got.Position != "resource.go:7:1" || got.Details["resource"] != "*os.File" {
		t.Fatalf("trace = %+v", got)
	}
}

func TestEmitSerializesConcurrentEvents(t *testing.T) {
	resetTrace(t)
	var output bytes.Buffer
	global.config.writer = &output
	global.config.selectors = map[string]bool{"all": true}
	global.active.Store(true)
	pass := &analysis.Pass{Fset: token.NewFileSet()}
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			Emit(pass, Event{Analyzer: "test", Phase: "decision", Reason: "concurrent", Outcome: OutcomeObserved})
		}()
	}
	workers.Wait()
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 20 {
		t.Fatalf("trace line count = %d, want 20", len(lines))
	}
	for _, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("invalid trace line %q", line)
		}
	}
}

func TestRegisterFlagsRejectsInvalidValues(t *testing.T) {
	resetTrace(t)
	flags := flag.NewFlagSet("trace", flag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Parse([]string{"-gohawk-trace=", "./..."}); err == nil {
		t.Fatal("empty trace selector was accepted")
	}
}

func TestEnabledUsesAnalyzerAndCheckSelectors(t *testing.T) {
	resetTrace(t)
	global.config.selectors = map[string]bool{"goroutineownership/unjoined": true}
	global.active.Store(true)
	if !Enabled("goroutineownership", "goroutineownership/unjoined") {
		t.Fatal("selected check is not enabled")
	}
	if Enabled("goroutineownership", "goroutineownership/detached") {
		t.Fatal("unselected check is enabled")
	}
}

func TestEmitDiagnosticResolvesSourceFunction(t *testing.T) {
	resetTrace(t)
	var output bytes.Buffer
	global.config.writer = &output
	global.config.selectors = map[string]bool{"oncepolicy": true}
	global.config.function = "openFile"
	global.active.Store(true)

	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "sample.go", "package sample\nfunc openFile() { println() }\n", 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	function := file.Decls[0]
	pass := &analysis.Pass{Fset: files, Files: []*ast.File{file}}
	EmitDiagnostic(pass, "oncepolicy", "candidate", "diagnostic-candidate", OutcomeObserved, analysis.Diagnostic{Category: "oncepolicy/discarded-wrapper", Pos: function.Pos(), Message: "discarded"})

	var got record
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &got); err != nil {
		t.Fatalf("decode trace: %v\n%s", err, output.String())
	}
	if got.Function != "openFile" || got.Details["message"] != "discarded" {
		t.Fatalf("trace = %+v", got)
	}
}

func resetTrace(t *testing.T) {
	t.Helper()
	global.Lock()
	previous := global.config
	global.config = settings{writer: osStderrForTest{}}
	global.active.Store(false)
	global.Unlock()
	t.Cleanup(func() {
		global.Lock()
		if global.config.file != nil {
			_ = global.config.file.Close()
		}
		global.config = previous
		global.active.Store(len(previous.selectors) > 0)
		global.Unlock()
	})
}

type osStderrForTest struct{}

func (osStderrForTest) Write(value []byte) (int, error) { return len(value), nil }
