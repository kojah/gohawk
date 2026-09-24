package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"golang.org/x/tools/go/analysis"
)

func TestImportedHeapSummaryVersion(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
func Exported() {}
`)
	version := heapmodel.SummaryVersion - 1
	pass := &analysis.Pass{ImportObjectFact: func(_ types.Object, target analysis.Fact) bool {
		fact := target.(*Fact) //nolint:forcetypeassert // The query imports this fact type.
		fact.Heap = &heapmodel.HeapSummary{Version: version}
		return true
	}}
	if _, ok := factForFunction(pass, pkg.Func("Exported")); ok {
		t.Fatal("older heap edge semantics must not be imported")
	}
	version = heapmodel.SummaryVersion
	if _, ok := factForFunction(pass, pkg.Func("Exported")); !ok {
		t.Fatal("current heap edge semantics should be imported")
	}
}
