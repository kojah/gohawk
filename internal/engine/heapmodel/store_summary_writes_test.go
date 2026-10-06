package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSummaryWriteInvalidatesAggregateAndKeepsEarlierCopy(t *testing.T) {
	for _, test := range []struct {
		name, path  string
		must, exact bool
	}{
		{"possibleField", "field:1", false, false},
		{"definiteField", "field:1", true, true},
		{"possibleElement", "field:0/index:0", false, false},
		{"possibleWildcard", "field:0/index:*", false, false},
		{"definiteWildcard", "field:0/index:*", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "summarywrite", `package summarywrite
type holder struct{values [1]*int;field *int;sibling *int}
func possibleWrite(*holder,*int)
func probe()(**int,**int,**int,*int,*int){
 old:=new(int);replacement:=new(int)
 source:=holder{values:[1]*int{old},field:old,sibling:old};owner:=new(holder);*owner=source
 before:=*owner;possibleWrite(owner,replacement);after:=*owner
 `+summaryWriteReturn(test.path)+`
}`)
			RegisterHeapSummary(pkg.Func("possibleWrite"), HeapSummary{Edges: []HeapEdge{{
				From: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 0}, Path: test.path},
				To:   HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 1}}},
				Must: test.must,
			}}})
			fn := pkg.Func("probe")
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			returns := ssaflow.InstructionsOf[*ssa.Return](fn)
			if len(returns) != 1 {
				t.Fatalf("return count=%d, want one", len(returns))
			}
			returned := returns[0]
			values := returned.Results
			checkSummarySlotContents(t, returned, test.exact)
			for _, address := range values[1:3] {
				contents, exact := ContentValue(address, returned)
				if !exact || !DefinitelySame(contents, values[3]) {
					t.Fatal("summary write changed earlier snapshot or sibling")
				}
			}
		})
	}
}

func summaryWriteReturn(path string) string {
	if strings.Contains(path, "index:") {
		return `return &after.values[0],&before.values[0],&after.sibling,old,replacement`
	}
	return `return &after.field,&before.field,&after.sibling,old,replacement`
}

// Possible containment may retain historical aggregate objects even after a
// definite replacement. Exact content and positive possible contents are the
// guarantees this test needs; it does not require negative may evidence.
func checkSummarySlotContents(t *testing.T, returned *ssa.Return, must bool) {
	t.Helper()
	v := returned.Results
	contents, exact := ContentValue(v[0], returned)
	if must {
		if !exact || !DefinitelySame(contents, v[4]) {
			t.Fatal("definite summary write lost exact replacement")
		}
	} else {
		if exact {
			t.Fatalf("possible summary write established exact contents %v", contents)
		}
		retained, known := ContainsAt(v[0], v[3], returned)
		if !known || !retained {
			t.Fatal("possible summary write lost prior contents")
		}
	}
	retained, known := ContainsAt(v[0], v[4], returned)
	if !known || !retained {
		t.Fatal("summary write lost possible replacement")
	}
}
