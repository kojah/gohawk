package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPrivateHelperRetentionUsesReturnedWrapperAtEscape(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
import "os"
type holder struct { file *os.File }
var global *holder
var globalHook func()
func wrap(file *os.File) *holder { return &holder{file} }
func install(file *os.File) { global = wrap(file) }
func discard(file *os.File) { _ = wrap(file) }
func returnOnly(file *os.File) *holder { return wrap(file) }
func maybeInstall(file *os.File, fail bool) {
	h := wrap(file)
	if fail { return }
	global = h
}
func unrelated(file *os.File) { global = wrap(nil) }
func recurse(file *os.File) { recurse(file) }
func capture(file *os.File) { globalHook = func() { file.Close() } }
func localMap(file *os.File) { _ = map[int]*os.File{0:file} }
func localSlice(file *os.File) { _ = append([]*os.File{}, file) }
func callerCell(file *os.File, dst **os.File) { *dst = file }
func exercise(file *os.File, fail bool) {
	install(file)
	discard(file)
	_ = returnOnly(file)
	maybeInstall(file, fail)
	unrelated(file)
	recurse(file)
	capture(file)
	localMap(file)
	localSlice(file)
	var dst *os.File
	callerCell(file, &dst)
}
`)
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{Analyzer: Summaries{
			pkg.Func("wrap"): {ReturnedOwner: 1, ReturnedView: 1, Stored: 1, Retained: 1},
		}},
		ImportObjectFact: func(types.Object, analysis.Fact) bool {
			t.Error("consumer retention must use prerequisite summaries")
			return false
		},
	}
	evidence := NewLifecycleEvidence(pass, "test", "test")
	want := map[string]bool{
		"install": true, "discard": false, "returnOnly": false,
		"maybeInstall": false, "unrelated": false, "recurse": false, "capture": false,
		"localMap": false, "localSlice": false, "callerCell": false,
	}
	function := pkg.Func("exercise")
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		name := call.Common().StaticCallee().Name()
		expected, ok := want[name]
		if !ok {
			continue
		}
		if got := evidence.ArgumentRetainedByCallee(call, function.Params[0]); got != expected {
			t.Errorf("%s retention = %v, want %v", name, got, expected)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing calls: %v", want)
	}
}
