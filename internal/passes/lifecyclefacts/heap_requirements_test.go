package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
)

// A requirement is a method the function calls on the object at a named
// slot on every normal return, directly or through a summarized callee.
// The accepted forms pin the boundary: a call on some path only, a call on
// an object the function made itself, and a receiver the graph cannot
// resolve to one object require nothing.
func TestLifecycleSummaryRequirements(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type reader struct{ id int }

func (r *reader) Read(p []byte) (int, error) { return 0, nil }
func (r *reader) Err() error                 { return nil }

type holder struct{ r *reader }

func open() *reader { return &reader{} }

func Drain(r *reader) error                   { _, err := r.Read(nil); return err }
func Sometimes(r *reader, ok bool) error      { if ok { _, err := r.Read(nil); return err }; return nil }
func Twice(r *reader) error                   { _, _ = r.Read(nil); return r.Err() }
func ViaHelper(r *reader) error               { return Drain(r) }
func ViaSometimes(r *reader, ok bool) error   { return Sometimes(r, ok) }
func Replaced(r *reader) error                { r = open(); _, err := r.Read(nil); return err }
func Held(h holder) error                     { _, err := h.r.Read(nil); return err }
func Either(a, b *reader, ok bool) error      { r := a; if ok { r = b }; _, err := r.Read(nil); return err }
func Panics(r *reader) error                  { _, _ = r.Read(nil); panic("never returns") }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"Drain":        {"P0 method Read"},
		"Sometimes":    nil,
		"Twice":        {"P0 method Err", "P0 method Read"},
		"ViaHelper":    {"P0 method Read"},
		"ViaSometimes": nil,
		"Replaced":     nil,
		"Held":         {"P0/field:0 method Read"},
		"Either":       nil,
		"Panics":       nil,
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no projection", name)
		}
		var got []string
		for _, requirement := range fact.Heap.Requires {
			got = append(got, requirement.Slot.String()+" method "+requirement.Method)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s requires %q, want %q\n%s", name, got, want, fact.Heap.String())
		}
	}
	var _ ssaflow.HeapRequirement
}
