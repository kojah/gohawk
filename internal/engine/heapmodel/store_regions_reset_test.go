package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

func TestFreshLoopAllocationDoesNotKeepEarlierContents(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "regionreset", `package regionreset
type box struct{value *int}
func observe(a,b *int){}
func probe(value *int, again bool){
 for again {var local box; observe(local.value,value);local.value=value}
}
`)
	function := pkg.Func("probe")
	var dump strings.Builder
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	call := heapObservation(t, function)
	graph := regionsOfFunction(function)
	if !graph.available {
		t.Fatalf("graph unavailable: %s", graph.buildReason)
	}
	arguments := call.Common().Args
	if graph.aliasProof(arguments[0], arguments[1]).Aliases {
		t.Fatal("zero field of a fresh loop allocation retained an earlier iteration's value")
	}
}
