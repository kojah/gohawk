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

type sink interface{ Log(string) }

func SetThenLog(o *owner, c *closer, s sink) { o.body = c; s.Log("set") }
func UseAfterLog(c *closer, s sink) *closer  { o := &owner{}; SetThenLog(o, c, s); return o.body }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"Wrap":        {"edge R0 -> fresh(new#", "edge R0/field:0 -> P0 must"},
		"CloseSecond": {"effect P0/field:1 released Close every", "read P0/field:1"},
		"Keep":        {"edge G:example.com/lifecyclefactstest.saved -> P0 must", "effect P0 escaped global every"},
		"Inspect":     {"read P0/field:0"},
		// The interface call can reach only what it was handed: the sink
		// is truncated, the owner and the closer keep their edge.
		"SetThenLog":  {"edge P0/field:0 -> P1 must", "truncated P2"},
		"UseAfterLog": {"holds R0 P0 must"},
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
		if name == "SetThenLog" && (strings.Contains(rendered, "truncated P0") || strings.Contains(rendered, "truncated P1")) {
			t.Errorf("%s truncates a parameter the interface call could not reach:\n%s", name, rendered)
		}
	}
	// Inspect reads and dereferences its parameter and nothing more: the
	// projection carries the read and the non-nil requirement, no edge, no
	// effect, and no cut.
	if fact := summarize(pass, pkg.Func("Inspect")); len(fact.Heap.Edges) != 0 || len(fact.Heap.Effects) != 0 || len(fact.Heap.Truncated) != 0 {
		t.Errorf("Inspect should claim nothing beyond its reads, got heap:\n%s", fact.Heap.String())
	}
}

func TestAsynchronousExposureClaim(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
type writer interface{ Write([]byte) (int, error) }
type holder struct{ destination writer }
func Async(w writer) { go w.Write(nil) }
func Borrow(w writer) { _, _ = w.Write(nil) }
func Other(w, other writer) { _, _ = w.Write(nil); go other.Write(nil) }
func Nested(h *holder) { go h.destination.Write(nil) }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]ParameterMask{
		"Async": parameterMaskFor(0), "Borrow": 0, "Other": parameterMaskFor(1), "Nested": 0,
	} {
		fact := summarize(pass, pkg.Func(name))
		if got := fact.Claim(ClaimAsynchronouslyExposes); got != want {
			t.Errorf("%s asynchronous parameters=%b, want %b; heap:\n%s", name, got, want, fact.Heap.String())
		}
	}
	if mask := (&Fact{}).Claim(ClaimAsynchronouslyExposes); mask != 0 {
		t.Errorf("unavailable heap claims asynchronous exposure: %b", mask)
	}
}
