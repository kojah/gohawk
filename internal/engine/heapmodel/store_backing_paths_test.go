package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
)

func TestNestedBackingCopyIdentity(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"root", `copied:=*source;observe(copied.value,source.value)`, true},
		{"nested", `var copied outer;copied.nested=*source;observe(copied.nested.value,source.value)`, true},
		{"sibling", `var copied outer;copied.nested=*source;observe(copied.nested.value,source.other)`, false},
		{"copyOfCopy", `var copied outer;copied.nested=*source;var next outer;next.nested=copied.nested;observe(next.nested.value,source.value)`, true},
		{"deeper", `var copied struct{padding int;nested outer};copied.nested.nested=*source;observe(copied.nested.nested.value,source.value)`, true},
		{"copyChanged", `var copied outer;copied.nested=*source;copied.nested.value=other;observe(copied.nested.value,source.value)`, false},
		{"sourceChanged", `var copied outer;copied.nested=*source;source.value=other;observe(copied.nested.value,source.value)`, false},
		{"snapshotBeforeChange", `old:=source.value;var copied outer;copied.nested=*source;source.value=other;observe(copied.nested.value,old)`, true},
		{"opaqueCopy", `var copied outer;copied.nested=*source;opaque(&copied);observe(copied.nested.value,source.value)`, false},
		{"opaqueSource", `var copied outer;copied.nested=*source;opaque(source);observe(copied.nested.value,source.value)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "backingpaths", `package backingpaths
type inner struct{value,other *int}
type outer struct{padding int;nested inner}
func observe(a,b *int){}
func opaque(any)
func probe(source *inner,other *int){`+test.body+`}
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
			if got := graph.mustSame(arguments[0], arguments[1]); got != test.want {
				t.Fatalf("nested copy identity = %v, want %v", got, test.want)
			}
			if test.want && !graph.aliasProof(arguments[0], arguments[1]).Aliases {
				t.Fatal("exact copied identity did not support possible aliasing")
			}
		})
	}
}
