package lifecyclefacts

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// A loop that releases values drawn from a parameter is a may-claim about that
// parameter alone. A flag-conditional release is not a loop, and a loop that
// releases some other parameter's elements says nothing about this one.
func TestLifecycleSummaryRecordsReleaseInLoop(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

func CloseEach(name string, files ...*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}
func CloseSlice(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}
func CloseWhenFlagged(file *os.File, flagged bool) {
	if flagged {
		_ = file.Close()
	}
}
func CloseOthers(files [2]*os.File, others []*os.File) {
	_ = files
	for _, other := range others {
		_ = other.Close()
	}
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]map[int]bool{
		"CloseEach":        {0: false, 1: true},
		"CloseSlice":       {0: true},
		"CloseWhenFlagged": {0: false},
		"CloseOthers":      {0: false, 1: true},
	} {
		fact := summarize(pass, pkg.Func(name))
		for index, expected := range want {
			if got := fact.LoopReleased.contains(index); got != expected {
				t.Errorf("%s: LoopReleased parameter %d = %t, want %t", name, index, got, expected)
			}
		}
		if fact.MethodMask("Close") != 0 {
			t.Errorf("%s: a loop or flag must not claim Closed, got %v", name, fact.MethodMask("Close"))
		}
	}
}
