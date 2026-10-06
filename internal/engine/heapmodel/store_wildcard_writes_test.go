package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestWildcardWriteKeepsUnwrittenElementPossibilities(t *testing.T) {
	for _, test := range []struct {
		body  string
		exact bool
	}{
		{`var a [2]*int;a[i]=p`, false},
		{`a:=supplied;a[i]=p`, false},
		{`var a [2]*int;a[0]=p`, true},
		{`a:=supplied;a[0]=p`, true},
	} {
		t.Run(test.body, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "wildcardwrite", `package wildcardwrite
func probe(i int,p *int,supplied [2]*int)(*int,*int){`+test.body+`;return a[0],p}
`)
			fn := pkg.Func("probe")
			t.Log(RenderRegions(fn))
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			if exact := DefinitelySame(returned.Results[0], returned.Results[1]); exact != test.exact {
				t.Fatalf("element zero exact=%v, want %v", exact, test.exact)
			}
		})
	}
}
