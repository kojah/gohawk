package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStructuralSameBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "structural", `package structural
type pair struct { first, second *int }
func opaque(*int) *int
func identity(p *int) (*int,*int) { return p,p }
func unrelated(p,q *int) (*int,*int) { return p,q }
func boxed(p *int) (any,*int) { return p,p }
func directional(c chan int) (chan<- int,<-chan int) { return c,c }
func phi(p,q *int, b bool) (*int,*int) { x:=p; if b { x=q }; return x,p }
func cyclic(p *int,n int) (*int,*int) {
 x:=p; for i:=0;i<n;i++ { if i%2==0 { x=new(int) } }; return x,p
}
func unrelatedCycle(p *int,n int) (*int,*int) {
 x:=new(int); for i:=0;i<n;i++ { if i%2==0 { x=new(int) } }; return x,p
}
func fields(p *pair) (**int,**int) { return &p.first,&p.first }
func distinctFields(p *pair) (**int,**int) { return &p.first,&p.second }
func indexes(p []*int,i int) (**int,**int) { return &p[i],&p[i] }
func distinctIndexes(p []*int) (**int,**int) { return &p[0],&p[1] }
func loaded(p **int) (*int,*int) { return *p,*p }
func stored(p,q *int) (*int,*int) {
 x:=p; f:=func(){x=q}; f(); return x,p
}
func contained(p *int) (pair,*int) { x:=pair{first:p}; return x,p }
func called(p *int) (*int,*int) { return opaque(p),p }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"identity", true},
		{"unrelated", false},
		{"boxed", true},
		{"directional", true},
		{"phi", true},
		{"cyclic", true},
		{"unrelatedCycle", false},
		{"fields", true},
		{"distinctFields", false},
		{"indexes", true},
		{"distinctIndexes", false},
		{"loaded", true},
		{"stored", true},
		{"contained", false},
		{"called", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](function)
			if len(returned) != 1 || len(returned[0].Results) != 2 {
				t.Fatal("expected one return with two values")
			}
			left, right := returned[0].Results[0], returned[0].Results[1]
			if got := ssaflow.StructurallySame(left, right); got != test.want {
				t.Errorf("forward possible identity = %v, want %v", got, test.want)
			}
			if got := ssaflow.StructurallySame(right, left); got != test.want {
				t.Errorf("reverse possible identity = %v, want %v", got, test.want)
			}
			if !ssaflow.StructurallySame(left, left) {
				t.Error("direct identity must precede phi or wrapper expansion")
			}
		})
	}
}
