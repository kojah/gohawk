package heapmodel

import (
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const cycleInventoryFixture = `package cycleprobe
import "fmt"
type worker interface{Work()}
func leaf(){}
func left(){leaf()}
func right(){leaf()}
func root(){left();right()}
func otherRoot(){right();left()}
func recursive(){recursive()}
func a(){b()}
func b(){a()}
func launches(dynamic func(), value worker){go left();defer right();dynamic();value.Work();println("ignored")}
func foreign(){fmt.Sprint(1)}
func identity[T any](value T)T{return value}
func generic(){identity(1)}
`

func TestCallCycleInventoryIsReadOnly(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cycleprobe", cycleInventoryFixture)
	root := pkg.Func("root")
	metadata := cycleMetadata(root)
	want := []*ssa.Function{pkg.Func("left"), pkg.Func("right")}
	if metadata.home != pkg || !slices.Equal(metadata.callees, want) {
		t.Fatalf("unexpected root inventory: %+v", metadata)
	}
	for _, name := range []string{"root", "otherRoot"} {
		reach := reachableCallees(pkg.Func(name))
		for _, callee := range []string{"left", "right", "leaf"} {
			if !reach[pkg.Func(callee)] {
				t.Errorf("%s does not reach %s", name, callee)
			}
		}
	}
	if !slices.Equal(metadata.callees, want) {
		t.Fatal("reachability queue overwrote the shared direct-callee inventory")
	}
	RegisterHeapSummary(pkg.Func("left"), HeapSummary{})
	if cycleMetadata(root) != metadata || !slices.Equal(metadata.callees, want) {
		t.Fatal("summary publication changed structural body metadata")
	}
}

func TestCallCycleInventoryPolicies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cycleprobe", cycleInventoryFixture)
	var dump strings.Builder
	for _, name := range []string{"root", "launches", "generic"} {
		if _, err := pkg.Func(name).WriteTo(&dump); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(dump.String())
	if got := cycleMetadata(pkg.Func("launches")).callees; !slices.Equal(got, []*ssa.Function{pkg.Func("left"), pkg.Func("right")}) {
		t.Fatalf("static Go/Defer inventory gained dynamic or builtin calls: %v", got)
	}
	for _, test := range []struct {
		caller, callee string
		want           bool
	}{
		{"root", "left", false}, {"recursive", "recursive", true}, {"a", "b", true}, {"b", "a", true},
	} {
		if got := sameCallCycle(pkg.Func(test.caller), pkg.Func(test.callee)); got != test.want {
			t.Errorf("cycle %s -> %s = %v, want %v", test.caller, test.callee, got, test.want)
		}
	}
	foreign := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("foreign"))[0].Common().StaticCallee()
	if reach := reachableCallees(pkg.Func("foreign")); len(reach) != 0 {
		t.Fatalf("cycle discovery crossed package boundary: %v", reach)
	}
	callCycleReach.Lock()
	foreignMetadata := callCycleReach.metadata[foreign]
	callCycleReach.Unlock()
	if foreignMetadata != nil {
		t.Fatal("cycle discovery scanned a foreign body")
	}
	callees := cycleMetadata(pkg.Func("generic")).callees
	if len(callees) != 2 || callees[0].Origin() == nil || callees[1] != callees[0].Origin() {
		t.Fatalf("generic wrapper/origin pair lost: %v", callees)
	}
	if sameCallCycle(callees[0], callees[1]) {
		t.Fatal("instantiation wrapper calling its origin became recursion")
	}
}

func TestCallCycleInventoryConcurrentPublication(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cycleprobe", cycleInventoryFixture)
	const readers = 16
	start := make(chan struct{})
	results := make(chan *callCycleMetadata, readers)
	for range readers {
		go func() {
			<-start
			reachableCallees(pkg.Func("root"))
			results <- cycleMetadata(pkg.Func("root"))
		}()
	}
	close(start)
	first := <-results
	for range readers - 1 {
		if got := <-results; got != first {
			t.Error("competing readers did not reuse the published inventory")
		}
	}
	if !slices.Equal(first.callees, []*ssa.Function{pkg.Func("left"), pkg.Func("right")}) {
		t.Fatal("concurrent reachability changed the direct inventory")
	}
}
