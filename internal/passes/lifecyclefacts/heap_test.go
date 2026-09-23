package lifecyclefacts

import (
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// The exported heap projection carries the claims the masks make, in one
// encoding: a constructor's result holding its parameter is an edge from
// the result to the parameter, a release on every return is a release
// effect at the parameter's path, and a parameter stored into a global is
// an escape effect.
func TestLifecycleSummaryHeapProjection(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type owner struct{ body *closer }
type pair struct{ first, second *closer }

var saved *closer

func Wrap(c *closer) *owner        { return &owner{body: c} }
func CloseSecond(p *pair) error    { return p.second.Close() }
func Keep(c *closer)               { saved = c }
func Inspect(p *pair) bool         { return p.first != nil }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"Wrap":        {"edge R0 -> fresh(new#", "edge R0/field:0 -> P0 must"},
		"CloseSecond": {"effect P0/field:1 released Close every", "read P0/field:1"},
		"Keep":        {"edge G:example.com/lifecyclefactstest.saved -> P0 must", "effect P0 escaped global every"},
		"Inspect":     {"read P0/field:0"},
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no heap projection", name)
		}
		rendered := fact.Heap.String()
		for _, line := range want {
			if !strings.Contains(rendered, line) {
				t.Errorf("%s projection lacks %q:\n%s", name, line, rendered)
			}
		}
	}
	// Inspect reads and dereferences its parameter and nothing more: the
	// projection carries the read and the non-nil requirement, no edge, no
	// effect, and no cut.
	if fact := summarize(pass, pkg.Func("Inspect")); len(fact.Heap.Edges) != 0 || len(fact.Heap.Effects) != 0 || len(fact.Heap.Truncated) != 0 {
		t.Errorf("Inspect should claim nothing beyond its reads, got heap:\n%s", fact.Heap.String())
	}
}
