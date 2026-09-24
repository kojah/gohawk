package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestContentIsNilAcrossPointerField(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilpathprobe", `package nilpathprobe
type inner struct{ value *int }
type outer struct{ next *inner }
type inline struct{ next inner }
func external() *inner
func observe(any) {}
func opaque() { o := &outer{next: external()}; observe(o) }
func knownNil() { o := &outer{next: &inner{}}; observe(o) }
func nilParent() { o := &outer{}; observe(o) }
func inlineNil() { o := &inline{}; observe(o) }
`)
	for _, test := range []struct {
		name string
		path []string
		want bool
	}{
		{"opaque", []string{"field:0", "field:0"}, false},
		{"knownNil", []string{"field:0", "field:0"}, true},
		{"nilParent", []string{"field:0"}, true},
		{"nilParent", []string{"field:0", "field:0"}, false},
		{"inlineNil", []string{"field:0", "field:0"}, true},
	} {
		t.Run(test.name+"/"+test.path[len(test.path)-1], func(t *testing.T) {
			call := heapObservation(t, pkg.Func(test.name))
			root := unwrapInterface(call.Common().Args[0])
			got := regionsOfFunction(call.Parent()).contentIsNil(root, test.path, call)
			if got != test.want {
				t.Errorf("contentIsNil(%v) = %t, want %t\n%s", test.path, got, test.want, RenderRegions(call.Parent()))
			}
		})
	}
}
